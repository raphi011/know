// Package memory provides persistent memory management with decay,
// auto-archiving, and LLM-powered consolidation. Memories can be
// scoped to a project or stored globally with label-based filtering.
package memory

import (
	"context"
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/raphi011/know/internal/db"
	"github.com/raphi011/know/internal/file"
	"github.com/raphi011/know/internal/llm"
	"github.com/raphi011/know/internal/logutil"
	"github.com/raphi011/know/internal/models"
	"github.com/raphi011/know/internal/pathutil"
)

// ScoredMemory is a memory document with its computed relevance score.
type ScoredMemory struct {
	Document models.File `json:"document"`
	Score    float64     `json:"score"`
}

// Service manages project-scoped memories with decay, archiving, and consolidation.
type Service struct {
	db         *db.Client
	docService *file.Service
	model      *llm.Model
}

// NewService creates a new memory service. model may be nil to disable consolidation.
// Panics if dbClient or docService are nil.
func NewService(dbClient *db.Client, docService *file.Service, model *llm.Model) *Service {
	if dbClient == nil {
		panic("memory.NewService: dbClient must not be nil")
	}
	if docService == nil {
		panic("memory.NewService: docService must not be nil")
	}
	return &Service{
		db:         dbClient,
		docService: docService,
		model:      model,
	}
}

// Create stores a memory document, optionally scoped to a project.
// If project is empty, the memory is global (stored directly under the memory path).
func (s *Service) Create(ctx context.Context, vaultID, project, title, content string, labels []string, settings models.VaultSettings) (*models.File, error) {
	path, fullContent := BuildMemoryDocument(project, title, content, labels, settings)

	doc, err := s.docService.Create(ctx, models.FileInput{
		VaultID: vaultID,
		Path:    path,
		Content: fullContent,
	})
	if err != nil {
		return nil, fmt.Errorf("create: %w", err)
	}
	return doc, nil
}

// Retrieve returns memories sorted by relevance score, with auto-archiving
// and optional consolidation. If project is empty, returns all memories
// (optionally filtered by extraLabels). If project is set, returns only
// memories for that project.
func (s *Service) Retrieve(ctx context.Context, vaultID, project string, extraLabels []string, includeArchived bool, settings models.VaultSettings) ([]ScoredMemory, error) {
	logger := logutil.FromCtx(ctx)

	// Build label filter
	labels := []string{"memory"}
	if project != "" {
		labels = append(labels, projectLabel(project))
	}
	for _, l := range extraLabels {
		labels = appendUnique(labels, l)
	}

	docs, err := s.db.GetFilesByAllLabels(ctx, vaultID, labels)
	if err != nil {
		return nil, fmt.Errorf("retrieve: %w", err)
	}

	now := time.Now()
	var active []ScoredMemory
	var toArchive []models.File

	for _, doc := range docs {
		isArchived := hasLabel(doc.Labels, "archived")
		score := ComputeScore(doc, now, settings.MemoryDecayHalfLife)

		if score < settings.MemoryArchiveThreshold && !isArchived {
			toArchive = append(toArchive, doc)
			if !includeArchived {
				continue
			}
		}

		if isArchived && !includeArchived {
			continue
		}

		active = append(active, ScoredMemory{Document: doc, Score: score})
	}

	// Auto-archive low-scoring memories
	for _, doc := range toArchive {
		docID, err := models.RecordIDString(doc.ID)
		if err != nil {
			logger.Warn("archive: extract id", "error", err)
			continue
		}
		vID, err := models.RecordIDString(doc.Vault)
		if err != nil {
			logger.Warn("archive: extract vault id", "error", err)
			continue
		}
		newLabels := appendUnique(doc.Labels, "archived")
		if err := s.db.SyncFileLabels(ctx, docID, vID, newLabels); err != nil {
			logger.Warn("archive: sync labels", "doc_id", docID, "vault_id", vID, "path", doc.Path, "error", err)
		}
	}

	// Consolidation pass (requires LLM)
	if s.model != nil && len(active) > 1 {
		consolidated, err := s.consolidate(ctx, vaultID, active, settings)
		if err != nil {
			logger.Warn("consolidation failed, returning unconsolidated", "error", err)
		} else {
			active = consolidated
		}
	}

	// Sort by score descending
	sort.Slice(active, func(i, j int) bool {
		return active[i].Score > active[j].Score
	})

	// Update access tracking for returned memories
	for _, m := range active {
		docID, err := models.RecordIDString(m.Document.ID)
		if err != nil {
			logger.Warn("update access: extract id", "path", m.Document.Path, "error", err)
			continue
		}
		if err := s.db.UpdateFileAccess(ctx, docID); err != nil {
			logger.Warn("update access", "path", m.Document.Path, "error", err)
		}
	}

	return active, nil
}

