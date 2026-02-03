package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/raphaelgruber/memcp-go/internal/db"
	"github.com/raphaelgruber/memcp-go/internal/llm"
	"github.com/raphaelgruber/memcp-go/internal/models"
	"github.com/raphaelgruber/memcp-go/internal/parser"
)

// IngestService handles file ingestion into the knowledge base.
type IngestService struct {
	db            *db.Client
	embedder      *llm.Embedder
	model         *llm.Model
	entityService *EntityService
}

// NewIngestService creates a new ingest service.
func NewIngestService(db *db.Client, embedder *llm.Embedder, model *llm.Model) *IngestService {
	return &IngestService{
		db:            db,
		embedder:      embedder,
		model:         model,
		entityService: NewEntityService(db, embedder, model),
	}
}

// IngestOptions configures file ingestion.
type IngestOptions struct {
	// Labels to apply to all ingested entities
	Labels []string
	// ExtractGraph uses LLM to extract entity relationships
	ExtractGraph bool
	// DryRun previews what would be ingested without making changes
	DryRun bool
	// Recursive processes subdirectories
	Recursive bool
}

// IngestResult summarizes an ingestion operation.
type IngestResult struct {
	FilesProcessed   int
	EntitiesCreated  int
	ChunksCreated    int
	RelationsCreated int
	Errors           []string
}

// IngestFile ingests a single Markdown file.
func (s *IngestService) IngestFile(ctx context.Context, filePath string, opts IngestOptions) (*models.Entity, error) {
	// Read file
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	// Parse markdown
	doc, err := parser.ParseMarkdown(string(content))
	if err != nil {
		return nil, fmt.Errorf("parse markdown: %w", err)
	}

	// Determine entity type from frontmatter or default
	entityType := doc.GetFrontmatterString("type")
	if entityType == "" {
		entityType = "document"
	}

	// Get name from title or filename
	name := doc.Title
	if name == "" {
		name = strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))
	}

	// Merge labels from frontmatter and options
	labels := doc.GetFrontmatterStringSlice("labels")
	if labels == nil {
		labels = doc.GetFrontmatterStringSlice("tags")
	}
	labels = append(labels, opts.Labels...)

	// Build entity input
	fullContent := doc.Content
	input := models.EntityInput{
		Type:       entityType,
		Name:       name,
		Content:    &fullContent,
		Labels:     labels,
		SourcePath: &filePath,
	}

	// Add summary if present
	if summary := doc.GetFrontmatterString("summary"); summary != "" {
		input.Summary = &summary
	} else if description := doc.GetFrontmatterString("description"); description != "" {
		input.Summary = &description
	}

	// Set source
	source := string(models.SourceScrape)
	input.Source = &source

	// Check confidence/verified from frontmatter
	if verified, ok := doc.Frontmatter["verified"].(bool); ok {
		input.Verified = &verified
	}

	// Dry run - just return what would be created
	if opts.DryRun {
		return &models.Entity{
			Type:       input.Type,
			Name:       name,
			Content:    &fullContent,
			Labels:     labels,
			SourcePath: &filePath,
			Source:     source,
		}, nil
	}

	// Create entity
	entity, err := s.entityService.Create(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("create entity: %w", err)
	}

	// Extract relations from content
	relations := s.extractInferredRelations(ctx, doc, entity)
	for _, rel := range relations {
		if err := s.db.CreateRelation(ctx, rel); err != nil {
			// Log but don't fail
			fmt.Printf("Warning: failed to create relation: %v\n", err)
		}
	}

	// Extract graph relations using LLM if requested
	if opts.ExtractGraph && s.model != nil {
		if err := s.extractGraphRelations(ctx, entity); err != nil {
			// Log but don't fail
			fmt.Printf("Warning: graph extraction failed: %v\n", err)
		}
	}

	return entity, nil
}

// extractInferredRelations finds [[wiki-links]] and @mentions.
func (s *IngestService) extractInferredRelations(ctx context.Context, doc *parser.MarkdownDoc, entity *models.Entity) []models.RelationInput {
	var relations []models.RelationInput
	entityID := entity.ID.ID.(string)

	// Extract wiki links
	links := parser.ExtractWikiLinks(doc.Content)
	for _, link := range links {
		// Try to find matching entity
		target, _ := s.db.GetEntityByName(ctx, link)
		if target != nil {
			targetID := target.ID.ID.(string)
			source := string(models.RelationSourceInferred)
			relations = append(relations, models.RelationInput{
				FromID:  entityID,
				ToID:    targetID,
				RelType: "references",
				Source:  &source,
			})
		}
	}

	// Extract mentions
	mentions := parser.ExtractMentions(doc.Content)
	for _, mention := range mentions {
		// Try to find matching person entity
		target, _ := s.db.GetEntityByName(ctx, mention)
		if target != nil && target.Type == "person" {
			targetID := target.ID.ID.(string)
			source := string(models.RelationSourceInferred)
			relations = append(relations, models.RelationInput{
				FromID:  entityID,
				ToID:    targetID,
				RelType: "mentions",
				Source:  &source,
			})
		}
	}

	// Extract relations from frontmatter
	if relatesTo := doc.GetFrontmatterStringSlice("relates_to"); len(relatesTo) > 0 {
		for _, targetName := range relatesTo {
			target, _ := s.db.GetEntityByName(ctx, targetName)
			if target != nil {
				targetID := target.ID.ID.(string)
				source := string(models.RelationSourceInferred)
				relations = append(relations, models.RelationInput{
					FromID:  entityID,
					ToID:    targetID,
					RelType: "relates_to",
					Source:  &source,
				})
			}
		}
	}

	return relations
}

