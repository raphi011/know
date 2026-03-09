package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/raphi011/knowhow/internal/auth"
	"github.com/raphi011/knowhow/internal/models"
)

func (s *Server) getDocument(w http.ResponseWriter, r *http.Request) {
	vaultID := r.URL.Query().Get("vault")
	path := r.URL.Query().Get("path")

	if vaultID == "" || path == "" {
		writeError(w, http.StatusBadRequest, "vault and path query parameters required")
		return
	}

	if err := auth.RequireVaultRole(r.Context(), vaultID, models.RoleRead); err != nil {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	doc, err := s.app.DBClient().GetDocumentByPath(r.Context(), vaultID, path)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get document")
		slog.Error("get document", "error", err)
		return
	}
	if doc == nil {
		writeError(w, http.StatusNotFound, "document not found")
		return
	}

	writeJSON(w, http.StatusOK, documentFromModel(doc))
}

type upsertDocumentRequest struct {
	VaultID string `json:"vaultId"`
	Path    string `json:"path"`
	Content string `json:"content"`
	Source  string `json:"source"`
}

func (s *Server) upsertDocument(w http.ResponseWriter, r *http.Request) {
	var body upsertDocumentRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.VaultID == "" || body.Path == "" {
		writeError(w, http.StatusBadRequest, "vaultId and path are required")
		return
	}

	if err := auth.RequireVaultRole(r.Context(), body.VaultID, models.RoleWrite); err != nil {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	src := models.SourceManual
	if body.Source != "" {
		src = models.DocumentSource(body.Source)
		if !src.Valid() {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid document source: %q", body.Source))
			return
		}
	}

	doc, err := s.app.DocumentService().Create(r.Context(), models.DocumentInput{
		VaultID: body.VaultID,
		Path:    body.Path,
		Content: body.Content,
		Source:  src,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create/update document")
		slog.Error("upsert document", "error", err)
		return
	}

	writeJSON(w, http.StatusOK, documentFromModel(doc))
}

func documentFromModel(d *models.Document) Document {
	id, err := models.RecordIDString(d.ID)
	if err != nil {
		slog.Warn("unexpected document ID format", "path", d.Path, "error", err)
		id = fmt.Sprintf("%v", d.ID.ID)
	}
	vaultID, err := models.RecordIDString(d.Vault)
	if err != nil {
		slog.Warn("unexpected document vault ID format", "path", d.Path, "error", err)
		vaultID = fmt.Sprintf("%v", d.Vault.ID)
	}
	return Document{
		ID:          id,
		VaultID:     vaultID,
		Path:        d.Path,
		Title:       d.Title,
		Content:     d.Content,
		Source:      string(d.Source),
		ContentHash: d.ContentHash,
		CreatedAt:   d.CreatedAt,
		UpdatedAt:   d.UpdatedAt,
	}
}
