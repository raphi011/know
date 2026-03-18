package api

import (
	"net/http"

	"github.com/raphi011/know/internal/auth"
	"github.com/raphi011/know/internal/logutil"
	"github.com/raphi011/know/internal/models"
)

// VaultScopeHandler wraps an http.Handler with vault scope resolution.
// Exported for use by cmd_serve.go for protocol handlers (SSE events) that need
// vault scoping but aren't registered through Server.Register.
func (s *Server) VaultScopeHandler(next http.Handler) http.Handler {
	return s.vaultScope(next.ServeHTTP)
}

// vaultScope is a middleware that extracts the {vault} path parameter (vault name),
// resolves it to a bare vault ID via VaultService, checks at least RoleRead access,
// and injects the vault ID into the request context via auth.WithVaultID.
//
// Handlers retrieve the vault ID with auth.MustVaultIDFromCtx(ctx).
// Write/admin handlers should additionally call auth.RequireVaultRole for higher roles.
func (s *Server) vaultScope(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("vault")
		if name == "" {
			writeError(w, http.StatusBadRequest, "vault name required")
			return
		}

		ctx := r.Context()

		ac, err := auth.FromContext(ctx)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		v, err := s.app.VaultService().GetByName(ctx, name)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to resolve vault")
			logutil.FromCtx(ctx).Error("vault scope: get by name", "name", name, "error", err)
			return
		}
		if v == nil {
			writeError(w, http.StatusNotFound, "vault not found")
			return
		}

		vaultID, err := models.RecordIDString(v.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "invalid vault ID")
			logutil.FromCtx(ctx).Error("vault scope: extract ID", "name", name, "error", err)
			return
		}

		if err := auth.CheckVaultRole(ac, vaultID, models.RoleRead); err != nil {
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}

		ctx = auth.WithVaultID(ctx, vaultID)
		logger := logutil.FromCtx(ctx).With("vault_id", vaultID)
		ctx = logutil.WithLogger(ctx, logger)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
