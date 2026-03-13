package memory

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/raphi011/knowhow/internal/models"
)

func TestComputeScore(t *testing.T) {
	now := time.Now()
	halfLife := 30

	tests := []struct {
		name    string
		doc     models.Document
		wantMin float64
		wantMax float64
	}{
		{
			name: "fresh, never re-accessed",
			doc: models.Document{
				CreatedAt:   now,
				AccessCount: 1,
			},
			wantMin: 1.0,
			wantMax: 1.2,
		},
		{
			name: "30 days old, accessed 3x",
			doc: models.Document{
				LastAccessedAt: new(now.Add(-30 * 24 * time.Hour)),
				AccessCount:    3,
				CreatedAt:      now.Add(-60 * 24 * time.Hour),
			},
			wantMin: 0.6,
			wantMax: 0.7,
		},
		{
			name: "60 days old, accessed once",
			doc: models.Document{
				LastAccessedAt: new(now.Add(-60 * 24 * time.Hour)),
				AccessCount:    1,
				CreatedAt:      now.Add(-60 * 24 * time.Hour),
			},
			wantMin: 0.2,
			wantMax: 0.3,
		},
		{
			name: "90 days old, accessed once - should be below archive threshold",
			doc: models.Document{
				LastAccessedAt: new(now.Add(-90 * 24 * time.Hour)),
				AccessCount:    1,
				CreatedAt:      now.Add(-90 * 24 * time.Hour),
			},
			wantMin: 0.1,
			wantMax: 0.2,
		},
		{
			name: "60 days old, accessed 5x",
			doc: models.Document{
				LastAccessedAt: new(now.Add(-60 * 24 * time.Hour)),
				AccessCount:    5,
				CreatedAt:      now.Add(-60 * 24 * time.Hour),
			},
			wantMin: 0.6,
			wantMax: 0.7,
		},
		{
			name: "access boost capped at 2.0",
			doc: models.Document{
				LastAccessedAt: new(now),
				AccessCount:    100,
				CreatedAt:      now,
			},
			wantMin: 2.9,
			wantMax: 3.1,
		},
		{
			name: "uses created_at when last_accessed_at is nil",
			doc: models.Document{
				CreatedAt:   now.Add(-30 * 24 * time.Hour),
				AccessCount: 0,
			},
			wantMin: 0.3,
			wantMax: 0.4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := ComputeScore(tt.doc, now, halfLife)
			if score < tt.wantMin || score > tt.wantMax {
				t.Errorf("ComputeScore() = %f, want between %f and %f", score, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestComputeScore_ArchiveThreshold(t *testing.T) {
	now := time.Now()
	halfLife := 30
	archiveThreshold := 0.2

	// Memory accessed once, ~70 days ago should be below the threshold
	doc := models.Document{
		LastAccessedAt: new(now.Add(-70 * 24 * time.Hour)),
		AccessCount:    1,
		CreatedAt:      now.Add(-70 * 24 * time.Hour),
	}

	score := ComputeScore(doc, now, halfLife)
	if score >= archiveThreshold {
		t.Errorf("expected score < %f for 70-day-old memory accessed once, got %f", archiveThreshold, score)
	}

	// Memory accessed 5 times, ~70 days ago should still be above threshold
	doc.AccessCount = 5
	score = ComputeScore(doc, now, halfLife)
	if score < archiveThreshold {
		t.Errorf("expected score >= %f for 70-day-old memory accessed 5x, got %f", archiveThreshold, score)
	}
}

func TestBuildMemoryDocument(t *testing.T) {
	settings := models.VaultSettings{
		MemoryPath: "/memories",
	}

	path, content := BuildMemoryDocument("knowhow", "API Design Notes", "Some content here", nil, settings)

	// Check path format
	if !contains(path, "/memories/knowhow/") {
		t.Errorf("path should contain /memories/knowhow/, got: %s", path)
	}
	if !contains(path, "-api-design-notes.md") {
		t.Errorf("path should contain slugified title, got: %s", path)
	}

	// Check labels in frontmatter
	if !contains(content, `"memory"`) {
		t.Error("content should contain memory label")
	}
	if !contains(content, `"project/knowhow"`) {
		t.Error("content should contain project label")
	}
	if !contains(content, "Some content here") {
		t.Error("content should contain the body")
	}
}

func TestBuildMemoryDocument_GlobalMemory(t *testing.T) {
	settings := models.VaultSettings{
		MemoryPath: "/memories",
	}

	path, content := BuildMemoryDocument("", "Go Concurrency Patterns", "Some content", []string{"golang"}, settings)

	// Global memory: no project subfolder
	if contains(path, "/memories/go-concurrency") {
		// path should be /memories/YYYY-MM-DD-go-concurrency-patterns.md (no project dir)
	}
	if contains(path, "/memories//") {
		t.Errorf("path should not contain double slash, got: %s", path)
	}
	if !contains(path, "-go-concurrency-patterns.md") {
		t.Errorf("path should contain slugified title, got: %s", path)
	}

	// Should NOT contain project label
	if contains(content, `"project/`) {
		t.Error("global memory should not contain project label")
	}
	// Should contain memory label
	if !contains(content, `"memory"`) {
		t.Error("content should contain memory label")
	}
	// Should contain extra label
	if !contains(content, `"golang"`) {
		t.Error("content should contain golang label")
	}
}

func TestBuildMemoryDocument_CustomPath(t *testing.T) {
	settings := models.VaultSettings{
		MemoryPath: "/agent-memories",
	}

	path, _ := BuildMemoryDocument("my-project", "Test", "content", nil, settings)

	if !contains(path, "/agent-memories/my-project/") {
		t.Errorf("path should use custom memory path, got: %s", path)
	}
}

func TestBuildMemoryDocument_ExtraLabels(t *testing.T) {
	settings := models.VaultSettings{
		MemoryPath: "/memories",
	}

	_, content := BuildMemoryDocument("proj", "Test", "content", []string{"golang", "memory"}, settings)

	// "memory" should not be duplicated
	if count := countOccurrences(content, `"memory"`); count != 1 {
		t.Errorf("memory label should appear exactly once, got %d times", count)
	}
	if !contains(content, `"golang"`) {
		t.Error("content should contain extra golang label")
	}
}

func TestProjectLabel(t *testing.T) {
	if got := projectLabel("Knowhow"); got != "project/knowhow" {
		t.Errorf("projectLabel(Knowhow) = %q, want project/knowhow", got)
	}
	if got := projectLabel("  My Project  "); got != "project/my project" {
		t.Errorf("projectLabel with spaces = %q", got)
	}
	// Empty project still produces "project/" - callers should skip calling this for empty project
	if got := projectLabel(""); got != "project/" {
		t.Errorf("projectLabel empty = %q, want project/", got)
	}
}

func TestCosineSimilarity(t *testing.T) {
	// Identical vectors
	a := []float32{1, 0, 0}
	sim := cosineSimilarity(a, a)
	if math.Abs(sim-1.0) > 0.001 {
		t.Errorf("identical vectors: got %f, want 1.0", sim)
	}

	// Orthogonal vectors
	b := []float32{0, 1, 0}
	sim = cosineSimilarity(a, b)
	if math.Abs(sim) > 0.001 {
		t.Errorf("orthogonal vectors: got %f, want 0.0", sim)
	}

	// Different lengths
	sim = cosineSimilarity([]float32{1, 0}, []float32{1, 0, 0})
	if sim != 0 {
		t.Errorf("different lengths: got %f, want 0.0", sim)
	}

	// Empty vectors
	sim = cosineSimilarity(nil, nil)
	if sim != 0 {
		t.Errorf("nil vectors: got %f, want 0.0", sim)
	}
}

func TestHasLabel(t *testing.T) {
	labels := []string{"memory", "project/knowhow", "golang"}
	if !hasLabel(labels, "memory") {
		t.Error("should find memory label")
	}
	if hasLabel(labels, "missing") {
		t.Error("should not find missing label")
	}
}

func TestMergeLabels(t *testing.T) {
	a := []string{"memory", "project/knowhow"}
	b := []string{"memory", "golang"}
	result := mergeLabels(a, b)

	if len(result) != 3 {
		t.Errorf("expected 3 labels, got %d: %v", len(result), result)
	}
	if !hasLabel(result, "golang") {
		t.Error("should contain golang")
	}
}

func TestExtractProject(t *testing.T) {
	labels := []string{"memory", "project/knowhow", "golang"}
	if got := extractProject(labels); got != "knowhow" {
		t.Errorf("extractProject = %q, want knowhow", got)
	}
	if got := extractProject([]string{"memory"}); got != "" {
		t.Errorf("extractProject without project label = %q, want empty", got)
	}
}

//go:fix inline
func timePtr(t time.Time) *time.Time {
	return new(t)
}

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

func countOccurrences(s, substr string) int {
	return strings.Count(s, substr)
}