// Delete removes a memory document by path.
func (s *Service) Delete(ctx context.Context, vaultID, path string) error {
	path = models.NormalizePath(path)
	if err := s.docService.Delete(ctx, vaultID, path); err != nil {
		return fmt.Errorf("delete: %w", err)
	}
	return nil
}

// consolidate finds pairs of memories with high embedding similarity and merges them.
func (s *Service) consolidate(ctx context.Context, vaultID string, memories []ScoredMemory, settings models.VaultSettings) ([]ScoredMemory, error) {
	logger := logutil.FromCtx(ctx)

	// Collect first-chunk embeddings for each memory
	type memEmbed struct {
		idx       int
		embedding []float32
	}
	var embeds []memEmbed

	for i, m := range memories {
		docID, err := models.RecordIDString(m.Document.ID)
		if err != nil {
			logger.Warn("consolidate: extract id", "path", m.Document.Path, "error", err)
			continue
		}
		chunks, err := s.db.GetChunks(ctx, docID)
		if err != nil {
			logger.Warn("consolidate: get chunks", "path", m.Document.Path, "error", err)
			continue
		}
		if len(chunks) == 0 {
			continue
		}
		if chunks[0].Embedding != nil {
			embeds = append(embeds, memEmbed{idx: i, embedding: chunks[0].Embedding})
		}
	}

	if len(embeds) < 2 {
		return memories, nil
	}

	// Find merge candidates (pairwise similarity above threshold)
	type mergePair struct {
		i, j int // indices into memories slice
	}
	var pairs []mergePair
	merged := map[int]bool{}

	for a := 0; a < len(embeds); a++ {
		if merged[embeds[a].idx] {
			continue
		}
		for b := a + 1; b < len(embeds); b++ {
			if merged[embeds[b].idx] {
				continue
			}
			sim := cosineSimilarity(embeds[a].embedding, embeds[b].embedding)
			if sim >= settings.MemoryMergeThreshold {
				pairs = append(pairs, mergePair{embeds[a].idx, embeds[b].idx})
				merged[embeds[a].idx] = true
				merged[embeds[b].idx] = true
				break // each memory merges at most once per pass
			}
		}
	}

	if len(pairs) == 0 {
		return memories, nil
	}

	// Merge each pair via LLM
	removed := map[int]bool{}
	for _, pair := range pairs {
		m1 := memories[pair.i].Document
		m2 := memories[pair.j].Document

		mergedContent, err := s.model.GenerateWithSystem(ctx,
			"You are a memory consolidation agent. Merge these two overlapping memories into a single, concise memory that preserves all unique information. Output only the merged memory content in markdown, no frontmatter.",
			fmt.Sprintf("Memory 1 (title: %s):\n%s\n\nMemory 2 (title: %s):\n%s", m1.Title, m1.Content, m2.Title, m2.Content),
		)
		if err != nil {
			logger.Warn("consolidation LLM merge failed", "error", err)
			continue
		}

		// Create merged memory
		combinedLabels := mergeLabels(m1.Labels, m2.Labels)
		combinedLabels = removeLabel(combinedLabels, "archived")
		mergedTitle := m1.Title // keep the first title
		combinedAccess := m1.AccessCount + m2.AccessCount

		path, fullContent := BuildMemoryDocument(
			extractProject(m1.Labels),
			mergedTitle,
			mergedContent,
			combinedLabels,
			settings,
		)

		doc, err := s.docService.Create(ctx, models.FileInput{
			VaultID: vaultID,
			Path:    path,
			Content: fullContent,
		})
		if err != nil {
			logger.Warn("consolidation create merged doc", "error", err)
			continue
		}

		// Set access count on the merged document
		if combinedAccess > 0 {
			docID, err := models.RecordIDString(doc.ID)
			if err != nil {
				logger.Warn("consolidation: extract merged doc id", "path", doc.Path, "error", err)
			} else if err := s.db.SetFileAccessCount(ctx, docID, combinedAccess); err != nil {
				logger.Warn("consolidation: set access count", "path", doc.Path, "error", err)
			}
		}

		// Delete originals
		for _, orig := range []models.File{m1, m2} {
			origVaultID, err := models.RecordIDString(orig.Vault)
			if err != nil {
				logger.Warn("consolidation: extract vault id for delete", "path", orig.Path, "error", err)
				continue
			}
			if err := s.docService.Delete(ctx, origVaultID, orig.Path); err != nil {
				logger.Warn("consolidation delete original", "path", orig.Path, "error", err)
			}
		}

		// Update the active list: replace first with merged doc, mark second for removal
		memories[pair.i] = ScoredMemory{
			Document: *doc,
			Score:    ComputeScore(*doc, time.Now(), settings.MemoryDecayHalfLife),
		}
		removed[pair.j] = true
	}

	// Filter out removed entries
	var result []ScoredMemory
	for i, m := range memories {
		if !removed[i] {
			result = append(result, m)
		}
	}
	return result, nil
}

