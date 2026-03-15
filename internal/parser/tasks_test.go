package parser

import (
	"strings"
	"testing"
)

func TestExtractTasks_BasicOpenDone(t *testing.T) {
	content := `# My Note

- [ ] open task
- [x] done task
- [X] also done
`
	tasks := ExtractTasks(content)
	if len(tasks) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(tasks))
	}

	if tasks[0].Status != "open" || tasks[0].Text != "open task" {
		t.Errorf("task 0: got status=%q text=%q", tasks[0].Status, tasks[0].Text)
	}
	if tasks[1].Status != "done" || tasks[1].Text != "done task" {
		t.Errorf("task 1: got status=%q text=%q", tasks[1].Status, tasks[1].Text)
	}
	if tasks[2].Status != "done" || tasks[2].Text != "also done" {
		t.Errorf("task 2: got status=%q text=%q", tasks[2].Status, tasks[2].Text)
	}
}

func TestExtractTasks_InlineLabels(t *testing.T) {
	content := `- [ ] fix the bug #work #urgent`
	tasks := ExtractTasks(content)
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	task := tasks[0]
	if len(task.Labels) != 2 {
		t.Fatalf("expected 2 labels, got %d: %v", len(task.Labels), task.Labels)
	}
	if task.Labels[0] != "work" || task.Labels[1] != "urgent" {
		t.Errorf("expected [work, urgent], got %v", task.Labels)
	}
	if task.Text != "fix the bug" {
		t.Errorf("expected cleaned text 'fix the bug', got %q", task.Text)
	}
}

func TestExtractTasks_DueDate(t *testing.T) {
	content := `- [ ] deploy staging due:2026-03-20`
	tasks := ExtractTasks(content)
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	task := tasks[0]
	if task.DueDate == nil || *task.DueDate != "2026-03-20" {
		t.Errorf("expected due date 2026-03-20, got %v", task.DueDate)
	}
	if task.Text != "deploy staging" {
		t.Errorf("expected cleaned text 'deploy staging', got %q", task.Text)
	}
}

func TestExtractTasks_DueDateAndLabels(t *testing.T) {
	content := `- [ ] review PR #42 #work due:2026-03-20 #urgent`
	tasks := ExtractTasks(content)
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	task := tasks[0]
	if task.DueDate == nil || *task.DueDate != "2026-03-20" {
		t.Errorf("expected due date 2026-03-20, got %v", task.DueDate)
	}
	if len(task.Labels) != 2 {
		t.Fatalf("expected 2 labels, got %d: %v", len(task.Labels), task.Labels)
	}
	// #42 should NOT be treated as a label (starts with digit)
	if task.Labels[0] != "work" || task.Labels[1] != "urgent" {
		t.Errorf("expected [work, urgent], got %v", task.Labels)
	}
	if task.Text != "review PR #42" {
		t.Errorf("expected cleaned text 'review PR #42', got %q", task.Text)
	}
}

func TestExtractTasks_HeadingPath(t *testing.T) {
	content := `# Daily Note

## Morning

- [ ] standup meeting

## Afternoon

### Tasks

- [ ] code review
- [x] write tests

## Evening

- [ ] read book
`
	tasks := ExtractTasks(content)
	if len(tasks) != 4 {
		t.Fatalf("expected 4 tasks, got %d", len(tasks))
	}

	if tasks[0].HeadingPath != "Daily Note > Morning" {
		t.Errorf("task 0 heading: got %q", tasks[0].HeadingPath)
	}
	if tasks[1].HeadingPath != "Daily Note > Afternoon > Tasks" {
		t.Errorf("task 1 heading: got %q", tasks[1].HeadingPath)
	}
	if tasks[2].HeadingPath != "Daily Note > Afternoon > Tasks" {
		t.Errorf("task 2 heading: got %q", tasks[2].HeadingPath)
	}
	if tasks[3].HeadingPath != "Daily Note > Evening" {
		t.Errorf("task 3 heading: got %q", tasks[3].HeadingPath)
	}
}

func TestExtractTasks_NestedCheckboxes(t *testing.T) {
	content := `- [ ] parent task
  - [ ] child task
    - [x] grandchild task`
	tasks := ExtractTasks(content)
	if len(tasks) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(tasks))
	}
	if tasks[0].Text != "parent task" {
		t.Errorf("task 0: got %q", tasks[0].Text)
	}
	if tasks[1].Text != "child task" {
		t.Errorf("task 1: got %q", tasks[1].Text)
	}
	if tasks[2].Text != "grandchild task" || tasks[2].Status != "done" {
		t.Errorf("task 2: got text=%q status=%q", tasks[2].Text, tasks[2].Status)
	}
}

