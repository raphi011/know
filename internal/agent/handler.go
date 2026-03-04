package agent

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/raphi011/knowhow/internal/auth"
)

type chatRequestBody struct {
	ConversationID string   `json:"conversationId"`
	VaultID        string   `json:"vaultId"`
	Content        string   `json:"content"`
	DocRefs        []string `json:"docRefs"`
}

// HandleChat returns an http.HandlerFunc that handles POST /agent/chat.
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

		if err := auth.RequireVaultAccess(r.Context(), body.VaultID); err != nil {
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
		}

		if err := s.Chat(r.Context(), req, emit); err != nil {
			slog.Error("agent chat error", "error", err)
			emit(StreamEvent{Type: "error", Content: "internal error"})
		}
	}
}
