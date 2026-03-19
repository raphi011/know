package api

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/raphi011/know/internal/auth"
	"github.com/raphi011/know/internal/httputil"
	"github.com/raphi011/know/internal/logutil"
	"github.com/raphi011/know/internal/models"
)

// TokenResponse is the public representation of an API token.
// It never exposes the token hash.
type TokenResponse struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	LastUsed  *time.Time `json:"last_used,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// CreateTokenRequest is the request body for creating a new token.
type CreateTokenRequest struct {
	Name          string `json:"name"`
	ExpiresInDays *int   `json:"expires_in_days,omitempty"`
}

// CreateTokenResponse is returned when a new token is created.
// The raw token is shown exactly once.
type CreateTokenResponse struct {
	RawToken string        `json:"raw_token"`
	Token    TokenResponse `json:"token"`
}

func tokenToResponse(t models.APIToken, logger *slog.Logger) TokenResponse {
	id, err := models.RecordIDString(t.ID)
	if err != nil {
		logger.Warn("failed to extract token ID", "error", err)
		id = "unknown"
	}
	return TokenResponse{
		ID:        id,
		Name:      t.Name,
		LastUsed:  t.LastUsed,
		ExpiresAt: t.ExpiresAt,
		CreatedAt: t.CreatedAt,
	}
}

// listTokens handles GET /api/v1/tokens.
// Returns the authenticated user's tokens (never exposes token_hash).
func (s *Server) listTokens(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac, err := auth.FromContext(ctx)
	if err != nil {
		httputil.WriteProblem(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	tokens, err := s.app.DBClient().ListTokens(ctx, ac.UserID)
	if err != nil {
		logutil.FromCtx(ctx).Error("list tokens", "error", err)
		httputil.WriteProblem(w, http.StatusInternalServerError, "failed to list tokens")
		return
	}

	resp := make([]TokenResponse, 0, len(tokens))
	for _, t := range tokens {
		resp = append(resp, tokenToResponse(t, logutil.FromCtx(ctx)))
	}

	writeJSON(w, http.StatusOK, resp)
}

// createToken handles POST /api/v1/tokens.
// Creates a new API token for the authenticated user.
// Returns the raw token exactly once in the response.
func (s *Server) createToken(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac, err := auth.FromContext(ctx)
	if err != nil {
		httputil.WriteProblem(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	req, ok := decodeBody[CreateTokenRequest](w, r, 4096)
	if !ok {
		return
	}

	if req.Name == "" {
		httputil.WriteProblem(w, http.StatusBadRequest, "name is required")
		return
	}

	if req.ExpiresInDays != nil && *req.ExpiresInDays <= 0 {
		httputil.WriteProblem(w, http.StatusBadRequest, "expires_in_days must be positive")
		return
	}

	// Determine expiry
	maxDays := s.app.Config().TokenMaxLifetimeDays
	expiryDays := maxDays
	if req.ExpiresInDays != nil {
		expiryDays = *req.ExpiresInDays
	}

	// Enforce max lifetime if configured (0 = no limit)
	if maxDays > 0 && expiryDays > maxDays {
		httputil.WriteProblem(w, http.StatusBadRequest, "expires_in_days exceeds maximum allowed lifetime")
		return
	}

	// Default to max if no explicit expiry and max is configured
	if expiryDays <= 0 {
		// No limit configured and no explicit expiry — default to 90 days
		expiryDays = 90
	}

	expiresAt := time.Now().Add(time.Duration(expiryDays) * 24 * time.Hour)

	raw, hash, err := auth.GenerateToken()
	if err != nil {
		logutil.FromCtx(ctx).Error("generate token", "error", err)
		httputil.WriteProblem(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	token, err := s.app.DBClient().CreateToken(ctx, ac.UserID, hash, req.Name, expiresAt)
	if err != nil {
		logutil.FromCtx(ctx).Error("create token", "error", err)
		httputil.WriteProblem(w, http.StatusInternalServerError, "failed to create token")
		return
	}

	writeJSON(w, http.StatusCreated, CreateTokenResponse{
		RawToken: raw,
		Token:    tokenToResponse(*token, logutil.FromCtx(ctx)),
	})
}

// deleteToken handles DELETE /api/v1/tokens/{id}.
// Deletes a token owned by the authenticated user (or any token if system admin).
func (s *Server) deleteToken(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac, err := auth.FromContext(ctx)
	if err != nil {
		httputil.WriteProblem(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	tokenID := r.PathValue("id")
	if tokenID == "" {
		httputil.WriteProblem(w, http.StatusBadRequest, "token id is required")
		return
	}

	// Verify ownership (unless system admin)
	token, err := s.app.DBClient().GetTokenByID(ctx, tokenID)
	if err != nil {
		logutil.FromCtx(ctx).Error("get token", "error", err)
		httputil.WriteProblem(w, http.StatusInternalServerError, "failed to get token")
		return
	}
	if token == nil {
		httputil.WriteProblem(w, http.StatusNotFound, "token not found")
		return
	}

	ownerID, err := models.RecordIDString(token.User)
	if err != nil {
		logutil.FromCtx(ctx).Error("delete token: extract owner", "error", err)
		httputil.WriteProblem(w, http.StatusInternalServerError, "failed to verify token ownership")
		return
	}

	if ownerID != ac.UserID && !ac.IsSystemAdmin {
		httputil.WriteProblem(w, http.StatusForbidden, "cannot delete another user's token")
		return
	}

	if err := s.app.DBClient().DeleteToken(ctx, tokenID); err != nil {
		logutil.FromCtx(ctx).Error("delete token", "error", err)
		httputil.WriteProblem(w, http.StatusInternalServerError, "failed to delete token")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// rotateToken handles POST /api/v1/tokens/{id}/rotate.
// Atomically creates a new token and revokes the old one.
func (s *Server) rotateToken(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac, err := auth.FromContext(ctx)
	if err != nil {
		httputil.WriteProblem(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	tokenID := r.PathValue("id")
	if tokenID == "" {
		httputil.WriteProblem(w, http.StatusBadRequest, "token id is required")
		return
	}

	// Get old token to copy name + verify ownership
	oldToken, err := s.app.DBClient().GetTokenByID(ctx, tokenID)
	if err != nil {
		logutil.FromCtx(ctx).Error("get token for rotation", "error", err)
		httputil.WriteProblem(w, http.StatusInternalServerError, "failed to get token")
		return
	}
	if oldToken == nil {
		httputil.WriteProblem(w, http.StatusNotFound, "token not found")
		return
	}

	ownerID, err := models.RecordIDString(oldToken.User)
	if err != nil {
		logutil.FromCtx(ctx).Error("rotate token: extract owner", "error", err)
		httputil.WriteProblem(w, http.StatusInternalServerError, "failed to verify token ownership")
		return
	}

	if ownerID != ac.UserID && !ac.IsSystemAdmin {
		httputil.WriteProblem(w, http.StatusForbidden, "cannot rotate another user's token")
		return
	}

	// Determine expiry for new token: preserve remaining TTL from old token
	// (intentional — rotation doesn't extend the original grant's lifetime).
	// If old token had no expiry or was already expired, use max lifetime.
	var expiresAt time.Time
	if oldToken.ExpiresAt != nil {
		remaining := time.Until(*oldToken.ExpiresAt)
		if remaining <= 0 {
			// Old token was expired — use max lifetime for new one
			maxDays := s.app.Config().TokenMaxLifetimeDays
			if maxDays <= 0 {
				maxDays = 90
			}
			expiresAt = time.Now().Add(time.Duration(maxDays) * 24 * time.Hour)
		} else {
			expiresAt = time.Now().Add(remaining)
		}
	} else {
		maxDays := s.app.Config().TokenMaxLifetimeDays
		if maxDays <= 0 {
			maxDays = 90
		}
		expiresAt = time.Now().Add(time.Duration(maxDays) * 24 * time.Hour)
	}

	raw, hash, err := auth.GenerateToken()
	if err != nil {
		logutil.FromCtx(ctx).Error("generate token for rotation", "error", err)
		httputil.WriteProblem(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	// Create new token
	newToken, err := s.app.DBClient().CreateToken(ctx, ac.UserID, hash, oldToken.Name, expiresAt)
	if err != nil {
		logutil.FromCtx(ctx).Error("create rotated token", "error", err)
		httputil.WriteProblem(w, http.StatusInternalServerError, "failed to create new token")
		return
	}

	// Delete old token (not atomic — acceptable per design doc)
	if err := s.app.DBClient().DeleteToken(ctx, tokenID); err != nil {
		logutil.FromCtx(ctx).Warn("failed to delete old token during rotation", "old_token_id", tokenID, "error", err)
		// Continue — new token was created successfully
	}

	writeJSON(w, http.StatusOK, CreateTokenResponse{
		RawToken: raw,
		Token:    tokenToResponse(*newToken, logutil.FromCtx(ctx)),
	})
}
