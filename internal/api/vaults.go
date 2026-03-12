package api

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/raphi011/knowhow/internal/auth"
	"github.com/raphi011/knowhow/internal/logutil"
	"github.com/raphi011/knowhow/internal/models"
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

	// System admin or wildcard → return all vaults
	if ac.IsSystemAdmin {
		result := make([]Vault, len(all))
		for i := range all {
			result[i] = vaultFromModel(&all[i])
		}
		writeJSON(w, http.StatusOK, result)
		return
	}

	// Filter to vaults the user has access to
	accessSet := make(map[string]bool, len(ac.Vaults))
	for _, vp := range ac.Vaults {
		if vp.VaultID == auth.WildcardVaultAccess {
			result := make([]Vault, len(all))
			for i := range all {
				result[i] = vaultFromModel(&all[i])
			}
			writeJSON(w, http.StatusOK, result)
			return
		}
		accessSet[vp.VaultID] = true
	}

	logger := logutil.FromCtx(r.Context())

	var result []Vault
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
	if result == nil {
		result = []Vault{}
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) getVaultInfo(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "vault name required")
		return
	}

	logger := logutil.FromCtx(r.Context())

	v, err := s.app.VaultService().GetByName(r.Context(), name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to resolve vault")
		logger.Error("get vault by name", "name", name, "error", err)
		return
	}
	if v == nil {
		writeError(w, http.StatusNotFound, "vault not found")
		return
	}

	vaultID, err := models.RecordIDString(v.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "invalid vault ID")
		logger.Error("extract vault ID", "name", name, "error", err)
		return
	}

	if err := auth.RequireVaultRole(r.Context(), vaultID, models.RoleRead); err != nil {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	stats, err := s.app.DBClient().GetVaultInfo(r.Context(), vaultID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get vault info")
		logger.Error("get vault info", "vault_id", vaultID, "error", err)
		return
	}

	topLabels := make([]LabelStat, len(stats.TopLabels))
	for i, l := range stats.TopLabels {
		topLabels[i] = LabelStat{Name: l.Name, Count: l.Count}
	}
	members := make([]MemberStat, len(stats.Members))
	for i, m := range stats.Members {
		members[i] = MemberStat{Name: m.Name, Role: m.Role}
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
		TopLabels:          topLabels,
		Members:            members,
		AssetCount:         stats.AssetCount,
		AssetTotalSize:     stats.AssetTotalSize,
		WikiLinkTotal:      stats.WikiLinkTotal,
		WikiLinkBroken:     stats.WikiLinkBroken,
		TemplateCount:      stats.TemplateCount,
		VersionCount:       stats.VersionCount,
		ConversationCount:  stats.ConversationCount,
		TokenInput:         stats.TokenInput,
		TokenOutput:        stats.TokenOutput,
	}

	writeJSON(w, http.StatusOK, info)
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
