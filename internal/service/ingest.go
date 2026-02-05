package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

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
	// Name is a user-provided identifier for the job (for rerunning)
	Name string
	// Labels to apply to all ingested entities (curated)
	Labels []string
	// ExtractGraph uses LLM to extract entity relationships
	ExtractGraph bool
	// DryRun previews what would be ingested without making changes
	DryRun bool
	// Recursive processes subdirectories
	Recursive bool
	// Concurrency sets number of parallel workers (default 4)
	Concurrency int
	// Job for progress reporting (optional, set by async ingestion)
	Job *Job
	// BaseDir is used to compute unique entity IDs (e.g., "insights" from ~/.claude/insights)
	BaseDir string
}

// IngestResult summarizes an ingestion operation.
type IngestResult struct {
	FilesProcessed   int
	FilesSkipped     int
	EntitiesCreated  int
	ChunksCreated    int
	RelationsCreated int
	Errors           []string
}

// FileHash represents a file path and its content hash.
type FileHash struct {
	Path string
	Hash string
}

// FileContent represents a file with its content and hash.
type FileContent struct {
	Path    string
	Content string
	Hash    string
}

// IngestFilesInput contains files and metadata for content-based ingestion.
type IngestFilesInput struct {
	Files   []FileContent
	BaseDir string // Base directory name for entity ID derivation (e.g., "insights")
}

// IngestFileResult contains the result of ingesting a single file.
type IngestFileResult struct {
	Entity        *models.Entity
	ChunksCreated int
}

// CheckHashes determines which files need uploading based on their content hashes.
// Returns paths that are NOT in the database (new or changed content).
func (s *IngestService) CheckHashes(ctx context.Context, files []FileHash) ([]string, error) {
	if len(files) == 0 {
		return []string{}, nil
	}

	// Extract all hashes for bulk query
	hashes := make([]string, len(files))
	hashToPath := make(map[string]string, len(files))
	for i, f := range files {
		hashes[i] = f.Hash
		hashToPath[f.Hash] = f.Path
	}

	// Query existing hashes in DB
	existing, err := s.db.GetExistingHashes(ctx, hashes)
	if err != nil {
		return nil, fmt.Errorf("check existing hashes: %w", err)
	}

	// Build set of existing hashes for O(1) lookup
	existingSet := make(map[string]struct{}, len(existing))
	for _, h := range existing {
		existingSet[h] = struct{}{}
	}

	// Return paths whose hashes are NOT in DB
	needed := make([]string, 0, len(files))
	for _, f := range files {
		if _, exists := existingSet[f.Hash]; !exists {
			needed = append(needed, f.Path)
		}
	}

	return needed, nil
}

// IngestFileWithContent ingests a file from provided content (not from disk).
// Used by the two-phase hash-based ingestion flow.
// baseDir is used to compute unique entity IDs from relative file paths.
func (s *IngestService) IngestFileWithContent(ctx context.Context, filePath, content, contentHash, baseDir string, opts IngestOptions) (*IngestFileResult, error) {
	return s.ingestFileInternal(ctx, filePath, []byte(content), &contentHash, baseDir, opts)
}

// IngestFile ingests a single Markdown file.
func (s *IngestService) IngestFile(ctx context.Context, filePath string, opts IngestOptions) (*IngestFileResult, error) {
	// Read file
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}
	return s.ingestFileInternal(ctx, filePath, content, nil, opts.BaseDir, opts)
}

