package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/raphi011/knowhow/internal/auth"
	"github.com/raphi011/knowhow/internal/logutil"
	"github.com/raphi011/knowhow/internal/models"
)

const maxAttachments = 20

type chatRequestBody struct {
	ConversationID string                  `json:"conversationId"`
	VaultID        string                  `json:"vaultId"`
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
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		ac, err := auth.FromContext(r.Context())
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, 50*1024*1024) // 50MB max (base64-encoded image attachments)
		var body chatRequestBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			var maxBytesErr *http.MaxBytesError
			if errors.As(err, &maxBytesErr) {
				http.Error(w, "request body too large (max 50MB)", http.StatusRequestEntityTooLarge)
				return
			}
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		if body.VaultID == "" {
			http.Error(w, "vaultId is required", http.StatusBadRequest)
			return
		}
		if body.Content == "" {
			http.Error(w, "content is required", http.StatusBadRequest)
			return
		}
		if len(body.Attachments) > maxAttachments {
			http.Error(w, fmt.Sprintf("too many attachments (max %d)", maxAttachments), http.StatusBadRequest)
			return
		}
		for _, att := range body.Attachments {
			if att.Path == "" || att.Content == "" {
				http.Error(w, "attachments must have non-empty path and content", http.StatusBadRequest)
				return
			}
			if att.Type != models.AttachmentTypeText && att.Type != models.AttachmentTypeImage {
				http.Error(w, fmt.Sprintf("unsupported attachment type: %q", att.Type), http.StatusBadRequest)
				return
			}
			if att.Type == models.AttachmentTypeImage {
				switch att.MimeType {
				case "image/png", "image/jpeg", "image/gif", "image/webp":
					// valid
				default:
					http.Error(w, fmt.Sprintf("invalid mime type %q for image attachment %s", att.MimeType, att.Path), http.StatusBadRequest)
					return
				}
			}
		}

		if err := auth.RequireVaultRole(r.Context(), body.VaultID, models.RoleWrite); err != nil {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		req := ChatRequest{
			ConversationID: body.ConversationID,
			VaultID:        body.VaultID,
			UserID:         ac.UserID,
			Content:        body.Content,
			DocRefs:        body.DocRefs,
			Attachments:    body.Attachments,
			AutoApprove:    body.AutoApprove,
		}

		convID, err := rn.Start(r.Context(), req)
		if err != nil {
			logutil.FromCtx(r.Context()).Error("failed to start agent", "error", err)
			http.Error(w, "failed to start agent", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(chatResponseBody{
			ConversationID: convID,
			Status:         "running",
		})
	}
}

// HandleEvents returns an HTTP handler for GET /agent/events/{id} that streams
// SSE events for a running (or recently completed) agent.
func (rn *Runner) HandleEvents() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		if _, err := auth.FromContext(r.Context()); err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		convID := r.PathValue("id")
		if convID == "" {
			http.Error(w, "conversation ID required", http.StatusBadRequest)
			return
		}

		// Try to subscribe to a running task
		history, ch, unsub, err := rn.Subscribe(convID)
		if err != nil {
			// Task not running — check DB for completed/failed status
			conv, dbErr := rn.db.GetConversation(r.Context(), convID)
			if dbErr != nil || conv == nil {
				http.Error(w, "conversation not found", http.StatusNotFound)
				return
			}
			if conv.BgStatus == nil {
				http.Error(w, "not a background task", http.StatusNotFound)
				return
			}

			// Return terminal status as SSE
			flusher, ok := w.(http.Flusher)
			if !ok {
				http.Error(w, "streaming not supported", http.StatusInternalServerError)
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
				http.Error(w, "task not available", http.StatusNotFound)
			}
			return
		}
		defer unsub()

		// Set up SSE
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
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
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		if _, err := auth.FromContext(r.Context()); err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		convID := r.PathValue("id")
		if convID == "" {
			http.Error(w, "conversation ID required", http.StatusBadRequest)
			return
		}

		if err := rn.Cancel(convID); err != nil {
			http.Error(w, "no running task for this conversation", http.StatusNotFound)
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
	CallID         string         `json:"callId"`
	Action         ApprovalAction `json:"action"`
	HunkIndexes    []int          `json:"hunkIndexes,omitempty"`
}

// HandleApproval returns an HTTP handler for POST /agent/approval that resolves pending tool approvals.
func (s *Service) HandleApproval() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		if _, err := auth.FromContext(r.Context()); err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, 16*1024)
		var body approvalRequestBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		if body.ConversationID == "" || body.CallID == "" {
			http.Error(w, "conversationId and callId are required", http.StatusBadRequest)
			return
		}

		switch body.Action {
		case ApprovalApproveAll, ApprovalApproveHunks, ApprovalReject:
			// valid
		default:
			http.Error(w, "invalid action", http.StatusBadRequest)
			return
		}

		val, ok := s.activeApprovals.Load(body.ConversationID)
		if !ok {
			http.Error(w, "no active approval session for this conversation", http.StatusNotFound)
			return
		}
		session := val.(*approvalSession)

		if err := auth.RequireVaultRole(r.Context(), session.vaultID, models.RoleWrite); err != nil {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		if err := session.registry.resolve(ApprovalResponse{
			CallID:      body.CallID,
			Action:      body.Action,
			HunkIndexes: body.HunkIndexes,
		}); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.WriteHeader(http.StatusOK)
	}
}