// extractGraphRelations uses LLM to extract entity relationships (GraphRAG-style).
func (s *IngestService) extractGraphRelations(ctx context.Context, entity *models.Entity) error {
	if entity.Content == nil || s.model == nil {
		return nil
	}

	// Get existing entity names for context
	existingEntities, _ := s.db.ListEntities(ctx, "", nil, 100)
	entityNames := make([]string, 0, len(existingEntities))
	for _, e := range existingEntities {
		entityNames = append(entityNames, e.Name)
	}

	// Extract entities and relations using LLM
	result, err := s.model.ExtractEntitiesAndRelations(ctx, *entity.Content, entityNames)
	if err != nil {
		return fmt.Errorf("LLM extraction: %w", err)
	}

	// Parse LLM output
	lines := strings.Split(result, "\n")
	entityID := entity.ID.ID.(string)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		parts := strings.Split(line, "|")

		if len(parts) >= 4 && parts[0] == "ENTITY" {
			// Create new entity if it doesn't exist
			name := strings.TrimSpace(parts[1])
			existing, _ := s.db.GetEntityByName(ctx, name)
			if existing == nil {
				entityType := strings.TrimSpace(parts[2])
				description := strings.TrimSpace(parts[3])
				source := string(models.SourceAIGenerated)
				verified := false
				confidence := 0.7

				_, err := s.entityService.Create(ctx, models.EntityInput{
					Type:       entityType,
					Name:       name,
					Summary:    &description,
					Source:     &source,
					Verified:   &verified,
					Confidence: &confidence,
				})
				if err != nil {
					fmt.Printf("Warning: failed to create entity %s: %v\n", name, err)
				}
			}
		}

		if len(parts) >= 5 && parts[0] == "RELATION" {
			sourceName := strings.TrimSpace(parts[1])
			targetName := strings.TrimSpace(parts[2])
			relType := strings.TrimSpace(parts[3])

			// Find source and target entities
			sourceEntity, _ := s.db.GetEntityByName(ctx, sourceName)
			targetEntity, _ := s.db.GetEntityByName(ctx, targetName)

			if sourceEntity != nil && targetEntity != nil {
				sourceID := sourceEntity.ID.ID.(string)
				targetID := targetEntity.ID.ID.(string)
				relSource := string(models.RelationSourceAIDetected)

				err := s.db.CreateRelation(ctx, models.RelationInput{
					FromID:  sourceID,
					ToID:    targetID,
					RelType: relType,
					Source:  &relSource,
				})
				if err != nil {
					fmt.Printf("Warning: failed to create relation %s->%s: %v\n", sourceName, targetName, err)
				}
			}
		}

		// Also link extracted entities to the source entity
		if len(parts) >= 4 && parts[0] == "ENTITY" {
			name := strings.TrimSpace(parts[1])
			targetEntity, _ := s.db.GetEntityByName(ctx, name)
			if targetEntity != nil {
				targetID := targetEntity.ID.ID.(string)
				relSource := string(models.RelationSourceAIDetected)

				_ = s.db.CreateRelation(ctx, models.RelationInput{
					FromID:  entityID,
					ToID:    targetID,
					RelType: "mentions",
					Source:  &relSource,
				})
			}
		}
	}

	return nil
}

// IngestDirectory ingests all Markdown files from a directory.
func (s *IngestService) IngestDirectory(ctx context.Context, dirPath string, opts IngestOptions) (*IngestResult, error) {
	result := &IngestResult{}

	// Find markdown files
	var files []string
	walkFn := func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && !opts.Recursive && path != dirPath {
			return filepath.SkipDir
		}
		ext := strings.ToLower(filepath.Ext(path))
		if !d.IsDir() && (ext == ".md" || ext == ".markdown") {
			files = append(files, path)
		}
		return nil
	}

	if err := filepath.WalkDir(dirPath, walkFn); err != nil {
		return nil, fmt.Errorf("scan directory: %w", err)
	}

	// Process files
	for _, file := range files {
		result.FilesProcessed++

		entity, err := s.IngestFile(ctx, file, opts)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", file, err))
			continue
		}

		result.EntitiesCreated++

		// Count chunks
		if entity != nil && !opts.DryRun {
			chunks, _ := s.db.GetChunks(ctx, entity.ID.ID.(string))
			result.ChunksCreated += len(chunks)
		}
	}

	return result, nil
}