// ingestFileInternal handles the core ingestion logic for both IngestFile and IngestFileWithContent.
// If contentHash is nil, no hash is stored; if provided, it's stored for skip-unchanged deduplication.
// baseDir is used to compute unique entity IDs: baseDir + filename (without ext). If empty, uses name.
func (s *IngestService) ingestFileInternal(ctx context.Context, filePath string, content []byte, contentHash *string, baseDir string, opts IngestOptions) (*IngestFileResult, error) {
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

	// Compute entity ID from baseDir + filename for uniqueness
	// e.g., baseDir="insights", filePath=".../2026-02-04-tests-abc.md" → ID="insights-2026-02-04-tests-abc"
	var entityID *string
	if baseDir != "" {
		filename := strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))
		id := slugify(baseDir + "-" + filename)
		entityID = &id
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
		ID:          entityID,
		Type:        entityType,
		Name:        name,
		Content:     &fullContent,
		Labels:      labels,
		SourcePath:  &filePath,
		ContentHash: contentHash,
	}

	// Add summary if present
	if summary := doc.GetFrontmatterString("summary"); summary != "" {
		input.Summary = &summary
	} else if description := doc.GetFrontmatterString("description"); description != "" {
		input.Summary = &description
	}

	// Set source
	source := models.SourceScrape
	input.Source = &source

	// Check confidence/verified from frontmatter
	if verified, ok := doc.Frontmatter["verified"].(bool); ok {
		input.Verified = &verified
	}

	// Dry run - just return what would be created
	if opts.DryRun {
		return &IngestFileResult{
			Entity: &models.Entity{
				Type:        input.Type,
				Name:        name,
				Content:     &fullContent,
				Labels:      labels,
				SourcePath:  &filePath,
				ContentHash: contentHash,
				Source:      source,
			},
		}, nil
	}

	// Create entity
	createResult, err := s.entityService.Create(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("create entity: %w", err)
	}

	// Extract relations from content
	relations := s.extractInferredRelations(ctx, doc, createResult.Entity)
	for _, rel := range relations {
		if err := s.db.CreateRelation(ctx, rel); err != nil {
			// Log but don't fail
			slog.Warn("failed to create inferred relation", "from", rel.FromID, "to", rel.ToID, "error", err)
		}
	}

	// Extract graph relations using LLM if requested
	if opts.ExtractGraph && s.model != nil {
		if err := s.extractGraphRelations(ctx, createResult.Entity); err != nil {
			// Fatal API errors (billing, auth) should stop everything
			if errors.Is(err, llm.ErrFatalAPI) {
				return nil, fmt.Errorf("graph extraction: %w", err)
			}
			// Log but don't fail for other errors
			slog.Warn("graph extraction failed", "file", filePath, "error", err)
		}
	}

	return &IngestFileResult{
		Entity:        createResult.Entity,
		ChunksCreated: createResult.ChunksCreated,
	}, nil
}

// extractInferredRelations finds [[wiki-links]] and @mentions.
func (s *IngestService) extractInferredRelations(ctx context.Context, doc *parser.MarkdownDoc, entity *models.Entity) []models.RelationInput {
	var relations []models.RelationInput
	entityID, err := models.RecordIDString(entity.ID)
	if err != nil {
		slog.Warn("failed to get entity ID for relation extraction", "error", err)
		return relations
	}

	// Extract all target names from various sources
	links := parser.ExtractWikiLinks(doc.Content)
	mentions := parser.ExtractMentions(doc.Content)
	relatesTo := doc.GetFrontmatterStringSlice("relates_to")

	// Collect all unique names for batch lookup
	allNames := make([]string, 0, len(links)+len(mentions)+len(relatesTo))
	allNames = append(allNames, links...)
	allNames = append(allNames, mentions...)
	allNames = append(allNames, relatesTo...)

	if len(allNames) == 0 {
		return relations
	}

	// Single batch lookup for all entity names
	entityMap, err := s.db.GetEntitiesByNames(ctx, allNames)
	if err != nil {
		slog.Warn("failed to batch lookup entities for relations", "error", err)
		return relations
	}

	relSource := string(models.RelationSourceInferred)

	// Process wiki links
	for _, link := range links {
		target := entityMap[strings.ToLower(link)]
		if target == nil {
			continue
		}
		targetID, err := models.RecordIDString(target.ID)
		if err != nil {
			slog.Debug("failed to get target ID for wiki link", "link", link, "error", err)
			continue
		}
		relations = append(relations, models.RelationInput{
			FromID:  entityID,
			ToID:    targetID,
			RelType: "references",
			Source:  &relSource,
		})
	}

	// Process mentions (only person entities)
	for _, mention := range mentions {
		target := entityMap[strings.ToLower(mention)]
		if target == nil || target.Type != "person" {
			continue
		}
		targetID, err := models.RecordIDString(target.ID)
		if err != nil {
			slog.Debug("failed to get target ID for mention", "mention", mention, "error", err)
			continue
		}
		relations = append(relations, models.RelationInput{
			FromID:  entityID,
			ToID:    targetID,
			RelType: "mentions",
			Source:  &relSource,
		})
	}

	// Process frontmatter relates_to
	for _, targetName := range relatesTo {
		target := entityMap[strings.ToLower(targetName)]
		if target == nil {
			continue
		}
		targetID, err := models.RecordIDString(target.ID)
		if err != nil {
			slog.Debug("failed to get target ID for frontmatter relation", "target", targetName, "error", err)
			continue
		}
		relations = append(relations, models.RelationInput{
			FromID:  entityID,
			ToID:    targetID,
			RelType: "relates_to",
			Source:  &relSource,
		})
	}

	return relations
}

