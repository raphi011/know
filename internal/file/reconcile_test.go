package file

import (
	"testing"

	"github.com/raphi011/know/internal/models"
)

func TestApplyTaskChanges(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		tasks    []models.Task
		expected string
	}{
		{
			name:    "single task open to done",
			content: "# Tasks\n- [ ] Buy milk\n- [ ] Walk dog\n",
			tasks: []models.Task{
				{Status: models.TaskStatusDone, ContentHash: taskContentHash("Buy milk"), LineNumber: 1, RawLine: "- [x] Buy milk"},
				{Status: models.TaskStatusOpen, ContentHash: taskContentHash("Walk dog"), LineNumber: 2, RawLine: "- [ ] Walk dog"},
			},
			expected: "# Tasks\n- [x] Buy milk\n- [ ] Walk dog\n",
		},
		{
			name:    "single task done to open",
			content: "# Tasks\n- [x] Buy milk\n",
			tasks: []models.Task{
				{Status: models.TaskStatusOpen, ContentHash: taskContentHash("Buy milk"), LineNumber: 1, RawLine: "- [ ] Buy milk"},
			},
			expected: "# Tasks\n- [ ] Buy milk\n",
		},
		{
			name:    "multiple toggles",
			content: "# Tasks\n- [ ] Task A\n- [x] Task B\n- [ ] Task C\n",
			tasks: []models.Task{
				{Status: models.TaskStatusDone, ContentHash: taskContentHash("Task A"), LineNumber: 1, RawLine: "- [x] Task A"},
				{Status: models.TaskStatusOpen, ContentHash: taskContentHash("Task B"), LineNumber: 2, RawLine: "- [ ] Task B"},
				{Status: models.TaskStatusDone, ContentHash: taskContentHash("Task C"), LineNumber: 3, RawLine: "- [x] Task C"},
			},
			expected: "# Tasks\n- [x] Task A\n- [ ] Task B\n- [x] Task C\n",
		},
		{
			name:    "no changes needed",
			content: "# Tasks\n- [x] Done task\n- [ ] Open task\n",
			tasks: []models.Task{
				{Status: models.TaskStatusDone, ContentHash: taskContentHash("Done task"), LineNumber: 1, RawLine: "- [x] Done task"},
				{Status: models.TaskStatusOpen, ContentHash: taskContentHash("Open task"), LineNumber: 2, RawLine: "- [ ] Open task"},
			},
			expected: "# Tasks\n- [x] Done task\n- [ ] Open task\n",
		},
		{
			name:    "with frontmatter",
			content: "---\ntitle: Test\n---\n# Tasks\n- [ ] Buy milk\n",
			tasks: []models.Task{
				{Status: models.TaskStatusDone, ContentHash: taskContentHash("Buy milk"), LineNumber: 1, RawLine: "- [x] Buy milk"},
			},
			expected: "---\ntitle: Test\n---\n# Tasks\n- [x] Buy milk\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := applyTaskChanges(tt.content, tt.tasks)
			if result != tt.expected {
				t.Errorf("applyTaskChanges():\ngot:  %q\nwant: %q", result, tt.expected)
			}
		})
	}
}

// taskContentHash mirrors the parser's content hash for test data.
func taskContentHash(text string) string {
	return models.ContentHash(text)
}
