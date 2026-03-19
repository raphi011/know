package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/raphi011/know/internal/auth"
	"github.com/raphi011/know/internal/httputil"
	"github.com/raphi011/know/internal/logutil"
	"github.com/raphi011/know/internal/models"
)

const maxAttachments = 20

type chatRequestBody struct {
	ConversationID string                  `json:"conversationId"`
	Content        string                  `json:"content"`
	DocRefs        []string                `json:"docRefs"`
	Attachments    []models.ChatAttachment `json:"attachments,omitempty"`
	AutoApprove    bool                    `json:"autoApprove"`
}

type chatResponseBody struct {
	ConversationID string `json:"conversationId"`
	Status         string `json:"status"`
}

// HandleChat returns an HTTP handler for POST /agent/chat that starts a background
// agent goroutine and returns 202 with the conversation ID.
func (rn *Runner) HandleChat() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			httputil.WriteProblem(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		ac, err := auth.FromContext(r.Context())
		if err != nil {
			httputil.WriteProblem(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, 50*1024*1024) // 50MB max (base64-encoded image attachments)
		var body chatRequestBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			var maxBytesErr *http.MaxBytesError
			if errors.As(err, &maxBytesErr) {
				httputil.WriteProblem(w, http.StatusRequestEntityTooLarge, "request body too large (max 50MB)")
				return
			}
			httputil.WriteProblem(w, http.StatusBadRequest, "invalid request body")
			return
		}

		vaultID := auth.MustVaultIDFromCtx(r.Context())

		vault, err := rn.db.GetVault(r.Context(), vaultID)
		if err != nil {
			logutil.FromCtx(r.Context()).Warn("vault lookup failed", "vault_id", vaultID, "error", err)
			http.Error(w, "vault lookup failed", http.StatusInternalServerError)
			return
		}
		if vault != nil && !vault.Defaults().IsAgentEnabled() {
			http.Error(w, "agent chat is disabled for this vault", http.StatusForbidden)
			return
		}

		if body.Content == "" {
			httputil.WriteProblem(w, http.StatusBadRequest, "content is required")
			return
		}
		if len(body.Attachments) > maxAttachments {
			httputil.WriteProblem(w, http.StatusBadRequest, fmt.Sprintf("too many attachments (max %d)", maxAttachments))
			return
		}
		for _, att := range body.Attachments {
			if att.Path == "" || att.Content == "" {
				httputil.WriteProblem(w, http.StatusBadRequest, "attachments must have non-empty path and content")
				return
			}
			if att.Type != models.AttachmentTypeText && att.Type != models.AttachmentTypeImage {
				httputil.WriteProblem(w, http.StatusBadRequest, fmt.Sprintf("unsupported attachment type: %q", att.Type))
				return
			}
			if att.Type == models.AttachmentTypeImage {
				switch att.MimeType {
				case "image/png", "image/jpeg", "image/gif", "image/webp":
					// valid
				default:
					httputil.WriteProblem(w, http.StatusBadRequest, fmt.Sprintf("invalid mime type %q for image attachment %s", att.MimeType, att.Path))
					return
				}
			}
		}

		if err := auth.RequireVaultRole(r.Context(), vaultID, models.RoleWrite); err != nil {
			httputil.WriteProblem(w, http.StatusForbidden, "forbidden")
			return
		}

		req := ChatRequest{
			ConversationID: body.ConversationID,
			VaultID:        vaultID,
			UserID:         ac.UserID,
			Content:        body.Content,
			DocRefs:        body.DocRefs,
			Attachments:    body.Attachments,
			AutoApprove:    body.AutoApprove,
		}

		convID, err := rn.Start(r.Context(), req)
		if err != nil {
			logutil.FromCtx(r.Context()).Error("failed to start agent", "error", err)
			httputil.WriteProblem(w, http.StatusInternalServerError, "failed to start agent")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		if err := json.NewEncoder(w).Encode(chatResponseBody{
			ConversationID: convID,
			Status:         "running",
		}); err != nil {
			logutil.FromCtx(r.Context()).Warn("failed to write response body", "error", err)
		}
	}
}