// extractGraphRelations uses LLM to extract entity relationships (GraphRAG-style).
func (s *IngestService) extractGraphRelations(ctx context.Context, entity *models.Entity) error {
	if entity.Content == nil || s.model == nil {
		return nil
	}

	entityID, err := models.RecordIDString(entity.ID)
	if err != nil {
		return fmt.Errorf("get entity ID: %w", err)
	}

	contentLen := len(*entity.Content)
	slog.Debug("starting graph extraction", "entity", entity.Name, "content_len", contentLen)

	// Get existing entity names for context
	existingEntities, err := s.db.ListEntities(ctx, "", nil, 100)
	if err != nil {
		slog.Warn("failed to list entities for graph context", "error", err)
		// Continue with empty list - LLM can still extract new entities
	}
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

	for _, line := range lines {
		line = strings.TrimSpace(line)
		parts := strings.Split(line, "|")

		if len(parts) >= 4 && parts[0] == "ENTITY" {
			// Create new entity if it doesn't exist
			name := strings.TrimSpace(parts[1])
			existing, err := s.db.GetEntityByName(ctx, name)
			if err != nil {
				slog.Debug("failed to check existing entity", "name", name, "error", err)
			}
			if existing == nil && err == nil {
				entityType := strings.TrimSpace(parts[2])
				description := strings.TrimSpace(parts[3])
				aiSource := models.SourceAIGenerated
				verified := false
				confidence := 0.7

				_, err := s.entityService.Create(ctx, models.EntityInput{
					Type:       entityType,
					Name:       name,
					Summary:    &description,
					Source:     &aiSource,
					Verified:   &verified,
					Confidence: &confidence,
				})
				if err != nil {
					// Race condition: entity may have been created by another worker
					// Handle both "already exists" and transaction conflicts
					if errors.Is(err, db.ErrEntityAlreadyExists) || errors.Is(err, db.ErrTransactionConflict) {
						slog.Debug("entity already exists or conflict, skipping extraction", "name", name)
					} else {
						slog.Warn("failed to create entity from graph extraction", "name", name, "error", err)
					}
				}
			}
		}

		if len(parts) >= 5 && parts[0] == "RELATION" {
			sourceName := strings.TrimSpace(parts[1])
			targetName := strings.TrimSpace(parts[2])
			relType := strings.TrimSpace(parts[3])

			// Find source and target entities
			sourceEntity, err := s.db.GetEntityByName(ctx, sourceName)
			if err != nil {
				slog.Debug("failed to lookup source entity for relation", "source", sourceName, "error", err)
				continue
			}
			targetEntity, err := s.db.GetEntityByName(ctx, targetName)
			if err != nil {
				slog.Debug("failed to lookup target entity for relation", "target", targetName, "error", err)
				continue
			}

			if sourceEntity != nil && targetEntity != nil {
				sourceID, srcErr := models.RecordIDString(sourceEntity.ID)
				targetID, tgtErr := models.RecordIDString(targetEntity.ID)
				if srcErr == nil && tgtErr == nil {
					relSource := string(models.RelationSourceAIDetected)

					err := s.db.CreateRelation(ctx, models.RelationInput{
						FromID:  sourceID,
						ToID:    targetID,
						RelType: relType,
						Source:  &relSource,
					})
					if err != nil {
						slog.Warn("failed to create relation from graph extraction", "source", sourceName, "target", targetName, "error", err)
					}
				}
			}
		}

		// Also link extracted entities to the source entity
		if len(parts) >= 4 && parts[0] == "ENTITY" {
			name := strings.TrimSpace(parts[1])
			targetEntity, err := s.db.GetEntityByName(ctx, name)
			if err != nil {
				slog.Debug("failed to lookup extracted entity", "name", name, "error", err)
				continue
			}
			if targetEntity != nil {
				targetID, err := models.RecordIDString(targetEntity.ID)
				if err != nil {
					slog.Debug("failed to get target ID for extracted entity", "name", name, "error", err)
					continue
				}
				relSource := string(models.RelationSourceAIDetected)

				if err := s.db.CreateRelation(ctx, models.RelationInput{
					FromID:  entityID,
					ToID:    targetID,
					RelType: "mentions",
					Source:  &relSource,
				}); err != nil {
					slog.Warn("failed to create mentions relation from graph extraction", "entity", entityID, "target", targetID, "error", err)
				}
			}
		}
	}

	return nil
}

