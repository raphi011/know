package render

import (
	"strings"
	"testing"

	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"

	"github.com/raphi011/know/internal/db"
	"github.com/raphi011/know/internal/models"
	"github.com/raphi011/know/internal/parser"
)

func testRecordID(table, id string) surrealmodels.RecordID {
	return surrealmodels.RecordID{Table: table, ID: id}
}

func TestFindFencedBlockEnd(t *testing.T) {
	tests := []struct {
		name    string
		content string
		offset  int
		want    int
	}{
		{
			name:    "simple block",
			content: "before\n```know\nFROM /daily\n```\nafter",
			offset:  7,
			want:    31, // past "```\n"
		},
		{
			name:    "block at start",
			content: "```know\nFROM /daily\n```\n",
			offset:  0,
			want:    24, // past "```\n" (includes trailing newline)
		},
		{
			name:    "no closing fence",
			content: "```know\nFROM /daily\n",
			offset:  0,
			want:    -1,
		},
		{
			name:    "indented closing fence",
			content: "```know\nFROM /daily\n  ```  \nafter",
			offset:  0,
			want:    28, // past "  ```  \n"
		},
		{
			name:    "tilde fence",
			content: "~~~know\nFROM /daily\n~~~\nafter",
			offset:  0,
			want:    24, // past "~~~\n"
		},
		{
			name:    "tilde fence does not close with backticks",
			content: "~~~know\nFROM /daily\n```\n~~~\nafter",
			offset:  0,
			want:    28, // past "~~~\n", skipping the ``` line
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := findFencedBlockEnd(tt.content, tt.offset)
			if got != tt.want {
				t.Errorf("findFencedBlockEnd() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestWikiLinkPattern(t *testing.T) {
	tests := []struct {
		input string
		want  []string // expected full matches
	}{
		{"See [[Target Page]]", []string{"[[Target Page]]"}},
		{"[[A]] and [[B]]", []string{"[[A]]", "[[B]]"}},
		{"[[Target|alias]]", []string{"[[Target|alias]]"}},
		{"no links here", nil},
		{"[[]]", nil}, // empty target
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			matches := wikiLinkPattern.FindAllString(tt.input, -1)
			if len(matches) != len(tt.want) {
				t.Errorf("got %d matches, want %d: %v", len(matches), len(tt.want), matches)
				return
			}
			for i, m := range matches {
				if m != tt.want[i] {
					t.Errorf("match[%d] = %q, want %q", i, m, tt.want[i])
				}
			}
		})
	}
}

func TestRenderList(t *testing.T) {
	files := []struct {
		title string
		path  string
	}{
		{"My Note", "/notes/my-note.md"},
		{"Other", "/notes/other.md"},
	}

	// Construct models.File slice
	var modelFiles []models.File
	for _, f := range files {
		modelFiles = append(modelFiles, models.File{Title: f.title, Path: f.path})
	}

	result := renderList(modelFiles, nil, false)
	if !strings.Contains(result, "[My Note](/notes/my-note.md)") {
		t.Errorf("expected link to My Note, got: %s", result)
	}
	if !strings.Contains(result, "[Other](/notes/other.md)") {
		t.Errorf("expected link to Other, got: %s", result)
	}
}

func TestRenderList_WithoutID(t *testing.T) {
	modelFiles := []models.File{
		{Title: "My Note", Path: "/notes/my-note.md"},
		{Title: "Other", Path: "/notes/other.md"},
	}

	result := renderList(modelFiles, []parser.ShowField{{Name: "title"}}, true)
	if !strings.Contains(result, "- My Note") {
		t.Errorf("expected plain text 'My Note', got: %s", result)
	}
	if strings.Contains(result, "[My Note]") {
		t.Errorf("expected no link, got: %s", result)
	}
}

func TestRenderList_WithExtraField(t *testing.T) {
	modelFiles := []models.File{
		{Title: "Project A", Path: "/a.md", Labels: []string{"go", "api"}},
	}

	fields := []parser.ShowField{{Name: "title"}, {Name: "labels"}}
	result := renderList(modelFiles, fields, false)
	if !strings.Contains(result, "[Project A](/a.md)") {
		t.Errorf("expected link, got: %s", result)
	}
	if !strings.Contains(result, "go, api") {
		t.Errorf("expected labels as extra field, got: %s", result)
	}
}

func TestRenderTable(t *testing.T) {
	modelFiles := []models.File{
		{Title: "A", Path: "/a.md", MimeType: "text/markdown"},
	}

	fields := []parser.ShowField{
		{Name: "title"},
		{Name: "path"},
		{Name: "mime_type"},
	}
	result := renderTable(modelFiles, fields, true)
	if !strings.Contains(result, "| title |") {
		t.Errorf("expected header row, got: %s", result)
	}
	if !strings.Contains(result, "| A |") {
		t.Errorf("expected data row, got: %s", result)
	}
}

func TestRenderTable_WithFileColumn(t *testing.T) {
	modelFiles := []models.File{
		{Title: "A", Path: "/a.md", MimeType: "text/markdown"},
	}

	fields := []parser.ShowField{{Name: "mime_type"}}
	result := renderTable(modelFiles, fields, false)
	if !strings.Contains(result, "| File |") {
		t.Errorf("expected File header column, got: %s", result)
	}
	if !strings.Contains(result, "[A](/a.md)") {
		t.Errorf("expected file link in row, got: %s", result)
	}
}

func TestRenderTable_WithAlias(t *testing.T) {
	modelFiles := []models.File{
		{Title: "A", Path: "/a.md", Labels: []string{"go"}},
	}

	fields := []parser.ShowField{{Name: "labels", Alias: "Tags"}}
	result := renderTable(modelFiles, fields, true)
	if !strings.Contains(result, "| Tags |") {
		t.Errorf("expected alias header 'Tags', got: %s", result)
	}
}

func TestRenderTaskList(t *testing.T) {
	due := "2025-03-28"
	tasks := []models.TaskWithDoc{
		{
			Task:    models.Task{ID: testRecordID("task", "t1"), Text: "Fix search", Status: models.TaskStatusOpen, DueDate: &due},
			DocPath: "/bugs/search.md",
		},
		{
			Task:    models.Task{ID: testRecordID("task", "t2"), Text: "Update docs", Status: models.TaskStatusDone},
			DocPath: "/ops/runbook.md",
		},
	}

	result := renderTaskList(tasks, false)
	if !strings.Contains(result, "- [ ] Fix search") {
		t.Errorf("expected open checkbox, got: %s", result)
	}
	if !strings.Contains(result, "(due: 2025-03-28)") {
		t.Errorf("expected due date, got: %s", result)
	}
	if !strings.Contains(result, "*/bugs/search.md*") {
		t.Errorf("expected doc path, got: %s", result)
	}
	if !strings.Contains(result, "- [x] Update docs") {
		t.Errorf("expected done checkbox, got: %s", result)
	}
	if !strings.Contains(result, "<!-- task:t1 -->") {
		t.Errorf("expected task ID comment for t1, got: %s", result)
	}
	if !strings.Contains(result, "<!-- task:t2 -->") {
		t.Errorf("expected task ID comment for t2, got: %s", result)
	}
}

func TestRenderTaskList_WithoutID(t *testing.T) {
	tasks := []models.TaskWithDoc{
		{
			Task:    models.Task{ID: testRecordID("task", "t1"), Text: "Fix search", Status: models.TaskStatusOpen},
			DocPath: "/bugs/search.md",
		},
	}

	result := renderTaskList(tasks, true)
	if strings.Contains(result, "/bugs/search.md") {
		t.Errorf("expected no doc path with withoutID=true, got: %s", result)
	}
	if !strings.Contains(result, "<!-- task:t1 -->") {
		t.Errorf("expected task ID comment even with withoutID=true, got: %s", result)
	}
}

func TestQueryBlockOrderBy(t *testing.T) {
	tests := []struct {
		name      string
		sortField string
		sortDesc  bool
		want      db.FileOrderBy
	}{
		{"default path", "path", false, db.OrderByPathAsc},
		{"updated_at desc", "updated_at", true, db.OrderByUpdatedAtDesc},
		{"updated_at asc", "updated_at", false, db.OrderByUpdatedAtAsc},
		{"created_at desc", "created_at", true, db.OrderByCreatedAtDesc},
		{"created_at asc falls back to path", "created_at", false, db.OrderByPathAsc},
		{"unknown field", "title", false, db.OrderByPathAsc},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			qb := parser.QueryBlock{SortField: tt.sortField, SortDesc: tt.sortDesc}
			got := queryBlockOrderBy(qb)
			if got != tt.want {
				t.Errorf("queryBlockOrderBy() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRenderTaskList_NoID(t *testing.T) {
	tasks := []models.TaskWithDoc{
		{Task: models.Task{Text: "No ID task", Status: models.TaskStatusOpen}, DocPath: "/test.md"},
	}

	result := renderTaskList(tasks, false)
	if strings.Contains(result, "<!-- task:") {
		t.Errorf("expected no task comment for zero-value ID, got: %s", result)
	}
}
