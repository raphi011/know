package api

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/raphi011/know/internal/auth"
	"github.com/raphi011/know/internal/db"
	"github.com/raphi011/know/internal/logutil"
	"github.com/raphi011/know/internal/models"
)

func (s *Server) export(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vaultID := auth.MustVaultIDFromCtx(ctx)

	logger := logutil.FromCtx(ctx)
	dbClient := s.app.DBClient()

	// Phase 1: Build manifest — collect all paths without content.
	const pageSize = 1000

	isNotFolder := false
	var fileMetas []models.FileMeta
	for offset := 0; ; offset += pageSize {
		batch, err := dbClient.ListFileMetas(ctx, db.ListFilesFilter{
			VaultID:  vaultID,
			IsFolder: &isNotFolder,
			OrderBy:  db.OrderByPathAsc,
			Limit:    pageSize,
			Offset:   offset,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("list files: %v", err))
			return
		}
		fileMetas = append(fileMetas, batch...)
		if len(batch) < pageSize {
			break
		}
	}

	// Phase 2: Write tar.gz to temp file.
	tmp, err := os.CreateTemp("", "know-export-*.tar.gz")
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("create temp file: %v", err))
		return
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()

	gzw := gzip.NewWriter(tmp)
	tw := tar.NewWriter(gzw)

	for _, meta := range fileMetas {
		f, err := dbClient.GetFileByPath(ctx, vaultID, meta.Path)
		if err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("get file %s: %v", meta.Path, err))
			return
		}
		if f == nil {
			logger.Warn("file deleted since manifest", "path", meta.Path)
			continue
		}
		var data []byte
		if f.ContentHash != nil {
			rc, err := s.app.BlobStore().Get(ctx, *f.ContentHash)
			if err != nil {
				writeError(w, http.StatusInternalServerError, fmt.Sprintf("read blob for %s: %v", meta.Path, err))
				return
			}
			data, err = io.ReadAll(rc)
			rc.Close()
			if err != nil {
				writeError(w, http.StatusInternalServerError, fmt.Sprintf("read blob data for %s: %v", meta.Path, err))
				return
			}
		}
		if err := writeTarEntry(tw, f.Path, data, f.UpdatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("write file %s: %v", meta.Path, err))
			return
		}
	}

	if err := tw.Close(); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("close tar writer: %v", err))
		return
	}
	if err := gzw.Close(); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("close gzip writer: %v", err))
		return
	}

	// Phase 3: Serve completed file.
	if _, err := tmp.Seek(0, 0); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("seek temp file: %v", err))
		return
	}

	bareVault := models.BareID("vault", vaultID)
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="know-export-%s.tar.gz"`, bareVault))
	http.ServeContent(w, r, "", time.Time{}, tmp)
}

func writeTarEntry(tw *tar.Writer, path string, data []byte, modTime time.Time) error {
	if err := tw.WriteHeader(&tar.Header{
		Name:    strings.TrimPrefix(path, "/"),
		Size:    int64(len(data)),
		Mode:    0644,
		ModTime: modTime,
	}); err != nil {
		return fmt.Errorf("write header: %w", err)
	}
	if _, err := tw.Write(data); err != nil {
		return fmt.Errorf("write content: %w", err)
	}
	return nil
}
