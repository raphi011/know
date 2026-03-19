package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/raphi011/know/internal/auth"
	"github.com/raphi011/know/internal/db"
	"github.com/raphi011/know/internal/httputil"
	"github.com/raphi011/know/internal/logutil"
	"github.com/raphi011/know/internal/models"
)

func (s *Server) listTasks(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vaultID := auth.MustVaultIDFromCtx(ctx)
	logger := logutil.FromCtx(ctx)

	filter := db.TaskFilter{
		VaultID: vaultID,
	}

	if v := r.URL.Query().Get("status"); v != "" {
		status := models.TaskStatus(v)
		if !status.Valid() {
			httputil.WriteProblem(w, http.StatusBadRequest, "status must be 'open' or 'done'")
			return
		}
		filter.Status = &status
	}
	if l := r.URL.Query().Get("labels"); l != "" {
		filter.Labels = strings.Split(l, ",")
	}
	if v := r.URL.Query().Get("due_before"); v != "" {
		if !models.IsValidDate(v) {
			httputil.WriteProblem(w, http.StatusBadRequest, "due_before must be YYYY-MM-DD")
			return
		}
		filter.DueBefore = &v
	}
	if v := r.URL.Query().Get("due_after"); v != "" {
		if !models.IsValidDate(v) {
			httputil.WriteProblem(w, http.StatusBadRequest, "due_after must be YYYY-MM-DD")
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
			httputil.WriteProblem(w, http.StatusBadRequest, "limit must be a positive integer")
			return
		}
		filter.Limit = n
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			httputil.WriteProblem(w, http.StatusBadRequest, "offset must be a non-negative integer")
			return
		}
		filter.Offset = n
	}

	tasks, err := s.app.DBClient().ListTasks(ctx, filter)
	if err != nil {
		httputil.WriteProblem(w, http.StatusInternalServerError, "failed to list tasks")
		logger.Error("list tasks", "vault_id", vaultID, "error", err)
		return
	}

	total, err := s.app.DBClient().CountTasks(ctx, filter)
	if err != nil {
		httputil.WriteProblem(w, http.StatusInternalServerError, "failed to count tasks")
		logger.Error("count tasks", "vault_id", vaultID, "error", err)
		return
	}

	var resp []TaskResponse
	for _, t := range tasks {
		tr, err := taskResponseFromModel(t)
		if err != nil {
			httputil.WriteProblem(w, http.StatusInternalServerError, "invalid task record ID")
			logger.Error("task with corrupt record ID", "error", err)
			return
		}
		resp = append(resp, tr)
	}
	writeJSON(w, http.StatusOK, httputil.NewListResponse(resp, total))
}

func (s *Server) toggleTask(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("id")
	if taskID == "" {
		httputil.WriteProblem(w, http.StatusBadRequest, "task id required")
		return
	}

	ctx := r.Context()
	vaultID := auth.MustVaultIDFromCtx(ctx)
	if err := auth.RequireVaultRole(ctx, vaultID, models.RoleWrite); err != nil {
		httputil.WriteProblem(w, http.StatusForbidden, "forbidden")
		return
	}

	logger := logutil.FromCtx(ctx)

	updated, err := s.app.FileService().ToggleTask(ctx, vaultID, taskID)
	if err != nil {
		httputil.WriteProblem(w, http.StatusInternalServerError, "failed to toggle task")
		logger.Error("toggle task", "task_id", taskID, "error", err)
		return
	}
	// updated is never nil here — ToggleTask returns an error if the task
	// disappears after toggle (concurrent modification).

	resp, err := taskResponseFromTask(*updated)
	if err != nil {
		httputil.WriteProblem(w, http.StatusInternalServerError, "invalid task id in response")
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
