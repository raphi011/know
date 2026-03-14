package api

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/raphi011/know/internal/auth"
	"github.com/raphi011/know/internal/logutil"
	"github.com/raphi011/know/internal/models"
)

func (s *Server) listVaults(w http.ResponseWriter, r *http.Request) {
	ac, err := auth.FromContext(r.Context())
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	all, err := s.app.VaultService().List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list vaults")
		logutil.FromCtx(r.Context()).Error("list vaults", "error", err)
		return
	}

	var result []Vault

	// System admin or wildcard → return all vaults
	if ac.IsSystemAdmin {
		result = make([]Vault, len(all))
		for i := range all {
			result[i] = vaultFromModel(&all[i])
		}
	} else {
		// Filter to vaults the user has access to
		accessSet := make(map[string]bool, len(ac.Vaults))
		hasWildcard := false
		for _, vp := range ac.Vaults {
			if vp.VaultID == auth.WildcardVaultAccess {
				hasWildcard = true
				break
			}
			accessSet[vp.VaultID] = true
		}

		if hasWildcard {
			result = make([]Vault, len(all))
			for i := range all {
				result[i] = vaultFromModel(&all[i])
			}
		} else {
			logger := logutil.FromCtx(r.Context())
			for i := range all {
				id, err := models.RecordIDString(all[i].ID)
				if err != nil {
					logger.Warn("failed to extract vault ID, skipping", "vault_name", all[i].Name, "error", err)
					continue
				}
				if accessSet[id] {
					result = append(result, vaultFromModel(&all[i]))
				}
			}
		}
	}

	// Append remote vaults if remote service is configured
	if remoteSvc := s.app.RemoteService(); remoteSvc != nil {
		remoteVaults, err := remoteSvc.ListRemoteVaults(r.Context())
		if err != nil {
			logutil.FromCtx(r.Context()).Warn("failed to list remote vaults", "error", err)
		} else {
			for _, rv := range remoteVaults {
				remoteName := rv.RemoteName
				result = append(result, Vault{
					ID:     rv.VaultID,
					Name:   rv.Namespace,
					Remote: &remoteName,
				})
			}
		}
	}

	if result == nil {
		result = []Vault{}
	}
	writeJSON(w, http.StatusOK, result)
}

// resolveVault looks up a vault by the "name" path parameter, extracts its ID,
// and checks that the caller has at least minRole. Returns the vault, its bare ID,
// and true on success. On failure it writes the HTTP error and returns false.
func (s *Server) resolveVault(w http.ResponseWriter, r *http.Request, minRole models.VaultRole) (*models.Vault, string, bool) {
	name := r.PathValue("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "vault name required")
		return nil, "", false
	}

	logger := logutil.FromCtx(r.Context())

	v, err := s.app.VaultService().GetByName(r.Context(), name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to resolve vault")
		logger.Error("get vault by name", "name", name, "error", err)
		return nil, "", false
	}
	if v == nil {
		writeError(w, http.StatusNotFound, "vault not found")
		return nil, "", false
	}

	vaultID, err := models.RecordIDString(v.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "invalid vault ID")
		logger.Error("extract vault ID", "name", name, "error", err)
		return nil, "", false
	}

	if err := auth.RequireVaultRole(r.Context(), vaultID, minRole); err != nil {
		writeError(w, http.StatusForbidden, "forbidden")
		return nil, "", false
	}

	return v, vaultID, true
}

func (s *Server) getVaultInfo(w http.ResponseWriter, r *http.Request) {
	v, vaultID, ok := s.resolveVault(w, r, models.RoleRead)
	if !ok {
		return
	}

	stats, err := s.app.DBClient().GetVaultInfo(r.Context(), vaultID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get vault info")
		logutil.FromCtx(r.Context()).Error("get vault info", "vault_id", vaultID, "error", err)
		return
	}

	info := VaultInfo{
		Name:               v.Name,
		Description:        v.Description,
		CreatedAt:          v.CreatedAt,
		UpdatedAt:          v.UpdatedAt,
		DocumentCount:      stats.DocumentCount,
		UnprocessedDocs:    stats.UnprocessedDocs,
		ChunkTotal:         stats.ChunkTotal,
		ChunkWithEmbedding: stats.ChunkWithEmbedding,
		ChunkPending:       stats.ChunkPending,
		LabelCount:         stats.LabelCount,
		TopLabels:          stats.TopLabels,
		Members:            stats.Members,
		AssetCount:         stats.AssetCount,
		AssetTotalSize:     stats.AssetTotalSize,
		WikiLinkTotal:      stats.WikiLinkTotal,
		WikiLinkBroken:     stats.WikiLinkBroken,
		VersionCount:       stats.VersionCount,
		ConversationCount:  stats.ConversationCount,
		TokenInput:         stats.TokenInput,
		TokenOutput:        stats.TokenOutput,
	}

	writeJSON(w, http.StatusOK, info)
}

func (s *Server) getVaultSettings(w http.ResponseWriter, r *http.Request) {
	v, _, ok := s.resolveVault(w, r, models.RoleRead)
	if !ok {
		return
	}

	writeJSON(w, http.StatusOK, v.Defaults())
}

func (s *Server) updateVaultSettings(w http.ResponseWriter, r *http.Request) {
	patch, ok := decodeBody[models.VaultSettings](w, r, 64*1024)
	if !ok {
		return
	}

	if err := patch.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	v, vaultID, ok := s.resolveVault(w, r, models.RoleAdmin)
	if !ok {
		return
	}

	merged := v.Defaults().Merge(*patch)

	updated, err := s.app.DBClient().UpdateVaultSettings(r.Context(), vaultID, merged)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update settings")
		logutil.FromCtx(r.Context()).Error("update vault settings", "vault_id", vaultID, "error", err)
		return
	}

	writeJSON(w, http.StatusOK, updated.Defaults())
}

func vaultFromModel(v *models.Vault) Vault {
	id, err := models.RecordIDString(v.ID)
	if err != nil {
		slog.Warn("unexpected vault ID format", "vault_name", v.Name, "error", err)
		id = fmt.Sprintf("%v", v.ID.ID)
	}
	createdBy, err := models.RecordIDString(v.CreatedBy)
	if err != nil {
		slog.Warn("unexpected vault createdBy format", "vault_name", v.Name, "error", err)
		createdBy = fmt.Sprintf("%v", v.CreatedBy.ID)
	}
	return Vault{
		ID:          id,
		Name:        v.Name,
		Description: v.Description,
		CreatedBy:   createdBy,
		CreatedAt:   v.CreatedAt,
		UpdatedAt:   v.UpdatedAt,
	}
}
