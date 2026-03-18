package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/raphi011/know/internal/blob"
	"github.com/raphi011/know/internal/db"
	"github.com/raphi011/know/internal/logutil"
	"github.com/raphi011/know/internal/models"
)

// Export writes a manifest-based backup archive to w.
// The archive is a tar.gz containing:
//   - manifest.json — vault metadata, file metadata, folder settings, version history
//   - blobs/<sharded-hash> — content-addressed blobs (same layout as blob store)
//
// All content (files + versions) shares a single blob pool with automatic dedup.
func Export(ctx context.Context, dbClient *db.Client, blobStore blob.Store, vaultID string, w io.Writer) error {
	logger := logutil.FromCtx(ctx)

	vault, err := dbClient.GetVault(ctx, vaultID)
	if err != nil {
		return fmt.Errorf("get vault: %w", err)
	}
	if vault == nil {
		return fmt.Errorf("vault not found: %s", vaultID)
	}

	gzw := gzip.NewWriter(w)
	defer gzw.Close()
	tw := tar.NewWriter(gzw)
	defer tw.Close()

	manifest := Manifest{
		Version:    ManifestVersion,
		ExportedAt: time.Now().UTC(),
		Vault: VaultInfo{
			Name:        vault.Name,
			Description: vault.Description,
			Settings:    vault.Settings,
		},
	}

	// Collect folders.
	folders, err := dbClient.ListFolders(ctx, vaultID)
	if err != nil {
		return fmt.Errorf("list folders: %w", err)
	}
	for _, f := range folders {
		manifest.Folders = append(manifest.Folders, FolderInfo{
			Path:    f.Path,
			NoEmbed: f.NoEmbed,
		})
	}

	// Collect files and write blobs to archive.
	// Track file IDs for the version collection pass (avoids N+1 re-fetch).
	const pageSize = 1000
	isNotFolder := false
	writtenBlobs := make(map[string]bool)
	fileIDs := make(map[string]string) // path → file ID

	for offset := 0; ; offset += pageSize {
		files, err := dbClient.ListFiles(ctx, db.ListFilesFilter{
			VaultID:  vaultID,
			IsFolder: &isNotFolder,
			OrderBy:  db.OrderByPathAsc,
			Limit:    pageSize,
			Offset:   offset,
		})
		if err != nil {
			return fmt.Errorf("list files: %w", err)
		}

		for _, f := range files {
			fi := FileInfo{
				Path:      f.Path,
				Title:     f.Title,
				Size:      f.ContentLength,
				Labels:    f.Labels,
				DocType:   f.DocType,
				Metadata:  f.Metadata,
				MimeType:  f.MimeType,
				NoEmbed:   f.NoEmbed,
				CreatedAt: f.CreatedAt,
				UpdatedAt: f.UpdatedAt,
			}

			if f.ContentHash != nil {
				fi.ContentHash = *f.ContentHash
				if err := writeBlob(ctx, tw, blobStore, fi.ContentHash, writtenBlobs); err != nil {
					logger.Warn("skipping file, blob not readable", "path", f.Path, "hash", fi.ContentHash, "error", err)
					continue
				}
			}

			// Capture file ID for version lookup.
			if fileID, idErr := models.RecordIDString(f.ID); idErr == nil {
				fileIDs[f.Path] = fileID
			}

			manifest.Files = append(manifest.Files, fi)
		}

		if len(files) < pageSize {
			break
		}
	}

	// Collect all versions in a single batch query.
	allFileIDs := make([]string, 0, len(fileIDs))
	fileIDToPath := make(map[string]string, len(fileIDs)) // file ID → path
	for path, id := range fileIDs {
		allFileIDs = append(allFileIDs, id)
		fileIDToPath[id] = path
	}

	versions, err := dbClient.ListVersionsByFileIDs(ctx, allFileIDs)
	if err != nil {
		return fmt.Errorf("list versions: %w", err)
	}

	for _, v := range versions {
		vFileID, idErr := models.RecordIDString(v.File)
		if idErr != nil {
			logger.Warn("skipping version with bad file ID", "error", idErr)
			continue
		}
		filePath, ok := fileIDToPath[vFileID]
		if !ok {
			continue
		}

		manifest.Versions = append(manifest.Versions, VersionInfo{
			FilePath:    filePath,
			Version:     v.Version,
			ContentHash: v.ContentHash,
			Title:       v.Title,
			CreatedAt:   v.CreatedAt,
		})

		if err := writeBlob(ctx, tw, blobStore, v.ContentHash, writtenBlobs); err != nil {
			logger.Warn("skipping version blob", "path", filePath, "version", v.Version, "error", err)
		}
	}

	// Write manifest as the last entry.
	manifestJSON, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}
	if err := writeTarBytes(tw, "manifest.json", manifestJSON, manifest.ExportedAt); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}

	logger.Info("backup export complete",
		"vault", vault.Name,
		"files", len(manifest.Files),
		"folders", len(manifest.Folders),
		"versions", len(manifest.Versions),
		"blobs", len(writtenBlobs))

	return nil
}

// writeBlob writes a blob to the archive under blobs/<sharded-hash>.
// Skips if the hash was already written (dedup).
func writeBlob(ctx context.Context, tw *tar.Writer, store blob.Store, hash string, written map[string]bool) error {
	if hash == "" || written[hash] {
		return nil
	}

	rc, err := store.Get(ctx, hash)
	if err != nil {
		return fmt.Errorf("get blob %s: %w", hash, err)
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		return fmt.Errorf("read blob %s: %w", hash, err)
	}

	archivePath := "blobs/" + blob.ShardedKey(hash)
	if err := writeTarBytes(tw, archivePath, data, time.Time{}); err != nil {
		return err
	}

	written[hash] = true
	return nil
}

func writeTarBytes(tw *tar.Writer, path string, data []byte, modTime time.Time) error {
	path = strings.TrimPrefix(path, "/")
	if modTime.IsZero() {
		modTime = time.Now()
	}
	if err := tw.WriteHeader(&tar.Header{
		Name:    path,
		Size:    int64(len(data)),
		Mode:    0644,
		ModTime: modTime,
	}); err != nil {
		return fmt.Errorf("write header %s: %w", path, err)
	}
	if _, err := tw.Write(data); err != nil {
		return fmt.Errorf("write content %s: %w", path, err)
	}
	return nil
}