// ComputeScore calculates the relevance score for a memory document.
// score = recency + access_boost where:
//   - recency = e^(-age_days / half_life) based on last_accessed_at
//   - access_boost = min(access_count * 0.1, 2.0)
func ComputeScore(doc models.File, now time.Time, halfLifeDays int) float64 {
	// Use last_accessed_at if available, otherwise fall back to created_at
	refTime := doc.CreatedAt
	if doc.LastAccessedAt != nil {
		refTime = *doc.LastAccessedAt
	}

	ageDays := now.Sub(refTime).Hours() / 24
	if ageDays < 0 {
		ageDays = 0
	}

	if halfLifeDays <= 0 {
		halfLifeDays = 30
	}

	recency := math.Exp(-ageDays / float64(halfLifeDays))
	accessBoost := math.Min(float64(doc.AccessCount)*0.1, 2.0)

	return recency + accessBoost
}

// BuildMemoryDocument builds a memory document's path and full content (with frontmatter).
// The "memory" label is always included. If project is non-empty, "project/{project}" is
// added and the path includes a project subfolder.
func BuildMemoryDocument(project, title, content string, extraLabels []string, settings models.VaultSettings) (path, fullContent string) {
	labels := []string{"memory"}
	if project != "" {
		labels = append(labels, projectLabel(project))
	}
	for _, l := range extraLabels {
		labels = appendUnique(labels, l)
	}

	var sb strings.Builder
	sb.WriteString("---\nlabels:\n")
	for _, l := range labels {
		fmt.Fprintf(&sb, "  - %q\n", l)
	}
	sb.WriteString("---\n\n")
	sb.WriteString(content)

	slug := pathutil.Slugify(title)
	date := time.Now().Format("2006-01-02")
	memoryPath := settings.MemoryPath
	if memoryPath == "" {
		memoryPath = "/memories"
	}
	if project != "" {
		projectSlug := pathutil.Slugify(project)
		path = fmt.Sprintf("%s/%s/%s-%s.md", memoryPath, projectSlug, date, slug)
	} else {
		path = fmt.Sprintf("%s/%s-%s.md", memoryPath, date, slug)
	}

	return path, sb.String()
}

func projectLabel(project string) string {
	return "project/" + strings.ToLower(strings.TrimSpace(project))
}

func extractProject(labels []string) string {
	for _, l := range labels {
		if after, ok := strings.CutPrefix(l, "project/"); ok {
			return after
		}
	}
	return ""
}

func hasLabel(labels []string, name string) bool {
	return slices.Contains(labels, name)
}

func appendUnique(labels []string, name string) []string {
	if hasLabel(labels, name) {
		return labels
	}
	return append(labels, name)
}

func removeLabel(labels []string, name string) []string {
	var result []string
	for _, l := range labels {
		if l != name {
			result = append(result, l)
		}
	}
	return result
}

func mergeLabels(a, b []string) []string {
	result := make([]string, len(a))
	copy(result, a)
	for _, l := range b {
		result = appendUnique(result, l)
	}
	return result
}

func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	denom := math.Sqrt(normA) * math.Sqrt(normB)
	if denom == 0 {
		return 0
	}
	return dot / denom
}
