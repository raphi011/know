package agent

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/raphi011/knowhow/internal/auth"
	"github.com/raphi011/knowhow/internal/models"
)

type chatRequestBody struct {
	ConversationID string   `json:"conversationId"`
	VaultID        string   `json:"vaultId"`
	Content        string   `json:"content"`
	DocRefs        []string `json:"docRefs"`
	AutoApprove    bool     `json:"autoApprove"`
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

		r.Body = http.MaxBytesReader(w, r.Body, 64*1024) // 64KB max
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
				slog.Warn("failed to marshal SSE event", "error", err)
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}

		req := ChatRequest{
			ConversationID: body.ConversationID,
			VaultID:        body.VaultID,
			UserID:         ac.UserID,
			Content:        body.Content,
			DocRefs:        body.DocRefs,
			AutoApprove:    body.AutoApprove,
		}

		if err := s.Chat(r.Context(), req, emit); err != nil {
			slog.Error("agent chat error", "error", err, "vault_id", req.VaultID, "user_id", req.UserID)
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