func TestExtractTasks_LineNumbers(t *testing.T) {
	content := `some text

- [ ] first task
more text
- [x] second task`
	tasks := ExtractTasks(content)
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(tasks))
	}
	if tasks[0].LineNumber != 3 {
		t.Errorf("task 0 line: expected 3, got %d", tasks[0].LineNumber)
	}
	if tasks[1].LineNumber != 5 {
		t.Errorf("task 1 line: expected 5, got %d", tasks[1].LineNumber)
	}
}

func TestExtractTasks_NoTasks(t *testing.T) {
	content := `# Just a heading

Some regular text.
- regular list item
- another item
`
	tasks := ExtractTasks(content)
	if len(tasks) != 0 {
		t.Fatalf("expected 0 tasks, got %d", len(tasks))
	}
}

func TestExtractTasks_ContentHashStability(t *testing.T) {
	// Same task text with different metadata should produce the same content hash.
	content1 := `- [ ] fix the bug`
	content2 := `- [x] fix the bug #work due:2026-03-20`

	tasks1 := ExtractTasks(content1)
	tasks2 := ExtractTasks(content2)

	if len(tasks1) != 1 || len(tasks2) != 1 {
		t.Fatal("expected 1 task each")
	}
	if tasks1[0].ContentHash != tasks2[0].ContentHash {
		t.Errorf("content hashes differ: %q vs %q", tasks1[0].ContentHash, tasks2[0].ContentHash)
	}
}

func TestExtractTasks_RawLine(t *testing.T) {
	content := `  - [ ] indented task #work`
	tasks := ExtractTasks(content)
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	if tasks[0].RawLine != "- [ ] indented task #work" {
		t.Errorf("raw line: got %q", tasks[0].RawLine)
	}
}

func TestExtractTasks_DuplicateLabels(t *testing.T) {
	content := `- [ ] task #Work #work`
	tasks := ExtractTasks(content)
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	if len(tasks[0].Labels) != 1 {
		t.Errorf("expected 1 deduplicated label, got %d: %v", len(tasks[0].Labels), tasks[0].Labels)
	}
}

func TestExtractTasks_WithFrontmatter(t *testing.T) {
	content := `---
title: Daily Note
labels: [daily]
---

# Tasks

- [ ] morning standup
- [x] review PR
`
	tasks := ExtractTasks(content)
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(tasks))
	}
	if tasks[0].HeadingPath != "Tasks" {
		t.Errorf("task 0 heading: got %q", tasks[0].HeadingPath)
	}
}

func TestExtractTasks_EmptyContent(t *testing.T) {
	tasks := ExtractTasks("")
	if len(tasks) != 0 {
		t.Fatalf("expected 0 tasks, got %d", len(tasks))
	}
}

func TestExtractTasks_WikiLinkInTask(t *testing.T) {
	content := `- [ ] Review [[project plan]]`
	tasks := ExtractTasks(content)
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	if !strings.Contains(tasks[0].Text, "project plan") {
		t.Errorf("task text should contain 'project plan', got %q", tasks[0].Text)
	}

	// Also verify wiki-link is extracted at document level
	doc := ParseMarkdown(content)
	if len(doc.WikiLinks) != 1 || doc.WikiLinks[0] != "project plan" {
		t.Errorf("expected wiki-link 'project plan', got %v", doc.WikiLinks)
	}
}

func TestExtractTasks_LabelAtStartOfText(t *testing.T) {
	content := `- [ ] #urgent fix the bug`
	tasks := ExtractTasks(content)
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	if len(tasks[0].Labels) != 1 || tasks[0].Labels[0] != "urgent" {
		t.Errorf("expected label [urgent], got %v", tasks[0].Labels)
	}
	if tasks[0].Text != "fix the bug" {
		t.Errorf("expected cleaned text 'fix the bug', got %q", tasks[0].Text)
	}
}

func TestExtractTasks_NestedWithHeadingPath(t *testing.T) {
	content := `## Work

- [ ] parent
  - [ ] child
    - [ ] grandchild`
	tasks := ExtractTasks(content)
	if len(tasks) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(tasks))
	}
	for i, task := range tasks {
		if task.HeadingPath != "Work" {
			t.Errorf("task %d heading path: expected 'Work', got %q", i, task.HeadingPath)
		}
	}
}
