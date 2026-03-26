package db

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/raphi011/know/internal/models"
)

func makeTaskInput(fileID, vaultID string, lineNumber int, status models.TaskStatus, text, hash string) models.TaskInput {
	return models.TaskInput{
		FileID:      fileID,
		VaultID:     vaultID,
		Status:      status,
		RawLine:     "- [" + string(status[0]) + "] " + text,
		Text:        text,
		Labels:      []string{},
		LineNumber:  lineNumber,
		ContentHash: hash,
	}
}

func TestCreateTask(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())
	file, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/task-create-" + suffix + ".md", Title: "Task Create",
		Content: "content", Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile failed: %v", err)
	}
	fileID := models.MustRecordIDString(file.ID)

	dueDate := "2026-06-15"
	heading := "Daily > Tasks"
	input := models.TaskInput{
		FileID:      fileID,
		VaultID:     vaultID,
		Status:      models.TaskStatusOpen,
		RawLine:     "- [ ] Buy milk due:2026-06-15 #shopping",
		Text:        "Buy milk",
		Labels:      []string{"shopping"},
		DueDate:     &dueDate,
		LineNumber:  3,
		HeadingPath: &heading,
		ContentHash: "abc123hash",
	}

	task, err := testDB.CreateTask(ctx, input)
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}
	if task == nil {
		t.Fatal("CreateTask returned nil")
	}

	taskID := models.MustRecordIDString(task.ID)
	fetched, err := testDB.GetTaskByID(ctx, taskID)
	if err != nil {
		t.Fatalf("GetTaskByID failed: %v", err)
	}
	if fetched == nil {
		t.Fatal("GetTaskByID returned nil for existing task")
	}
	if fetched.Status != models.TaskStatusOpen {
		t.Errorf("expected status 'open', got %q", fetched.Status)
	}
	if fetched.Text != "Buy milk" {
		t.Errorf("expected text 'Buy milk', got %q", fetched.Text)
	}
	if fetched.LineNumber != 3 {
		t.Errorf("expected line_number 3, got %d", fetched.LineNumber)
	}
	if fetched.DueDate == nil || *fetched.DueDate != "2026-06-15" {
		t.Errorf("expected due_date '2026-06-15', got %v", fetched.DueDate)
	}
	if fetched.HeadingPath == nil || *fetched.HeadingPath != "Daily > Tasks" {
		t.Errorf("expected heading_path 'Daily > Tasks', got %v", fetched.HeadingPath)
	}
	if len(fetched.Labels) != 1 || fetched.Labels[0] != "shopping" {
		t.Errorf("expected labels ['shopping'], got %v", fetched.Labels)
	}
	if fetched.ContentHash != "abc123hash" {
		t.Errorf("expected content_hash 'abc123hash', got %q", fetched.ContentHash)
	}
}

func TestCreateTask_ValidationError(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())
	file, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/task-validate-" + suffix + ".md", Title: "Task Validate",
		Content: "content", Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile failed: %v", err)
	}
	fileID := models.MustRecordIDString(file.ID)

	// ContentHash is empty — must fail validation
	_, err = testDB.CreateTask(ctx, models.TaskInput{
		FileID:      fileID,
		VaultID:     vaultID,
		Status:      models.TaskStatusOpen,
		RawLine:     "- [ ] missing hash",
		Text:        "missing hash",
		Labels:      []string{},
		LineNumber:  1,
		ContentHash: "", // invalid
	})
	if err == nil {
		t.Error("expected error for empty ContentHash, got nil")
	}
}

func TestUpdateTask(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())
	file, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/task-update-" + suffix + ".md", Title: "Task Update",
		Content: "content", Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile failed: %v", err)
	}
	fileID := models.MustRecordIDString(file.ID)

	task, err := testDB.CreateTask(ctx, makeTaskInput(fileID, vaultID, 1, models.TaskStatusOpen, "Original task", "hash1"))
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}
	taskID := models.MustRecordIDString(task.ID)

	if err := testDB.UpdateTask(ctx, taskID, TaskUpdate{
		Status:     models.TaskStatusDone,
		RawLine:    "- [x] Updated task",
		Text:       "Updated task",
		Labels:     []string{"done"},
		LineNumber: 2,
	}); err != nil {
		t.Fatalf("UpdateTask failed: %v", err)
	}

	fetched, err := testDB.GetTaskByID(ctx, taskID)
	if err != nil {
		t.Fatalf("GetTaskByID failed: %v", err)
	}
	if fetched.Status != models.TaskStatusDone {
		t.Errorf("expected status 'done', got %q", fetched.Status)
	}
	if fetched.Text != "Updated task" {
		t.Errorf("expected text 'Updated task', got %q", fetched.Text)
	}
	if fetched.LineNumber != 2 {
		t.Errorf("expected line_number 2, got %d", fetched.LineNumber)
	}
}