// CollectFiles walks a directory and returns all markdown files.
func (s *IngestService) CollectFiles(dirPath string, recursive bool) ([]string, error) {
	var files []string
	walkFn := func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && !recursive && path != dirPath {
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
	return files, nil
}

// IngestDirectory ingests all Markdown files from a directory (synchronous).
func (s *IngestService) IngestDirectory(ctx context.Context, dirPath string, opts IngestOptions) (*IngestResult, error) {
	files, err := s.CollectFiles(dirPath, opts.Recursive)
	if err != nil {
		return nil, err
	}
	// Compute baseDir from directory path for unique entity IDs
	opts.BaseDir = filepath.Base(filepath.Clean(dirPath))
	return s.processFilesInternal(ctx, nil, nil, files, len(files), opts)
}

// IngestFilesWithContent ingests multiple files with provided content (not from disk).
// Used by the two-phase hash-based ingestion flow after checkHashes.
// baseDir is used to compute unique entity IDs from relative file paths.
func (s *IngestService) IngestFilesWithContent(ctx context.Context, files []FileContent, baseDir string, opts IngestOptions) (*IngestResult, error) {
	if len(files) == 0 {
		return &IngestResult{}, nil
	}

	slog.Info("starting content-based file processing", "files", len(files), "base_dir", baseDir, "extract_graph", opts.ExtractGraph)

	// Set default concurrency
	concurrency := opts.Concurrency
	if concurrency <= 0 {
		concurrency = 4
	}

	// Result aggregation with thread-safe counters
	var (
		filesProcessed  atomic.Int32
		entitiesCreated atomic.Int32
		chunksCreated   atomic.Int32
		errorsMu        sync.Mutex
		errors          []string
	)

	// Worker pool - use struct to pass both path and content
	type workItem struct {
		path    string
		content string
		hash    string
		baseDir string
	}
	workChan := make(chan workItem, len(files))
	var wg sync.WaitGroup

	// Start workers
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for item := range workChan {
				if ctx.Err() != nil {
					return
				}

				processed := filesProcessed.Add(1)
				slog.Info("processing file", "worker", workerID, "file", filepath.Base(item.path), "progress", fmt.Sprintf("%d/%d", processed, len(files)))

				result, err := s.IngestFileWithContent(ctx, item.path, item.content, item.hash, item.baseDir, opts)
				if err != nil {
					errorsMu.Lock()
					errors = append(errors, fmt.Sprintf("%s: %v", item.path, err))
					errorsMu.Unlock()
					continue
				}

				entitiesCreated.Add(1)

				// Use chunk count from result (no extra DB query needed)
				if result != nil && !opts.DryRun {
					chunksCreated.Add(int32(result.ChunksCreated))
				}
			}
		}(i)
	}

	// Send files to workers
	for _, f := range files {
		workChan <- workItem{path: f.Path, content: f.Content, hash: f.Hash, baseDir: baseDir}
	}
	close(workChan)

	// Wait for completion
	wg.Wait()

	slog.Info("content-based processing complete", "entities", entitiesCreated.Load(), "chunks", chunksCreated.Load(), "errors", len(errors))

	return &IngestResult{
		FilesProcessed:  int(filesProcessed.Load()),
		EntitiesCreated: int(entitiesCreated.Load()),
		ChunksCreated:   int(chunksCreated.Load()),
		Errors:          errors,
	}, nil
}

// ProcessFiles processes a list of files with job manager integration.
// Used for both new jobs and resumed jobs.
func (s *IngestService) ProcessFiles(ctx context.Context, jobManager *JobManager, job *Job, files []string, opts IngestOptions) (*IngestResult, error) {
	totalFiles := job.Total // Use original total for progress calculation
	return s.processFilesInternal(ctx, jobManager, job, files, totalFiles, opts)
}

