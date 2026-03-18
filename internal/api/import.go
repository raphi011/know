package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"sync/atomic"

	"github.com/raphi011/know/internal/auth"
	"github.com/raphi011/know/internal/blob"
	"github.com/raphi011/know/internal/logutil"
	"github.com/raphi011/know/internal/models"
)

// countingReader wraps an io.Reader and counts the number of bytes read.
type countingReader struct {
	r io.Reader
	n atomic.Int64
}

func (cr *countingReader) Read(p []byte) (int, error) {
	n, err := cr.r.Read(p)
	cr.n.Add(int64(n))
	return n, err
}

// maxImportFileSize is the maximum size of a single file in an import upload (100 MB).
const maxImportFileSize = 100 << 20

// Import result status constants.
const (
	statusCreated = "created"
	statusUpdated = "updated"
	statusSkipped = "skipped"
	statusError   = "error"
)

// Import result reason constants.
const (
	reasonHashMatch    = "hash_match"
	reasonHashDiffers  = "hash_differs"
	reasonHashMismatch = "hash_mismatch"
)

// --- Manifest (Phase 1) ---

// importManifestFile is a single entry in the manifest request.
type importManifestFile struct {
	Path string `json:"path"`
	Hash string `json:"hash"`
}

// importManifestRequest is the request body for POST /api/import/manifest.
type importManifestRequest struct {
	Force  bool                 `json:"force"`
	DryRun bool                 `json:"dryRun"`
	Files  []importManifestFile `json:"files"`
}

// importManifestResponse is the response body for POST /api/import/manifest.
type importManifestResponse struct {
	Needed  []string           `json:"needed"`
	Results []importFileResult `json:"results"`
}

// importFileResult is the per-file status in import responses.
type importFileResult struct {
	Path   string `json:"path"`
	Status string `json:"status"`           // "created", "updated", "skipped", "error"
	Reason string `json:"reason,omitempty"` // e.g. "hash_match", "exists"
	Error  string `json:"error,omitempty"`
}