func TestUpdateTask_NotFound(t *testing.T) {
	ctx := context.Background()

	err := testDB.UpdateTask(ctx, "task:nonexistent_"+fmt.Sprint(time.Now().UnixNano()), TaskUpdate{
		Status:     models.TaskStatusDone,
		RawLine:    "- [x] ghost",
		Text:       "ghost",
		Labels:     []string{},
		LineNumber: 1,
	})
	if err == nil {
		t.Error("expected error when updating nonexistent task, got nil")
	}
}

func TestDeleteTask(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())
	file, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/task-delete-" + suffix + ".md", Title: "Task Delete",
		Content: "content", Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile failed: %v", err)
	}
	fileID := models.MustRecordIDString(file.ID)

	task, err := testDB.CreateTask(ctx, makeTaskInput(fileID, vaultID, 1, models.TaskStatusOpen, "Delete me", "hashdelete"))
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}
	taskID := models.MustRecordIDString(task.ID)

	if err := testDB.DeleteTask(ctx, taskID); err != nil {
		t.Fatalf("DeleteTask failed: %v", err)
	}

	fetched, err := testDB.GetTaskByID(ctx, taskID)
	if err != nil {
		t.Fatalf("GetTaskByID after delete failed: %v", err)
	}
	if fetched != nil {
		t.Error("expected nil after delete, got non-nil task")
	}
}

func TestGetTasksByFile(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())
	file, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/task-byfile-" + suffix + ".md", Title: "Task ByFile",
		Content: "content", Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile failed: %v", err)
	}
	fileID := models.MustRecordIDString(file.ID)

	// Create task at line 5 first, then line 1 — GetTasksByFile must sort by line_number ASC
	if _, err := testDB.CreateTask(ctx, makeTaskInput(fileID, vaultID, 5, models.TaskStatusOpen, "Line five", "hashline5")); err != nil {
		t.Fatalf("CreateTask line 5 failed: %v", err)
	}
	if _, err := testDB.CreateTask(ctx, makeTaskInput(fileID, vaultID, 1, models.TaskStatusDone, "Line one", "hashline1")); err != nil {
		t.Fatalf("CreateTask line 1 failed: %v", err)
	}

	tasks, err := testDB.GetTasksByFile(ctx, fileID)
	if err != nil {
		t.Fatalf("GetTasksByFile failed: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(tasks))
	}
	if tasks[0].LineNumber != 1 {
		t.Errorf("expected first task at line 1 (ASC order), got line %d", tasks[0].LineNumber)
	}
	if tasks[1].LineNumber != 5 {
		t.Errorf("expected second task at line 5, got line %d", tasks[1].LineNumber)
	}
}

func TestListTasks_StatusFilter(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())
	file, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/task-status-" + suffix + ".md", Title: "Task Status",
		Content: "content", Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile failed: %v", err)
	}
	fileID := models.MustRecordIDString(file.ID)

	if _, err := testDB.CreateTask(ctx, makeTaskInput(fileID, vaultID, 1, models.TaskStatusOpen, "Open task", "hashopen")); err != nil {
		t.Fatalf("CreateTask open failed: %v", err)
	}
	if _, err := testDB.CreateTask(ctx, makeTaskInput(fileID, vaultID, 2, models.TaskStatusDone, "Done task", "hashdone")); err != nil {
		t.Fatalf("CreateTask done failed: %v", err)
	}

	status := models.TaskStatusOpen
	tasks, err := testDB.ListTasks(ctx, TaskFilter{VaultID: vaultID, Status: &status})
	if err != nil {
		t.Fatalf("ListTasks with status filter failed: %v", err)
	}
	for _, task := range tasks {
		if task.Status != models.TaskStatusOpen {
			t.Errorf("expected only open tasks, got status %q", task.Status)
		}
	}
	// Must find the open task we created
	var found bool
	for _, task := range tasks {
		if task.Text == "Open task" {
			found = true
			break
		}
	}
	if !found {
		t.Error("open task not found in status-filtered results")
	}
}