// processFilesInternal is the core file processing logic.
func (s *IngestService) processFilesInternal(ctx context.Context, jobManager *JobManager, job *Job, files []string, totalFiles int, opts IngestOptions) (*IngestResult, error) {
	slog.Info("starting file processing", "files", len(files), "total", totalFiles, "concurrency", opts.Concurrency, "extract_graph", opts.ExtractGraph)

	// Set default concurrency
	concurrency := opts.Concurrency
	if concurrency <= 0 {
		concurrency = 4
	}

	// Calculate starting progress (for resumed jobs)
	startProgress := totalFiles - len(files)

	// Result aggregation with thread-safe counters
	var (
		filesProcessed  atomic.Int32
		entitiesCreated atomic.Int32
		chunksCreated   atomic.Int32
		errorsMu        sync.Mutex
		errors          []string
	)

	// Worker pool
	fileChan := make(chan string, len(files))
	var wg sync.WaitGroup

	// Start workers
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for file := range fileChan {
				if ctx.Err() != nil {
					return
				}

				processed := filesProcessed.Add(1)
				currentProgress := startProgress + int(processed)
				slog.Info("processing file", "worker", workerID, "file", filepath.Base(file), "progress", fmt.Sprintf("%d/%d", currentProgress, totalFiles))

				// Update job progress via job manager (handles DB persistence)
				if jobManager != nil && job != nil {
					jobManager.UpdateProgress(ctx, job, currentProgress, totalFiles)
				}

				result, err := s.IngestFile(ctx, file, opts)
				if err != nil {
					errorsMu.Lock()
					errors = append(errors, fmt.Sprintf("%s: %v", file, err))
					errorsMu.Unlock()
					continue
				}

				entitiesCreated.Add(1)

				// Use chunk count from result (no extra DB query needed)
				if result != nil && !opts.DryRun {
					chunksCreated.Add(int32(result.ChunksCreated))
				}
			}
		}(i)
	}

	// Send files to workers
	for _, file := range files {
		fileChan <- file
	}
	close(fileChan)

	// Wait for completion
	wg.Wait()

	slog.Info("file processing complete", "entities", entitiesCreated.Load(), "chunks", chunksCreated.Load(), "errors", len(errors))

	return &IngestResult{
		FilesProcessed:  int(filesProcessed.Load()),
		EntitiesCreated: int(entitiesCreated.Load()),
		ChunksCreated:   int(chunksCreated.Load()),
		Errors:          errors,
	}, nil
}

// IngestFilesWithContentAsync starts an async job for content-based file ingestion.
// Unlike IngestDirectoryAsync, files are provided with their content (not paths to read from disk).
// This is used for the two-phase hash-based ingestion flow where the client uploads file content.
// baseDir is used to compute unique entity IDs from relative file paths.
func (s *IngestService) IngestFilesWithContentAsync(ctx context.Context, jobManager *JobManager, files []FileContent, baseDir string, opts IngestOptions) (*Job, error) {
	if len(files) == 0 {
		return nil, fmt.Errorf("no files to ingest")
	}

	// Extract file paths for job tracking
	filePaths := make([]string, len(files))
	for i, f := range files {
		filePaths[i] = f.Path
	}

	// Prepare options for persistence (excluding name and labels which are now top-level)
	persistOpts := map[string]any{
		"extract_graph": opts.ExtractGraph,
		"content_based": true, // Mark as content-based job
		"base_dir":      baseDir,
	}

	// Create job with persistence (using first file's directory as dirPath for display)
	dirPath := filepath.Dir(files[0].Path)
	job, err := jobManager.CreateJob(ctx, "ingest", opts.Name, dirPath, filePaths, opts.Labels, persistOpts)
	if err != nil {
		return nil, fmt.Errorf("create job: %w", err)
	}

	// Set concurrency from job manager
	opts.Concurrency = jobManager.Concurrency()
	opts.Job = job

	// Start processing in background with detached context
	go func() {
		bgCtx := context.Background()

		// Mark as running
		jobManager.SetRunning(bgCtx, job)

		result, err := s.processFilesWithContentInternal(bgCtx, jobManager, job, files, baseDir, opts)
		if err != nil {
			jobManager.Fail(bgCtx, job, err)
			return
		}
		jobManager.Complete(bgCtx, job, result)
	}()

	return job, nil
}