// importManifest handles POST /api/import/manifest.
// The client sends a list of files with their SHA256 hashes. The server checks
// each file against existing records and responds with which files need to be
// uploaded (phase 2) and which can be skipped.
func (s *Server) importManifest(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeBody[importManifestRequest](w, r, 10*1024*1024) // 10 MB max
	if !ok {
		return
	}
	ctx := r.Context()
	vaultID := auth.MustVaultIDFromCtx(ctx)
	if err := auth.RequireVaultRole(ctx, vaultID, models.RoleWrite); err != nil {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	logger := logutil.FromCtx(ctx)

	// Batch-query existing file metadata for all paths in one DB round-trip.
	paths := make([]string, len(req.Files))
	for i, f := range req.Files {
		paths[i] = f.Path
	}
	existingMap, err := s.app.DBClient().GetFileMetaByPaths(ctx, vaultID, paths)
	if err != nil {
		logger.Error("import manifest: batch query", "vault", vaultID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to check existing files")
		return
	}

	needed := []string{}
	results := []importFileResult{}

	for _, f := range req.Files {
		existing := existingMap[f.Path]

		if existing != nil {
			if existing.ContentHash != nil && *existing.ContentHash == f.Hash {
				results = append(results, importFileResult{Path: f.Path, Status: statusSkipped, Reason: reasonHashMatch})
				continue
			}
			// Hash unknown (nil) or differs — need upload unless force is off and hash is known.
			if existing.ContentHash != nil && !req.Force {
				results = append(results, importFileResult{Path: f.Path, Status: statusSkipped, Reason: reasonHashDiffers})
				continue
			}
			// ContentHash is nil (unknown) or force=true → need upload.
			if req.DryRun {
				results = append(results, importFileResult{Path: f.Path, Status: statusUpdated})
				continue
			}
			needed = append(needed, f.Path)
			continue
		}

		// File does not exist
		if req.DryRun {
			results = append(results, importFileResult{Path: f.Path, Status: statusCreated})
			continue
		}
		needed = append(needed, f.Path)
	}

	writeJSON(w, http.StatusOK, importManifestResponse{
		Needed:  needed,
		Results: results,
	})
}

// --- Upload (Phase 2) ---

// importUploadMeta is the JSON metadata in the first multipart part of an upload.
type importUploadMeta struct {
	Hashes map[string]string `json:"hashes"` // path → expected SHA256 hash
}

// importUpload handles POST /api/import/upload.
// The client sends only files that the manifest indicated as "needed".
// Binary files are streamed directly to the blob store with hash verification.
// Text files are buffered for markdown parsing. If any hash mismatch is detected,
// the import is aborted immediately — the client is faulty or malicious.
func (s *Server) importUpload(w http.ResponseWriter, r *http.Request) {
	reader, err := r.MultipartReader()
	if err != nil {
		writeError(w, http.StatusBadRequest, "expected multipart/form-data request")
		return
	}

	// First part must be "meta" with JSON metadata including per-file hashes.
	metaPart, err := reader.NextPart()
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing meta part")
		return
	}
	if metaPart.FormName() != "meta" {
		writeError(w, http.StatusBadRequest, "first part must be named 'meta'")
		return
	}

	var meta importUploadMeta
	if err := json.NewDecoder(metaPart).Decode(&meta); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid meta JSON: %v", err))
		return
	}
	ctx := r.Context()
	vaultID := auth.MustVaultIDFromCtx(ctx)
	if err := auth.RequireVaultRole(ctx, vaultID, models.RoleWrite); err != nil {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	logger := logutil.FromCtx(ctx)
	var results []importFileResult

	// Process remaining parts — each is a file identified by form name (vault path).
	var streamErr error
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			logger.Error("import upload: read part", "vault", vaultID, "error", err)
			streamErr = err
			break
		}

		path := part.FormName()
		if path == "" {
			io.Copy(io.Discard, part) //nolint:errcheck // drain unconsumed body to keep multipart stream intact
			results = append(results, importFileResult{Path: "(unknown)", Status: statusError, Error: "missing path in form name"})
			continue
		}

		expectedHash, ok := meta.Hashes[path]
		if !ok {
			io.Copy(io.Discard, part) //nolint:errcheck // drain unconsumed body to keep multipart stream intact
			results = append(results, importFileResult{Path: path, Status: statusError, Error: "no hash provided in meta for this file"})
			continue
		}

		result := s.processImportPart(ctx, part, path, expectedHash, vaultID)

		// Hash mismatch = abort entire import. The client is faulty or malicious.
		if result.Status == statusError && result.Reason == reasonHashMismatch {
			results = append(results, result)
			writeJSON(w, http.StatusBadRequest, importUploadResponse{
				Results: results,
				Error:   fmt.Sprintf("hash mismatch for %s: import aborted — client sent incorrect hash", path),
			})
			return
		}

		results = append(results, result)
	}

	if results == nil {
		results = []importFileResult{}
	}

	resp := importUploadResponse{Results: results}
	status := http.StatusOK
	if streamErr != nil {
		resp.Error = fmt.Sprintf("multipart read failed after %d files: %v — remaining files were not processed", len(results), streamErr)
		status = http.StatusInternalServerError
	}
	writeJSON(w, status, resp)
}

// importUploadResponse is the response body for POST /api/import/upload.
type importUploadResponse struct {
	Results []importFileResult `json:"results"`
	Error   string             `json:"error,omitempty"`
}

// processImportPart handles a single file part from the upload request.
// Binary files are streamed to the blob store; text files are buffered for parsing.
func (s *Server) processImportPart(ctx context.Context, part *multipart.Part, path, expectedHash, vaultID string) importFileResult {
	logger := logutil.FromCtx(ctx)
	limited := io.LimitReader(part, maxImportFileSize+1)

	if models.IsImageFile(path) || models.IsAudioFile(path) {
		cr := &countingReader{r: limited}
		result := s.processImportBinary(ctx, cr, path, expectedHash, vaultID)
		// Detect oversized files: LimitReader truncates silently, causing a hash mismatch.
		// If we read exactly maxImportFileSize+1 bytes, the file is too large.
		if cr.n.Load() > int64(maxImportFileSize) {
			return importFileResult{Path: path, Status: statusError, Error: "file exceeds maximum size of 100 MB"}
		}
		return result
	}

	// Text file (markdown) — must buffer for parsing
	data, err := io.ReadAll(limited)
	if err != nil {
		return importFileResult{Path: path, Status: statusError, Error: fmt.Sprintf("read file: %v", err)}
	}
	if len(data) > maxImportFileSize {
		return importFileResult{Path: path, Status: statusError, Error: "file exceeds maximum size of 100 MB"}
	}

	content := string(data)
	actualHash := models.ContentHash(content)
	if actualHash != expectedHash {
		logger.Warn("import: hash mismatch for text file", "path", path, "expected", expectedHash, "actual", actualHash)
		return importFileResult{Path: path, Status: statusError, Reason: reasonHashMismatch, Error: "content hash does not match declared hash"}
	}

	// Check if file already exists to determine created vs updated
	existing, err := s.app.DBClient().GetFileMetaByPath(ctx, vaultID, path)
	if err != nil {
		logger.Error("import: check existing document", "vault", vaultID, "path", path, "error", err)
		return importFileResult{Path: path, Status: statusError, Error: fmt.Sprintf("check existing: %v", err)}
	}

	_, err = s.app.FileService().Create(ctx, models.FileInput{
		VaultID: vaultID,
		Path:    path,
		Content: content,
	})
	if err != nil {
		logger.Error("import: upsert document", "vault", vaultID, "path", path, "error", err)
		return importFileResult{Path: path, Status: statusError, Error: fmt.Sprintf("create/update: %v", err)}
	}

	if existing != nil {
		return importFileResult{Path: path, Status: statusUpdated}
	}
	return importFileResult{Path: path, Status: statusCreated}
}