func TestListTasks_LabelFilter(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())
	file, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/task-label-" + suffix + ".md", Title: "Task Label",
		Content: "content", Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile failed: %v", err)
	}
	fileID := models.MustRecordIDString(file.ID)

	// Task with label "work"
	if _, err := testDB.CreateTask(ctx, models.TaskInput{
		FileID: fileID, VaultID: vaultID, Status: models.TaskStatusOpen,
		RawLine: "- [ ] Work task #work", Text: "Work task",
		Labels: []string{"work"}, LineNumber: 1, ContentHash: "hashwork",
	}); err != nil {
		t.Fatalf("CreateTask work failed: %v", err)
	}
	// Task with label "personal"
	if _, err := testDB.CreateTask(ctx, models.TaskInput{
		FileID: fileID, VaultID: vaultID, Status: models.TaskStatusOpen,
		RawLine: "- [ ] Personal task #personal", Text: "Personal task",
		Labels: []string{"personal"}, LineNumber: 2, ContentHash: "hashpersonal",
	}); err != nil {
		t.Fatalf("CreateTask personal failed: %v", err)
	}

	tasks, err := testDB.ListTasks(ctx, TaskFilter{VaultID: vaultID, Labels: []string{"work"}})
	if err != nil {
		t.Fatalf("ListTasks with label filter failed: %v", err)
	}
	for _, task := range tasks {
		hasLabel := slices.Contains(task.Labels, "work")
		if !hasLabel {
			t.Errorf("task %q does not have label 'work'", task.Text)
		}
	}
	var found bool
	for _, task := range tasks {
		if task.Text == "Work task" {
			found = true
			break
		}
	}
	if !found {
		t.Error("work task not found in label-filtered results")
	}
}

func TestListTasks_DueDateFilter(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())
	file, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/task-due-" + suffix + ".md", Title: "Task Due",
		Content: "content", Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile failed: %v", err)
	}
	fileID := models.MustRecordIDString(file.ID)

	earlyDate := "2026-01-10"
	lateDate := "2026-12-31"

	if _, err := testDB.CreateTask(ctx, models.TaskInput{
		FileID: fileID, VaultID: vaultID, Status: models.TaskStatusOpen,
		RawLine: "- [ ] Early task", Text: "Early task",
		Labels: []string{}, DueDate: &earlyDate, LineNumber: 1, ContentHash: "hashearly",
	}); err != nil {
		t.Fatalf("CreateTask early failed: %v", err)
	}
	if _, err := testDB.CreateTask(ctx, models.TaskInput{
		FileID: fileID, VaultID: vaultID, Status: models.TaskStatusOpen,
		RawLine: "- [ ] Late task", Text: "Late task",
		Labels: []string{}, DueDate: &lateDate, LineNumber: 2, ContentHash: "hashlate",
	}); err != nil {
		t.Fatalf("CreateTask late failed: %v", err)
	}

	// due_before=2026-06-01 should only return the early task
	dueBefore := "2026-06-01"
	tasks, err := testDB.ListTasks(ctx, TaskFilter{VaultID: vaultID, DueBefore: &dueBefore})
	if err != nil {
		t.Fatalf("ListTasks with due_before filter failed: %v", err)
	}
	for _, task := range tasks {
		if task.DueDate != nil && *task.DueDate > dueBefore {
			t.Errorf("task with due_date %q should not appear with due_before=%q", *task.DueDate, dueBefore)
		}
	}
	var earlyFound bool
	for _, task := range tasks {
		if task.Text == "Early task" {
			earlyFound = true
			break
		}
	}
	if !earlyFound {
		t.Error("early task not found with due_before filter")
	}

	// due_after=2026-06-01 should only return the late task
	dueAfter := "2026-06-01"
	tasks, err = testDB.ListTasks(ctx, TaskFilter{VaultID: vaultID, DueAfter: &dueAfter})
	if err != nil {
		t.Fatalf("ListTasks with due_after filter failed: %v", err)
	}
	var lateFound bool
	for _, task := range tasks {
		if task.Text == "Late task" {
			lateFound = true
			break
		}
	}
	if !lateFound {
		t.Error("late task not found with due_after filter")
	}
}