// processFilesWithContentInternal processes files from provided content with job tracking.
func (s *IngestService) processFilesWithContentInternal(ctx context.Context, jobManager *JobManager, job *Job, files []FileContent, baseDir string, opts IngestOptions) (*IngestResult, error) {
	slog.Info("starting async content-based file processing", "files", len(files), "extract_graph", opts.ExtractGraph)

	// Set default concurrency
	concurrency := opts.Concurrency
	if concurrency <= 0 {
		concurrency = 4
	}

	totalFiles := len(files)

	// Result aggregation with thread-safe counters
	var (
		filesProcessed  atomic.Int32
		entitiesCreated atomic.Int32
		chunksCreated   atomic.Int32
		errorsMu        sync.Mutex
		errors          []string
	)

	// Worker pool
	type workItem struct {
		path    string
		content string
		hash    string
		baseDir string
	}
	workChan := make(chan workItem, len(files))
	var wg sync.WaitGroup

	// Start workers
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for item := range workChan {
				if ctx.Err() != nil {
					return
				}

				processed := filesProcessed.Add(1)
				slog.Info("processing file", "worker", workerID, "file", filepath.Base(item.path), "progress", fmt.Sprintf("%d/%d", processed, totalFiles))

				// Update job progress via job manager
				if jobManager != nil && job != nil {
					jobManager.UpdateProgress(ctx, job, int(processed), totalFiles)
				}

				result, err := s.IngestFileWithContent(ctx, item.path, item.content, item.hash, item.baseDir, opts)
				if err != nil {
					errorsMu.Lock()
					errors = append(errors, fmt.Sprintf("%s: %v", item.path, err))
					errorsMu.Unlock()
					continue
				}

				entitiesCreated.Add(1)

				// Use chunk count from result (no extra DB query needed)
				if result != nil && !opts.DryRun {
					chunksCreated.Add(int32(result.ChunksCreated))
				}
			}
		}(i)
	}

	// Send files to workers
	for _, f := range files {
		workChan <- workItem{path: f.Path, content: f.Content, hash: f.Hash, baseDir: baseDir}
	}
	close(workChan)

	// Wait for completion
	wg.Wait()

	slog.Info("async content-based processing complete", "entities", entitiesCreated.Load(), "chunks", chunksCreated.Load(), "errors", len(errors))

	return &IngestResult{
		FilesProcessed:  int(filesProcessed.Load()),
		EntitiesCreated: int(entitiesCreated.Load()),
		ChunksCreated:   int(chunksCreated.Load()),
		Errors:          errors,
	}, nil
}

// IngestDirectoryAsync starts an async ingestion job with persistence.
func (s *IngestService) IngestDirectoryAsync(ctx context.Context, jobManager *JobManager, dirPath string, opts IngestOptions) (*Job, error) {
	// Validate path exists before starting job
	info, err := os.Stat(dirPath)
	if err != nil {
		return nil, fmt.Errorf("invalid path: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("path must be a directory: %s", dirPath)
	}

	// Collect files upfront (deterministic list for resume)
	files, err := s.CollectFiles(dirPath, opts.Recursive)
	if err != nil {
		return nil, err
	}

	if len(files) == 0 {
		return nil, fmt.Errorf("no markdown files found in %s", dirPath)
	}

	// Compute baseDir from directory path for unique entity IDs
	baseDir := filepath.Base(filepath.Clean(dirPath))

	// Prepare options for persistence (excluding name and labels which are now top-level)
	persistOpts := map[string]any{
		"extract_graph": opts.ExtractGraph,
		"recursive":     opts.Recursive,
		"base_dir":      baseDir,
	}

	// Create job with persistence
	job, err := jobManager.CreateJob(ctx, "ingest", opts.Name, dirPath, files, opts.Labels, persistOpts)
	if err != nil {
		return nil, fmt.Errorf("create job: %w", err)
	}

	// Set concurrency and baseDir from job manager
	opts.Concurrency = jobManager.Concurrency()
	opts.BaseDir = baseDir

	// Start processing in background
	go func() {
		bgCtx := context.Background()

		// Mark as running
		jobManager.SetRunning(bgCtx, job)

		result, err := s.ProcessFiles(bgCtx, jobManager, job, files, opts)
		if err != nil {
			jobManager.Fail(bgCtx, job, err)
			return
		}
		jobManager.Complete(bgCtx, job, result)
	}()

	return job, nil
}

// slugify converts a name to a URL-safe ID.
func slugify(name string) string {
	// Simple slugification: lowercase, replace spaces with hyphens
	s := strings.ToLower(name)
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.ReplaceAll(s, "_", "-")
	// Remove non-alphanumeric except hyphens
	result := strings.Builder{}
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			result.WriteRune(r)
		}
	}
	return result.String()
}
