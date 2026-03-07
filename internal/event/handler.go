package event

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/raphi011/knowhow/internal/auth"
)

// HandleEvents returns an HTTP handler for GET /events that streams change events via SSE.
func HandleEvents(bus *Bus) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		vaultID := r.URL.Query().Get("vaultId")
		if vaultID == "" {
			http.Error(w, "vaultId query parameter required", http.StatusBadRequest)
			return
		}

		if _, err := auth.FromContext(r.Context()); err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if err := auth.RequireVaultAccess(r.Context(), vaultID); err != nil {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")

		ch, unsub := bus.Subscribe(vaultID)
		defer unsub()

		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case evt, ok := <-ch:
				if !ok {
					return // channel closed (slow consumer or shutdown)
				}
				data, err := json.Marshal(evt)
				if err != nil {
					slog.Warn("failed to marshal change event", "error", err)
					continue
				}
				fmt.Fprintf(w, "data: %s\n\n", data)
				flusher.Flush()
			case <-ticker.C:
				fmt.Fprintf(w, ": ping\n\n")
				flusher.Flush()
			case <-r.Context().Done():
				return
			}
		}
	}
}
