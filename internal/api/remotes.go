package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/raphi011/know/internal/auth"
	"github.com/raphi011/know/internal/db"
	"github.com/raphi011/know/internal/logutil"
	"github.com/raphi011/know/internal/models"
)

type remoteResponse struct {
	Name      string    `json:"name"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func (s *Server) listRemotes(w http.ResponseWriter, r *http.Request) {
	if err := auth.RequireSystemAdmin(r.Context()); err != nil {
		writeError(w, http.StatusForbidden, "system admin required")
		return
	}

	remoteSvc := s.app.RemoteService()
	if remoteSvc == nil {
		writeJSON(w, http.StatusOK, []remoteResponse{})
		return
	}

	remotes, err := remoteSvc.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list remotes")
		logutil.FromCtx(r.Context()).Error("list remotes", "error", err)
		return
	}

	resp := make([]remoteResponse, len(remotes))
	for i, r := range remotes {
		resp[i] = remoteResponse{
			Name:      r.Name,
			URL:       r.URL,
			CreatedAt: r.CreatedAt,
			UpdatedAt: r.UpdatedAt,
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

type addRemoteRequest struct {
	Name  string `json:"name"`
	URL   string `json:"url"`
	Token string `json:"token"`
}

func (s *Server) addRemote(w http.ResponseWriter, r *http.Request) {
	if err := auth.RequireSystemAdmin(r.Context()); err != nil {
		writeError(w, http.StatusForbidden, "system admin required")
		return
	}

	var body addRemoteRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.Name == "" || body.URL == "" || body.Token == "" {
		writeError(w, http.StatusBadRequest, "name, url, and token are required")
		return
	}
	if strings.Contains(body.Name, "/") || strings.Contains(body.Name, " ") {
		writeError(w, http.StatusBadRequest, "remote name must not contain '/' or spaces")
		return
	}

	remoteSvc := s.app.RemoteService()
	if remoteSvc == nil {
		writeError(w, http.StatusServiceUnavailable, "remote service not available")
		return
	}

	ctx := r.Context()
	logger := logutil.FromCtx(ctx)

	// Validate connectivity by calling /health on the remote
	if err := checkRemoteHealth(ctx, body.URL); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("cannot reach remote: %v", err))
		return
	}

	ac, err := auth.FromContext(ctx)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	remote, err := remoteSvc.Add(ctx, ac.UserID, models.RemoteInput{
		Name:  body.Name,
		URL:   body.URL,
		Token: body.Token,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to add remote")
		logger.Error("add remote", "name", body.Name, "error", err)
		return
	}

	writeJSON(w, http.StatusCreated, remoteResponse{
		Name:      remote.Name,
		URL:       remote.URL,
		CreatedAt: remote.CreatedAt,
		UpdatedAt: remote.UpdatedAt,
	})
}

func (s *Server) removeRemote(w http.ResponseWriter, r *http.Request) {
	if err := auth.RequireSystemAdmin(r.Context()); err != nil {
		writeError(w, http.StatusForbidden, "system admin required")
		return
	}

	name := r.PathValue("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "remote name required")
		return
	}

	remoteSvc := s.app.RemoteService()
	if remoteSvc == nil {
		writeError(w, http.StatusServiceUnavailable, "remote service not available")
		return
	}

	logger := logutil.FromCtx(r.Context())

	if err := remoteSvc.Remove(r.Context(), name); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeError(w, http.StatusNotFound, fmt.Sprintf("remote not found: %s", name))
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to remove remote")
		logger.Error("remove remote", "name", name, "error", err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// checkRemoteHealth performs a GET /health on the remote URL to verify connectivity.
func checkRemoteHealth(ctx context.Context, remoteURL string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, remoteURL+"/health", nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check returned %d", resp.StatusCode)
	}
	return nil
}
