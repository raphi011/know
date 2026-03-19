package event

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/raphi011/know/internal/auth"
	"github.com/raphi011/know/internal/httputil"
	"github.com/raphi011/know/internal/logutil"
	"github.com/raphi011/know/internal/models"
)

// HandleEvents returns an HTTP handler for GET /events that streams change events via SSE.
func HandleEvents(bus *Bus) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			httputil.WriteProblem(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		vaultID := r.URL.Query().Get("vaultId")
		if vaultID == "" {
			httputil.WriteProblem(w, http.StatusBadRequest, "vaultId query parameter required")
			return
		}

		if _, err := auth.FromContext(r.Context()); err != nil {
			httputil.WriteProblem(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		if err := auth.RequireVaultRole(r.Context(), vaultID, models.RoleRead); err != nil {
			httputil.WriteProblem(w, http.StatusForbidden, "forbidden")
			return
		}

		flusher, ok := w.(http.Flusher)
		if !ok {
			httputil.WriteProblem(w, http.StatusInternalServerError, "streaming not supported")
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
