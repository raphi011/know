package api

import (
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"

	"github.com/raphi011/know/internal/auth"
	"github.com/raphi011/know/internal/logutil"
	"github.com/raphi011/know/internal/models"
)

// bulkMeta is the JSON metadata sent as the first multipart part ("meta").
type bulkMeta struct {
	VaultID string `json:"vaultId"`
	Force   bool   `json:"force"`
	DryRun  bool   `json:"dryRun"`
}

// bulkFileResult is the per-file result in the response.
type bulkFileResult struct {
	Path   string `json:"path"`
	Status string `json:"status"`           // "created", "updated", "skipped", "error"
	Reason string `json:"reason,omitempty"` // e.g. "hash_match", "exists"
	Error  string `json:"error,omitempty"`
}

// bulkResponse is the response body for POST /api/bulk.
type bulkResponse struct {
	Results []bulkFileResult `json:"results"`
	Error   string           `json:"error,omitempty"`
}

func (s *Server) bulkUpload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 50*1024*1024) // 50 MB
	// Use streaming multipart reader to avoid buffering the entire request in memory.
	reader, err := r.MultipartReader()
	if err != nil {
		writeError(w, http.StatusBadRequest, "expected multipart/form-data request")
		return
	}

	// First part must be "meta" with JSON metadata.
	metaPart, err := reader.NextPart()
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing meta part")
		return
	}
	if metaPart.FormName() != "meta" {
		writeError(w, http.StatusBadRequest, "first part must be named 'meta'")
		return
	}

	var meta bulkMeta
	if err := json.NewDecoder(metaPart).Decode(&meta); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid meta JSON: %v", err))
		return
	}
	if meta.VaultID == "" {
		writeError(w, http.StatusBadRequest, "vaultId is required in meta")
		return
	}

	if err := auth.RequireVaultRole(r.Context(), meta.VaultID, models.RoleWrite); err != nil {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	var results []bulkFileResult

	// Process remaining parts — each is a file.
	var loopErr error
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			logutil.FromCtx(r.Context()).Error("bulk upload: read part", "vault", meta.VaultID, "error", err)
			loopErr = err
			break
		}

		result := s.processBulkPart(r, part, meta)
		results = append(results, result)
	}

	if results == nil {
		results = []bulkFileResult{}
	}

	resp := bulkResponse{Results: results}
	if loopErr != nil {
		resp.Error = "upload interrupted: not all files were processed"
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) processBulkPart(r *http.Request, part *multipart.Part, meta bulkMeta) bulkFileResult {
	path := part.FormName()
	if path == "" {
		return bulkFileResult{Path: "(unknown)", Status: "error", Error: "missing path in form name"}
	}

	data, err := io.ReadAll(part)
	if err != nil {
		return bulkFileResult{Path: path, Status: "error", Error: fmt.Sprintf("read file: %v", err)}
	}

	isImage := models.IsImageFile(path)

	if isImage {
		return s.processBulkAsset(r, path, data, meta)
	}
	return s.processBulkDocument(r, path, string(data), meta)
}

func (s *Server) processBulkDocument(r *http.Request, path, content string, meta bulkMeta) bulkFileResult {
	logger := logutil.FromCtx(r.Context())
	hash := models.ContentHash(content)

	existing, err := s.app.DBClient().GetFileByPath(r.Context(), meta.VaultID, path)
	if err != nil {
		logger.Error("bulk: check existing document", "vault", meta.VaultID, "path", path, "error", err)
		return bulkFileResult{Path: path, Status: "error", Error: fmt.Sprintf("check existing document: %v", err)}
	}

	if existing != nil {
		if existing.ContentHash != nil && *existing.ContentHash == hash {
			return bulkFileResult{Path: path, Status: "skipped", Reason: "hash_match"}
		}
		if !meta.Force {
			return bulkFileResult{Path: path, Status: "skipped", Reason: "exists"}
		}
	}

	if meta.DryRun {
		if existing != nil {
			return bulkFileResult{Path: path, Status: "updated"}
		}
		return bulkFileResult{Path: path, Status: "created"}
	}

	_, err = s.app.FileService().Create(r.Context(), models.FileInput{
		VaultID: meta.VaultID,
		Path:    path,
		Content: content,
	})
	if err != nil {
		logger.Error("bulk: upsert document", "vault", meta.VaultID, "path", path, "error", err)
		return bulkFileResult{Path: path, Status: "error", Error: fmt.Sprintf("create/update document: %v", err)}
	}

	if existing != nil {
		return bulkFileResult{Path: path, Status: "updated"}
	}
	return bulkFileResult{Path: path, Status: "created"}
}

func (s *Server) processBulkAsset(r *http.Request, path string, data []byte, meta bulkMeta) bulkFileResult {
	logger := logutil.FromCtx(r.Context())
	hash := models.ContentHash(string(data))

	existing, err := s.app.DBClient().GetFileMetaByPath(r.Context(), meta.VaultID, path)
	if err != nil {
		logger.Error("bulk: check existing asset", "vault", meta.VaultID, "path", path, "error", err)
		return bulkFileResult{Path: path, Status: "error", Error: fmt.Sprintf("check existing asset: %v", err)}
	}

	if existing != nil {
		if existing.ContentHash != nil && *existing.ContentHash == hash {
			return bulkFileResult{Path: path, Status: "skipped", Reason: "hash_match"}
		}
		if !meta.Force {
			return bulkFileResult{Path: path, Status: "skipped", Reason: "exists"}
		}
	}

	if meta.DryRun {
		if existing != nil {
			return bulkFileResult{Path: path, Status: "updated"}
		}
		return bulkFileResult{Path: path, Status: "created"}
	}

	_, err = s.app.FileService().Create(r.Context(), models.FileInput{
		VaultID: meta.VaultID,
		Path:    path,
		Data:    data,
	})
	if err != nil {
		logger.Error("bulk: upload asset", "vault", meta.VaultID, "path", path, "error", err)
		return bulkFileResult{Path: path, Status: "error", Error: fmt.Sprintf("upload asset: %v", err)}
	}

	if existing != nil {
		return bulkFileResult{Path: path, Status: "updated"}
	}
	return bulkFileResult{Path: path, Status: "created"}
}