// processImportBinary handles a binary file (image/audio) by streaming it
// directly to the blob store with hash verification. No full buffering needed.
func (s *Server) processImportBinary(ctx context.Context, r io.Reader, path, expectedHash, vaultID string) importFileResult {
	logger := logutil.FromCtx(ctx)

	// Check if blob already exists (dedup across paths).
	exists, err := s.app.BlobStore().Exists(ctx, expectedHash)
	if err != nil {
		logger.Error("import: check blob exists", "path", path, "hash", expectedHash, "error", err)
		return importFileResult{Path: path, Status: statusError, Error: fmt.Sprintf("check blob: %v", err)}
	}

	if !exists {
		// Stream directly to blob store with hash verification.
		// PutVerified computes SHA256 during write and only commits if hash matches.
		if err := s.app.BlobStore().PutVerified(ctx, expectedHash, r, -1); err != nil {
			logger.Warn("import: store blob", "path", path, "hash", expectedHash, "error", err)
			// Check if it's a hash mismatch error
			if blob.IsHashMismatch(err) {
				return importFileResult{Path: path, Status: statusError, Reason: reasonHashMismatch, Error: "content hash does not match declared hash"}
			}
			return importFileResult{Path: path, Status: statusError, Error: fmt.Sprintf("store blob: %v", err)}
		}
	} else {
		// Blob exists — still need to verify the client's data matches the hash.
		// Read and hash the data to verify, then discard.
		h := sha256.New()
		if _, err := io.Copy(h, r); err != nil {
			return importFileResult{Path: path, Status: statusError, Error: fmt.Sprintf("read file for verification: %v", err)}
		}
		actualHash := hex.EncodeToString(h.Sum(nil))
		if actualHash != expectedHash {
			logger.Warn("import: hash mismatch for existing blob", "path", path, "expected", expectedHash, "actual", actualHash)
			return importFileResult{Path: path, Status: statusError, Reason: reasonHashMismatch, Error: "content hash does not match declared hash"}
		}
	}

	// Check if file metadata exists to determine created vs updated
	existing, err := s.app.DBClient().GetFileMetaByPath(ctx, vaultID, path)
	if err != nil {
		logger.Error("import: check existing asset", "vault", vaultID, "path", path, "error", err)
		return importFileResult{Path: path, Status: statusError, Error: fmt.Sprintf("check existing: %v", err)}
	}

	// Determine the size of the streamed data for DB metadata.
	var size int
	if cr, ok := r.(*countingReader); ok {
		size = int(cr.n.Load())
	}

	_, err = s.app.FileService().CreateBinaryFromHash(ctx, models.FileInput{
		VaultID:     vaultID,
		Path:        path,
		ContentHash: &expectedHash,
		MimeType:    models.DetectMimeType(path),
		Size:        size,
	})
	if err != nil {
		logger.Error("import: upsert asset", "vault", vaultID, "path", path, "error", err)
		return importFileResult{Path: path, Status: statusError, Error: fmt.Sprintf("create/update: %v", err)}
	}

	if existing != nil {
		return importFileResult{Path: path, Status: statusUpdated}
	}
	return importFileResult{Path: path, Status: statusCreated}
}
