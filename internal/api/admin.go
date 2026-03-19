package api

import (
	"net/http"

	"github.com/raphi011/know/internal/auth"
	"github.com/raphi011/know/internal/httputil"
	"github.com/raphi011/know/internal/logutil"
	"github.com/raphi011/know/internal/models"
)

func (s *Server) listUsers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logutil.FromCtx(ctx)
	if err := auth.RequireSystemAdmin(ctx); err != nil {
		logger.Warn("admin access denied", "error", err)
		httputil.WriteProblem(w, http.StatusForbidden, "system admin required")
		return
	}

	users, err := s.app.DBClient().ListUsers(ctx)
	if err != nil {
		httputil.WriteProblem(w, http.StatusInternalServerError, "failed to list users")
		logger.Error("list users", "error", err)
		return
	}

	type userResponse struct {
		ID            string  `json:"id"`
		Name          string  `json:"name"`
		Email         *string `json:"email,omitempty"`
		IsSystemAdmin bool    `json:"is_system_admin"`
		OIDCProvider  *string `json:"oidc_provider,omitempty"`
	}

	items := make([]userResponse, 0, len(users))
	for _, u := range users {
		id, err := models.RecordIDString(u.ID)
		if err != nil {
			logger.Warn("list users: extract user ID, skipping", "name", u.Name, "error", err)
			continue
		}
		items = append(items, userResponse{
			ID:            id,
			Name:          u.Name,
			Email:         u.Email,
			IsSystemAdmin: u.IsSystemAdmin,
			OIDCProvider:  u.OIDCProvider,
		})
	}

	writeJSON(w, http.StatusOK, httputil.NewListResponse(items, len(items)))
}

func (s *Server) createUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logutil.FromCtx(ctx)
	if err := auth.RequireSystemAdmin(ctx); err != nil {
		logger.Warn("admin access denied", "error", err)
		httputil.WriteProblem(w, http.StatusForbidden, "system admin required")
		return
	}

	body, ok := decodeBody[struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	}](w, r, 4096)
	if !ok {
		return
	}

	if body.Name == "" {
		httputil.WriteProblem(w, http.StatusBadRequest, "name is required")
		return
	}
	if body.Email == "" {
		httputil.WriteProblem(w, http.StatusBadRequest, "email is required")
		return
	}

	// Check for duplicate email
	existing, err := s.app.DBClient().GetUserByEmail(ctx, body.Email)
	if err != nil {
		httputil.WriteProblem(w, http.StatusInternalServerError, "failed to check existing user")
		logger.Error("check existing user", "error", err)
		return
	}
	if existing != nil {
		httputil.WriteProblem(w, http.StatusConflict, "user with this email already exists")
		return
	}

	user, vault, err := s.app.DBClient().ProvisionUser(ctx, models.UserInput{
		Name:  body.Name,
		Email: &body.Email,
	})
	if err != nil {
		httputil.WriteProblem(w, http.StatusInternalServerError, "failed to create user")
		logger.Error("provision user", "error", err)
		return
	}

	userID, err := models.RecordIDString(user.ID)
	if err != nil {
		httputil.WriteProblem(w, http.StatusInternalServerError, "invalid user ID")
		logger.Error("create user: extract user id", "error", err)
		return
	}
	vaultID, err := models.RecordIDString(vault.ID)
	if err != nil {
		httputil.WriteProblem(w, http.StatusInternalServerError, "invalid vault ID")
		logger.Error("create user: extract vault id", "error", err)
		return
	}

	logger.Info("admin created user", "user_id", userID, "email", body.Email, "vault_id", vaultID)

	writeJSON(w, http.StatusCreated, map[string]any{
		"user": map[string]any{
			"id":    userID,
			"name":  user.Name,
			"email": user.Email,
		},
		"vault": map[string]any{
			"id":   vaultID,
			"name": vault.Name,
		},
	})
}
