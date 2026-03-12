package event

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/raphi011/knowhow/internal/auth"
	"github.com/raphi011/knowhow/internal/logutil"
	"github.com/raphi011/knowhow/internal/models"
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
		if err := auth.RequireVaultRole(r.Context(), vaultID, models.RoleRead); err != nil {
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

		logger := logutil.FromCtx(r.Context())

		for {
			select {
			case evt, ok := <-ch:
				if !ok {
					return // channel closed (slow consumer or shutdown)
				}
				data, err := json.Marshal(evt)
				if err != nil {
					logger.Warn("failed to marshal change event", "error", err)
					continue
				}
				if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
					return // client disconnected
				}
				flusher.Flush()
			case <-ticker.C:
				if _, err := fmt.Fprintf(w, ": ping\n\n"); err != nil {
					return // client disconnected
				}
				flusher.Flush()
			case <-r.Context().Done():
				return
			}
		}
	}
}