func TestListTasks_Pagination(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())
	file, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/task-page-" + suffix + ".md", Title: "Task Page",
		Content: "content", Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile failed: %v", err)
	}
	fileID := models.MustRecordIDString(file.ID)

	for i := range 3 {
		if _, err := testDB.CreateTask(ctx, makeTaskInput(
			fileID, vaultID, i+1, models.TaskStatusOpen,
			fmt.Sprintf("Task %d", i+1),
			fmt.Sprintf("hashpage%d", i),
		)); err != nil {
			t.Fatalf("CreateTask %d failed: %v", i, err)
		}
	}

	// All 3 tasks should be present
	all, err := testDB.ListTasks(ctx, TaskFilter{VaultID: vaultID})
	if err != nil {
		t.Fatalf("ListTasks all failed: %v", err)
	}
	if len(all) < 3 {
		t.Fatalf("expected at least 3 tasks, got %d", len(all))
	}

	// limit=2, offset=1 → should return 2 tasks
	paged, err := testDB.ListTasks(ctx, TaskFilter{VaultID: vaultID, Limit: 2, Offset: 1})
	if err != nil {
		t.Fatalf("ListTasks with pagination failed: %v", err)
	}
	if len(paged) != 2 {
		t.Errorf("expected 2 tasks with limit=2 offset=1, got %d", len(paged))
	}
}

func TestCountTasks(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())
	file, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/task-count-" + suffix + ".md", Title: "Task Count",
		Content: "content", Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile failed: %v", err)
	}
	fileID := models.MustRecordIDString(file.ID)

	for i := range 4 {
		if _, err := testDB.CreateTask(ctx, makeTaskInput(
			fileID, vaultID, i+1, models.TaskStatusOpen,
			fmt.Sprintf("Count task %d", i+1),
			fmt.Sprintf("hashcount%d-%s", i, suffix),
		)); err != nil {
			t.Fatalf("CreateTask %d failed: %v", i, err)
		}
	}

	count, err := testDB.CountTasks(ctx, TaskFilter{VaultID: vaultID})
	if err != nil {
		t.Fatalf("CountTasks failed: %v", err)
	}
	if count < 4 {
		t.Errorf("expected at least 4 tasks, got %d", count)
	}

	// Count and List must agree
	tasks, err := testDB.ListTasks(ctx, TaskFilter{VaultID: vaultID})
	if err != nil {
		t.Fatalf("ListTasks failed: %v", err)
	}
	if count != len(tasks) {
		t.Errorf("CountTasks=%d does not match ListTasks count=%d", count, len(tasks))
	}
}

func TestListTasks_FolderFilter(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())
	fileInFolder, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/daily-" + suffix + "/2026-03-17.md", Title: "Daily Note",
		Content: "content", Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile in folder failed: %v", err)
	}
	fileOutside, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/projects-" + suffix + "/readme.md", Title: "Project Readme",
		Content: "content", Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile outside failed: %v", err)
	}
	inFolderID := models.MustRecordIDString(fileInFolder.ID)
	outsideID := models.MustRecordIDString(fileOutside.ID)

	if _, err := testDB.CreateTask(ctx, makeTaskInput(inFolderID, vaultID, 1, models.TaskStatusOpen, "In folder task", "hashinfolder"+suffix)); err != nil {
		t.Fatalf("CreateTask in folder failed: %v", err)
	}
	if _, err := testDB.CreateTask(ctx, makeTaskInput(outsideID, vaultID, 1, models.TaskStatusOpen, "Outside task", "hashoutside"+suffix)); err != nil {
		t.Fatalf("CreateTask outside failed: %v", err)
	}

	// Filter by folder prefix
	folder := "/daily-" + suffix + "/"
	tasks, err := testDB.ListTasks(ctx, TaskFilter{VaultID: vaultID, Folder: &folder})
	if err != nil {
		t.Fatalf("ListTasks with folder filter failed: %v", err)
	}
	var found bool
	for _, task := range tasks {
		if task.Text == "In folder task" {
			found = true
		}
		if task.Text == "Outside task" {
			t.Error("outside task should not appear with folder filter")
		}
	}
	if !found {
		t.Error("in-folder task not found with folder filter")
	}
}

