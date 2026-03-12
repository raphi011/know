package agent

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/raphi011/knowhow/internal/auth"
	"github.com/raphi011/knowhow/internal/logutil"
	"github.com/raphi011/knowhow/internal/models"
)

const maxAttachments = 20

type chatRequestBody struct {
	ConversationID string               `json:"conversationId"`
	VaultID        string               `json:"vaultId"`
	Content        string               `json:"content"`
	DocRefs        []string             `json:"docRefs"`
	Attachments    []models.ChatAttachment `json:"attachments,omitempty"`
	AutoApprove    bool                 `json:"autoApprove"`
}

// HandleChat returns an HTTP handler for POST /agent/chat that streams SSE events back to the client.
func (s *Service) HandleChat() http.HandlerFunc {
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

		r.Body = http.MaxBytesReader(w, r.Body, 5*1024*1024) // 5MB max (text file attachments)
		var body chatRequestBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
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
		}

		if err := auth.RequireVaultRole(r.Context(), body.VaultID, models.RoleWrite); err != nil {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

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

		emit := func(event StreamEvent) {
			data, err := json.Marshal(event)
			if err != nil {
				logutil.FromCtx(r.Context()).Warn("failed to marshal SSE event", "error", err)
				return
			}
			if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
				return // client disconnected
			}
			flusher.Flush()
		}

		// Enrich context logger with conversation and vault info
		chatLogger := logutil.FromCtx(r.Context()).With(
			"conversation_id", body.ConversationID,
			"vault_id", body.VaultID,
		)
		ctx := logutil.WithLogger(r.Context(), chatLogger)

		req := ChatRequest{
			ConversationID: body.ConversationID,
			VaultID:        body.VaultID,
			UserID:         ac.UserID,
			Content:        body.Content,
			DocRefs:        body.DocRefs,
			Attachments:    body.Attachments,
			AutoApprove:    body.AutoApprove,
		}

		if err := s.Chat(ctx, req, emit); err != nil {
			logutil.FromCtx(ctx).Error("agent chat error", "error", err)
			emit(StreamEvent{Type: "error", Content: "Failed to process chat request. Please try again."})
		}
	}
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
