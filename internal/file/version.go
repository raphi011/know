package file

import (
	"context"
	"fmt"
	"time"

	"github.com/raphi011/know/internal/logutil"
	"github.com/raphi011/know/internal/models"
)

// resolveVersionConfig returns a VersionConfig that uses vault-specific overrides
// when explicitly set, falling back to the service-wide defaults.
// Checks raw vault.Settings (not Defaults()) so that hardcoded model defaults
// don't override the service-level config from env vars.
func (s *Service) resolveVersionConfig(ctx context.Context, vaultID string) VersionConfig {
	vc := s.versionConfig
	vault, err := s.db.GetVault(ctx, vaultID)
	if err != nil {
		logutil.FromCtx(ctx).Warn("failed to load vault for version config, using defaults", "vault_id", vaultID, "error", err)
		return vc
	}
	if vault.Settings == nil {
		return vc
	}
	if vault.Settings.VersionCoalesceMinutes > 0 {
		vc.CoalesceMinutes = vault.Settings.VersionCoalesceMinutes
	}
	if vault.Settings.VersionRetentionCount > 0 {
		vc.RetentionCount = vault.Settings.VersionRetentionCount
	}
	return vc
}

// maybeCreateVersion snapshots the old file content as a version if the
// coalescing window has elapsed and the content actually changed.
func (s *Service) maybeCreateVersion(ctx context.Context, fileID, vaultID string, oldFile *models.File, newContentHash string) {
	logger := logutil.FromCtx(ctx)

	// Skip if content hasn't actually changed
	if oldFile.Hash != nil && *oldFile.Hash == newContentHash {
		return
	}

	vc := s.resolveVersionConfig(ctx, vaultID)

	// Coalescing: skip version if last one is too recent (0 = disabled)
	if vc.CoalesceMinutes > 0 {
		latest, err := s.db.GetLatestVersion(ctx, fileID)
		if err != nil {
			logger.Warn("failed to check latest version for coalescing", "file_id", fileID, "error", err)
			return
		}
		if latest != nil {
			threshold := time.Now().Add(-time.Duration(vc.CoalesceMinutes) * time.Minute)
			if latest.CreatedAt.After(threshold) {
				return
			}
		}
	}

	// Get next version number
	nextVersion, err := s.db.NextVersionNumber(ctx, fileID)
	if err != nil {
		logger.Warn("failed to get next version number", "file_id", fileID, "error", err)
		return
	}

	// Create version snapshot — content blob already exists from when the file was created.
	var hash string
	if oldFile.Hash != nil {
		hash = *oldFile.Hash
	}
	if _, err := s.db.CreateVersion(ctx, models.FileVersionInput{
		FileID:  fileID,
		VaultID: vaultID,
		Hash:    hash,
		Title:   oldFile.Title,
	}, nextVersion); err != nil {
		logger.Warn("failed to create version snapshot", "file_id", fileID, "version", nextVersion, "error", err)
		return
	}

	logger.Info("created version snapshot", "file_id", fileID, "version", nextVersion)

	// Enforce retention cap
	s.enforceRetention(ctx, fileID, vc)
}

// enforceRetention deletes versions beyond the retention cap.
func (s *Service) enforceRetention(ctx context.Context, fileID string, vc VersionConfig) {
	logger := logutil.FromCtx(ctx)
	deleted, err := s.db.DeleteOldestVersions(ctx, fileID, vc.RetentionCount)
	if err != nil {
		logger.Warn("failed to enforce version retention", "file_id", fileID, "error", err)
		return
	}
	if deleted > 0 {
		logger.Info("pruned old versions", "file_id", fileID, "deleted", deleted)
	}
}

// Rollback restores a file to a previous version's content.
// Goes through the normal Create() pipeline, which triggers version creation
// of the pre-rollback state, syncChunks, wiki-link resolution, etc.
func (s *Service) Rollback(ctx context.Context, vaultID, fileID, versionID string) (*models.File, error) {
	version, err := s.db.GetVersion(ctx, versionID)
	if err != nil {
		return nil, fmt.Errorf("get version: %w", err)
	}
	if version == nil {
		return nil, fmt.Errorf("version not found: %s", versionID)
	}

	versionFileID, err := models.RecordIDString(version.File)
	if err != nil {
		return nil, fmt.Errorf("extract file ID from version: %w", err)
	}
	if versionFileID != fileID {
		return nil, fmt.Errorf("version %s does not belong to file %s", versionID, fileID)
	}

	doc, err := s.db.GetFileByID(ctx, fileID)
	if err != nil {
		return nil, fmt.Errorf("get file: %w", err)
	}
	if doc == nil {
		return nil, fmt.Errorf("file not found: %s", fileID)
	}

	// Load version content from blob store
	content, err := s.ReadContent(ctx, version.Hash)
	if err != nil {
		return nil, fmt.Errorf("read version content: %w", err)
	}

	// Go through normal save pipeline
	updated, err := s.Create(ctx, models.FileInput{
		VaultID: vaultID,
		Path:    doc.Path,
		Content: content,
	})
	if err != nil {
		return nil, fmt.Errorf("apply rollback: %w", err)
	}

	return updated, nil
}

// VersionContent resolves the content for a version ID, or the current file
// content if versionID is nil. Content is loaded from the blob store.
func (s *Service) VersionContent(ctx context.Context, fileID string, versionID *string) (string, error) {
	if versionID != nil {
		v, err := s.db.GetVersion(ctx, *versionID)
		if err != nil {
			return "", fmt.Errorf("get version: %w", err)
		}
		if v == nil {
			return "", fmt.Errorf("version not found: %s", *versionID)
		}
		return s.ReadContent(ctx, v.Hash)
	}

	doc, err := s.db.GetFileByID(ctx, fileID)
	if err != nil {
		return "", fmt.Errorf("get file: %w", err)
	}
	if doc == nil {
		return "", fmt.Errorf("file not found: %s", fileID)
	}
	if doc.Hash == nil {
		return "", nil
	}
	return s.ReadContent(ctx, *doc.Hash)
}