func TestListTasks_FilePathFilter(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())
	filePath := "/exact-file-" + suffix + ".md"
	file, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: filePath, Title: "Exact File",
		Content: "content", Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile failed: %v", err)
	}
	fileID := models.MustRecordIDString(file.ID)

	otherFile, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/other-file-" + suffix + ".md", Title: "Other File",
		Content: "content", Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile other failed: %v", err)
	}
	otherFileID := models.MustRecordIDString(otherFile.ID)

	if _, err := testDB.CreateTask(ctx, makeTaskInput(fileID, vaultID, 1, models.TaskStatusOpen, "Target task", "hashtarget"+suffix)); err != nil {
		t.Fatalf("CreateTask target failed: %v", err)
	}
	if _, err := testDB.CreateTask(ctx, makeTaskInput(otherFileID, vaultID, 1, models.TaskStatusOpen, "Other task", "hashother"+suffix)); err != nil {
		t.Fatalf("CreateTask other failed: %v", err)
	}

	tasks, err := testDB.ListTasks(ctx, TaskFilter{VaultID: vaultID, FilePath: &filePath})
	if err != nil {
		t.Fatalf("ListTasks with file_path filter failed: %v", err)
	}
	var found bool
	for _, task := range tasks {
		if task.Text == "Target task" {
			found = true
		}
		if task.Text == "Other task" {
			t.Error("other task should not appear with exact file_path filter")
		}
	}
	if !found {
		t.Error("target task not found with file_path filter")
	}
}

func TestListTasks_DocPathAndDocTitle(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())
	filePath := "/denorm-" + suffix + ".md"
	fileTitle := "Denorm Doc " + suffix
	file, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: filePath, Title: fileTitle,
		Content: "content", Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile failed: %v", err)
	}
	fileID := models.MustRecordIDString(file.ID)

	if _, err := testDB.CreateTask(ctx, makeTaskInput(fileID, vaultID, 1, models.TaskStatusOpen, "Denorm task", "hashdenorm"+suffix)); err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	tasks, err := testDB.ListTasks(ctx, TaskFilter{VaultID: vaultID})
	if err != nil {
		t.Fatalf("ListTasks failed: %v", err)
	}
	for _, task := range tasks {
		if task.Text == "Denorm task" {
			if task.DocPath != filePath {
				t.Errorf("expected doc_path %q, got %q", filePath, task.DocPath)
			}
			if task.DocTitle != fileTitle {
				t.Errorf("expected doc_title %q, got %q", fileTitle, task.DocTitle)
			}
			return
		}
	}
	t.Error("denorm task not found in results")
}

func TestUpdateTaskAndSetDirtyFlag(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())
	file, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/toggle-dirty-" + suffix + ".md",
		Title: "Toggle Dirty", Content: "- [ ] Test task", Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile: %v", err)
	}
	fileID := models.MustRecordIDString(file.ID)

	task, err := testDB.CreateTask(ctx, models.TaskInput{
		FileID: fileID, VaultID: vaultID, Status: models.TaskStatusOpen,
		RawLine: "- [ ] Test task", Text: "Test task", Labels: []string{},
		LineNumber: 1, ContentHash: "testhash",
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	taskID := models.MustRecordIDString(task.ID)

	// Update task status to done (simulates toggle).
	if err := testDB.UpdateTask(ctx, taskID, TaskUpdate{
		Status: models.TaskStatusDone, RawLine: "- [x] Test task",
		Text: "Test task", Labels: []string{}, LineNumber: 1,
	}); err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}

	// Set dirty flag.
	if err := testDB.SetFileDirtyTasks(ctx, fileID, true); err != nil {
		t.Fatalf("SetFileDirtyTasks: %v", err)
	}

	// Verify file is dirty.
	updatedFile, err := testDB.GetFileByID(ctx, fileID)
	if err != nil {
		t.Fatalf("GetFileByID: %v", err)
	}
	if !updatedFile.DirtyTasks {
		t.Error("expected dirty_tasks=true after toggle")
	}

	// Verify task status changed.
	updatedTask, err := testDB.GetTaskByID(ctx, taskID)
	if err != nil {
		t.Fatalf("GetTaskByID: %v", err)
	}
	if updatedTask.Status != models.TaskStatusDone {
		t.Errorf("expected task status 'done', got %q", updatedTask.Status)
	}
}
