package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/raphi011/know/internal/auth"
	"github.com/raphi011/know/internal/db"
	"github.com/raphi011/know/internal/logutil"
	"github.com/raphi011/know/internal/models"
)

func (s *Server) listTasks(w http.ResponseWriter, r *http.Request) {
	vaultID := r.URL.Query().Get("vault")
	if vaultID == "" {
		writeError(w, http.StatusBadRequest, "vault query parameter required")
		return
	}

	if err := auth.RequireVaultRole(r.Context(), vaultID, models.RoleRead); err != nil {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	logger := logutil.FromCtx(r.Context())

	filter := db.TaskFilter{
		VaultID: vaultID,
	}

	if v := r.URL.Query().Get("status"); v != "" {
		status := models.TaskStatus(v)
		if !status.Valid() {
			writeError(w, http.StatusBadRequest, "status must be 'open' or 'done'")
			return
		}
		filter.Status = &status
	}
	if l := r.URL.Query().Get("labels"); l != "" {
		filter.Labels = strings.Split(l, ",")
	}
	if v := r.URL.Query().Get("due_before"); v != "" {
		if !models.IsValidDate(v) {
			writeError(w, http.StatusBadRequest, "due_before must be YYYY-MM-DD")
			return
		}
		filter.DueBefore = &v
	}
	if v := r.URL.Query().Get("due_after"); v != "" {
		if !models.IsValidDate(v) {
			writeError(w, http.StatusBadRequest, "due_after must be YYYY-MM-DD")
			return
		}
		filter.DueAfter = &v
	}
	if v := r.URL.Query().Get("folder"); v != "" {
		filter.Folder = &v
	}
	if v := r.URL.Query().Get("path"); v != "" {
		filter.FilePath = &v
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			writeError(w, http.StatusBadRequest, "limit must be a positive integer")
			return
		}
		filter.Limit = n
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			writeError(w, http.StatusBadRequest, "offset must be a non-negative integer")
			return
		}
		filter.Offset = n
	}

	tasks, err := s.app.DBClient().ListTasks(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list tasks")
		logger.Error("list tasks", "vault_id", vaultID, "error", err)
		return
	}

	total, err := s.app.DBClient().CountTasks(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to count tasks")
		logger.Error("count tasks", "vault_id", vaultID, "error", err)
		return
	}

	if tasks == nil {
		tasks = []models.TaskWithDoc{}
	}

	var resp []TaskResponse
	for _, t := range tasks {
		tr, err := taskResponseFromModel(t)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "invalid task record ID")
			logger.Error("task with corrupt record ID", "error", err)
			return
		}
		resp = append(resp, tr)
	}
	if resp == nil {
		resp = []TaskResponse{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"tasks": resp,
		"total": total,
	})
}

func (s *Server) toggleTask(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("id")
	if taskID == "" {
		writeError(w, http.StatusBadRequest, "task id required")
		return
	}

	logger := logutil.FromCtx(r.Context())

	// Fetch task to get vault ID for auth check.
	task, err := s.app.DBClient().GetTaskByID(r.Context(), taskID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get task")
		logger.Error("get task for toggle", "task_id", taskID, "error", err)
		return
	}
	if task == nil {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}

	vaultID, err := models.RecordIDString(task.Vault)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "invalid vault id")
		logger.Error("extract vault id", "error", err)
		return
	}

	if err := auth.RequireVaultRole(r.Context(), vaultID, models.RoleWrite); err != nil {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	updated, err := s.app.FileService().ToggleTask(r.Context(), taskID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to toggle task")
		logger.Error("toggle task", "task_id", taskID, "error", err)
		return
	}
	// updated is never nil here — ToggleTask returns an error if the task
	// disappears after toggle (concurrent modification).

	resp, err := taskResponseFromTask(*updated)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "invalid task id in response")
		logger.Error("extract task id", "error", err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// TaskResponse is the JSON representation of a task, optionally with document context
// (populated for list, empty for toggle).
type TaskResponse struct {
	ID            string            `json:"id"`
	DocumentPath  string            `json:"documentPath,omitempty"`
	DocumentTitle string            `json:"documentTitle,omitempty"`
	Status        models.TaskStatus `json:"status"`
	Text          string            `json:"text"`
	Labels        []string          `json:"labels"`
	DueDate       *string           `json:"dueDate,omitempty"`
	HeadingPath   *string           `json:"headingPath,omitempty"`
	LineNumber    int               `json:"lineNumber"`
}

func taskResponseFromModel(t models.TaskWithDoc) (TaskResponse, error) {
	id, err := models.RecordIDString(t.ID)
	if err != nil {
		return TaskResponse{}, fmt.Errorf("extract task id: %w", err)
	}
	labels := t.Labels
	if labels == nil {
		labels = []string{}
	}
	return TaskResponse{
		ID:            id,
		DocumentPath:  t.DocPath,
		DocumentTitle: t.DocTitle,
		Status:        t.Status,
		Text:          t.Text,
		Labels:        labels,
		DueDate:       t.DueDate,
		HeadingPath:   t.HeadingPath,
		LineNumber:    t.LineNumber,
	}, nil
}

func taskResponseFromTask(t models.Task) (TaskResponse, error) {
	id, err := models.RecordIDString(t.ID)
	if err != nil {
		return TaskResponse{}, fmt.Errorf("extract task id: %w", err)
	}
	labels := t.Labels
	if labels == nil {
		labels = []string{}
	}
	return TaskResponse{
		ID:          id,
		Status:      t.Status,
		Text:        t.Text,
		Labels:      labels,
		DueDate:     t.DueDate,
		HeadingPath: t.HeadingPath,
		LineNumber:  t.LineNumber,
	}, nil
}