// HandleEvents returns an HTTP handler for GET /agent/events/{id} that streams
// SSE events for a running (or recently completed) agent.
func (rn *Runner) HandleEvents() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			httputil.WriteProblem(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		if _, err := auth.FromContext(r.Context()); err != nil {
			httputil.WriteProblem(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		convID := r.PathValue("id")
		if convID == "" {
			httputil.WriteProblem(w, http.StatusBadRequest, "conversation ID required")
			return
		}

		// Try to subscribe to a running task
		history, ch, unsub, err := rn.Subscribe(convID)
		if err != nil {
			// Task not running — check DB for completed/failed status
			conv, dbErr := rn.db.GetConversation(r.Context(), convID)
			if dbErr != nil {
				logutil.FromCtx(r.Context()).Error("get conversation for events", "conversation_id", convID, "error", dbErr)
				httputil.WriteProblem(w, http.StatusInternalServerError, "internal server error")
				return
			}
			if conv == nil {
				httputil.WriteProblem(w, http.StatusNotFound, "conversation not found")
				return
			}
			if conv.BgStatus == nil {
				httputil.WriteProblem(w, http.StatusNotFound, "not a background task")
				return
			}

			// Return terminal status as SSE
			flusher, ok := w.(http.Flusher)
			if !ok {
				httputil.WriteProblem(w, http.StatusInternalServerError, "streaming not supported")
				return
			}
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Connection", "keep-alive")
			w.Header().Set("X-Accel-Buffering", "no")

			switch *conv.BgStatus {
			case "completed":
				writeSSE(w, flusher, StreamEvent{Type: "msg_end"})
			case "failed":
				errMsg := "agent failed"
				if conv.BgError != nil {
					errMsg = *conv.BgError
				}
				writeSSE(w, flusher, StreamEvent{Type: "error", Content: errMsg})
			default:
				// "running" but not in tasks map — race condition or server restart
				httputil.WriteProblem(w, http.StatusNotFound, "task not available")
			}
			return
		}
		defer unsub()

		// Set up SSE
		flusher, ok := w.(http.Flusher)
		if !ok {
			httputil.WriteProblem(w, http.StatusInternalServerError, "streaming not supported")
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")

		// Replay history
		for _, event := range history {
			if !writeSSE(w, flusher, event) {
				return
			}
		}

		// Stream live events
		for {
			select {
			case event, ok := <-ch:
				if !ok {
					return // agent done, channel closed
				}
				if !writeSSE(w, flusher, event) {
					return // client disconnected
				}
			case <-r.Context().Done():
				return // client disconnected
			}
		}
	}
}

// HandleCancel returns an HTTP handler for POST /agent/cancel/{id} that cancels
// a running agent.
func (rn *Runner) HandleCancel() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			httputil.WriteProblem(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		if _, err := auth.FromContext(r.Context()); err != nil {
			httputil.WriteProblem(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		convID := r.PathValue("id")
		if convID == "" {
			httputil.WriteProblem(w, http.StatusBadRequest, "conversation ID required")
			return
		}

		if err := rn.Cancel(convID); err != nil {
			httputil.WriteProblem(w, http.StatusNotFound, "no running task for this conversation")
			return
		}

		w.WriteHeader(http.StatusOK)
	}
}

// writeSSE marshals an event and writes it as an SSE data line.
// Returns false if the write failed (client disconnected).
func writeSSE(w http.ResponseWriter, flusher http.Flusher, event StreamEvent) bool {
	data, err := json.Marshal(event)
	if err != nil {
		slog.Error("failed to marshal SSE event", "type", event.Type, "error", err)
		return false
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
		return false
	}
	flusher.Flush()
	return true
}

type approvalRequestBody struct {
	ConversationID string         `json:"conversationId"`
	InterruptID    string         `json:"interruptId"`
	Action         ApprovalAction `json:"action"`
	HunkIndexes    []int          `json:"hunkIndexes,omitempty"`
}

// HandleApproval returns an HTTP handler for POST /agent/approval that resumes
// an interrupted agent via eino's checkpoint system.
func (rn *Runner) HandleApproval() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			httputil.WriteProblem(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		ac, err := auth.FromContext(r.Context())
		if err != nil {
			httputil.WriteProblem(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, 16*1024)
		var body approvalRequestBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httputil.WriteProblem(w, http.StatusBadRequest, "invalid request body")
			return
		}

		if body.ConversationID == "" || body.InterruptID == "" {
			httputil.WriteProblem(w, http.StatusBadRequest, "conversationId and interruptId are required")
			return
		}

		switch body.Action {
		case ApprovalApproveAll, ApprovalApproveHunks, ApprovalReject:
			// valid
		default:
			httputil.WriteProblem(w, http.StatusBadRequest, "invalid action")
			return
		}

		// Look up the conversation to check vault access
		conv, dbErr := rn.db.GetConversation(r.Context(), body.ConversationID)
		if dbErr != nil {
			logutil.FromCtx(r.Context()).Error("get conversation for approval", "conversation_id", body.ConversationID, "error", dbErr)
			httputil.WriteProblem(w, http.StatusInternalServerError, "internal server error")
			return
		}
		if conv == nil {
			httputil.WriteProblem(w, http.StatusNotFound, "conversation not found")
			return
		}

		vaultID, vaultErr := models.RecordIDString(conv.Vault)
		if vaultErr != nil {
			httputil.WriteProblem(w, http.StatusInternalServerError, "invalid vault reference")
			return
		}

		if err := auth.RequireVaultRole(r.Context(), vaultID, models.RoleWrite); err != nil {
			httputil.WriteProblem(w, http.StatusForbidden, "forbidden")
			return
		}

		convID, resumeErr := rn.Resume(r.Context(), ResumeRequest{
			ConversationID: body.ConversationID,
			VaultID:        vaultID,
			UserID:         ac.UserID,
			InterruptID:    body.InterruptID,
			Response: ApprovalResponse{
				Action:      body.Action,
				HunkIndexes: body.HunkIndexes,
			},
		})
		if resumeErr != nil {
			logutil.FromCtx(r.Context()).Error("failed to resume agent", "error", resumeErr)
			httputil.WriteProblem(w, http.StatusInternalServerError, "failed to resume agent")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		if err := json.NewEncoder(w).Encode(chatResponseBody{
			ConversationID: convID,
			Status:         "running",
		}); err != nil {
			logutil.FromCtx(r.Context()).Warn("failed to write response body", "error", err)
		}
	}
}
