package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/raphi011/know/internal/blob"
	"github.com/raphi011/know/internal/db"
	"github.com/raphi011/know/internal/logutil"
	"github.com/raphi011/know/internal/models"
	"github.com/raphi011/know/internal/vault"
)

// Restore reads a manifest-based backup archive from r and restores it.
// Blobs are copied directly into the blob store (no re-hashing), then
// DB records are created from the manifest metadata. Pipeline jobs are
// enqueued for text files so chunks and embeddings are regenerated.
func Restore(ctx context.Context, dbClient *db.Client, blobStore blob.Store, vaultSvc *vault.Service, userID string, r io.Reader) error {
	logger := logutil.FromCtx(ctx)

	gzr, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("decompress archive: %w", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)

	// Stream archive: store blobs directly, buffer only the manifest.
	var manifestData []byte
	blobCount := 0

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read tar entry: %w", err)
		}

		switch {
		case header.Name == "manifest.json":
			manifestData, err = io.ReadAll(tr)
			if err != nil {
				return fmt.Errorf("read manifest: %w", err)
			}

		case strings.HasPrefix(header.Name, "blobs/"):
			hash := extractHashFromBlobPath(header.Name)
			if hash == "" {
				logger.Warn("skipping blob with unparseable path", "path", header.Name)
				continue
			}
			if err := blobStore.Put(ctx, hash, io.LimitReader(tr, header.Size), header.Size); err != nil {
				return fmt.Errorf("store blob %s: %w", hash, err)
			}
			blobCount++
		}
	}

	if manifestData == nil {
		return fmt.Errorf("manifest.json not found in archive")
	}

	var manifest Manifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return fmt.Errorf("parse manifest: %w", err)
	}
	if manifest.Version != ManifestVersion {
		return fmt.Errorf("unsupported manifest version %d (expected %d)", manifest.Version, ManifestVersion)
	}

	logger.Info("backup blobs restored", "count", blobCount)

	// Create or get vault. Try lookup first to avoid masking real errors
	// from Create (e.g. DB connection failures).
	v, err := dbClient.GetVaultByName(ctx, manifest.Vault.Name)
	if err != nil {
		return fmt.Errorf("lookup vault %q: %w", manifest.Vault.Name, err)
	}
	if v == nil {
		v, err = vaultSvc.Create(ctx, userID, models.VaultInput{
			Name:        manifest.Vault.Name,
			Description: manifest.Vault.Description,
		})
		if err != nil {
			return fmt.Errorf("create vault %q: %w", manifest.Vault.Name, err)
		}
	}
	vaultID, err := models.RecordIDString(v.ID)
	if err != nil {
		return fmt.Errorf("extract vault id: %w", err)
	}

	// Apply vault settings.
	if manifest.Vault.Settings != nil {
		if _, err := dbClient.UpdateVaultSettings(ctx, vaultID, *manifest.Vault.Settings); err != nil {
			logger.Warn("failed to apply vault settings", "error", err)
		}
	}

	// Create folders (with no_embed flags).
	for _, folder := range manifest.Folders {
		if err := dbClient.EnsureFolderPath(ctx, vaultID, folder.Path); err != nil {
			logger.Warn("failed to create folder", "path", folder.Path, "error", err)
			continue
		}
		if folder.NoEmbed {
			if err := dbClient.SetFolderNoEmbed(ctx, vaultID, folder.Path, true); err != nil {
				logger.Warn("failed to set no_embed on folder", "path", folder.Path, "error", err)
			}
		}
	}

	// Create file records (blobs already in store, folders already created).
	fileIDs := make(map[string]string, len(manifest.Files)) // path → file ID
	filesRestored := 0
	for _, fi := range manifest.Files {
		input := models.FileInput{
			VaultID:       vaultID,
			Path:          fi.Path,
			Title:         fi.Title,
			ContentHash:   strPtr(fi.ContentHash),
			ContentLength: fi.Size,
			Labels:        fi.Labels,
			DocType:       fi.DocType,
			Metadata:      fi.Metadata,
			MimeType:      fi.MimeType,
		}

		file, err := dbClient.CreateFile(ctx, input)
		if err != nil {
			logger.Warn("failed to create file record", "path", fi.Path, "error", err)
			continue
		}

		fileID, idErr := models.RecordIDString(file.ID)
		if idErr != nil {
			logger.Warn("failed to extract file id", "path", fi.Path, "error", idErr)
			continue
		}
		fileIDs[fi.Path] = fileID

		// Enqueue pipeline job for text files so chunks/embeddings are generated.
		if models.IsTextFile(fi.Path) {
			if err := dbClient.CreateJob(ctx, fileID, "parse", 0); err != nil {
				logger.Warn("failed to enqueue parse job", "path", fi.Path, "error", err)
			}
		}

		filesRestored++
	}

	// Restore version records using cached file IDs (no N+1).
	versionsRestored := 0
	for _, vi := range manifest.Versions {
		fileID, ok := fileIDs[vi.FilePath]
		if !ok {
			logger.Warn("file not found for version", "path", vi.FilePath)
			continue
		}

		if _, err := dbClient.CreateVersion(ctx, models.FileVersionInput{
			FileID:      fileID,
			VaultID:     vaultID,
			ContentHash: vi.ContentHash,
			Title:       vi.Title,
		}, vi.Version); err != nil {
			logger.Warn("failed to restore version", "path", vi.FilePath, "version", vi.Version, "error", err)
			continue
		}
		versionsRestored++
	}

	logger.Info("backup restore complete",
		"vault", manifest.Vault.Name,
		"files", filesRestored,
		"versions", versionsRestored,
		"blobs", blobCount)

	return nil
}

// extractHashFromBlobPath extracts the hash from a sharded blob path.
// "blobs/ab/cd/abcdef1234..." → "abcdef1234..."
func extractHashFromBlobPath(archivePath string) string {
	rest := strings.TrimPrefix(archivePath, "blobs/")
	parts := strings.Split(rest, "/")
	if len(parts) < 3 {
		return ""
	}
	return parts[len(parts)-1]
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
