package document

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/raphi011/know/internal/models"
)

// maybeCreateVersion snapshots the old document content as a version if the
// coalescing window has elapsed and the content actually changed.
func (s *Service) maybeCreateVersion(ctx context.Context, docID, vaultID string, oldDoc *models.Document, newContentHash string) {
	// Skip if content hasn't actually changed
	if oldDoc.ContentHash != nil && *oldDoc.ContentHash == newContentHash {
		return
	}

	// Coalescing: skip version if last one is too recent (0 = disabled)
	if s.versionConfig.CoalesceMinutes > 0 {
		latest, err := s.db.GetLatestVersion(ctx, docID)
		if err != nil {
			slog.Warn("failed to check latest version for coalescing", "doc_id", docID, "error", err)
			return
		}
		if latest != nil {
			threshold := time.Now().Add(-time.Duration(s.versionConfig.CoalesceMinutes) * time.Minute)
			if latest.CreatedAt.After(threshold) {
				return
			}
		}
	}

	// Get next version number
	nextVersion, err := s.db.NextVersionNumber(ctx, docID)
	if err != nil {
		slog.Warn("failed to get next version number", "doc_id", docID, "error", err)
		return
	}

	// Create version snapshot of the OLD content
	hash := models.ContentHash(oldDoc.Content)
	if _, err := s.db.CreateVersion(ctx, models.DocumentVersionInput{
		DocumentID:  docID,
		VaultID:     vaultID,
		Content:     oldDoc.Content,
		ContentHash: hash,
		Title:       oldDoc.Title,
		Source:      oldDoc.Source,
	}, nextVersion); err != nil {
		slog.Warn("failed to create version snapshot", "doc_id", docID, "version", nextVersion, "error", err)
		return
	}

	slog.Info("created version snapshot", "doc_id", docID, "version", nextVersion)

	// Enforce retention cap
	s.enforceRetention(ctx, docID)
}

// enforceRetention deletes versions beyond the retention cap.
func (s *Service) enforceRetention(ctx context.Context, docID string) {
	deleted, err := s.db.DeleteOldestVersions(ctx, docID, s.versionConfig.RetentionCount)
	if err != nil {
		slog.Warn("failed to enforce version retention", "doc_id", docID, "error", err)
		return
	}
	if deleted > 0 {
		slog.Info("pruned old versions", "doc_id", docID, "deleted", deleted)
	}
}

// Rollback restores a document to a previous version's content.
// Goes through the normal Create() pipeline, which triggers version creation
// of the pre-rollback state, syncChunks, wiki-link resolution, etc.
func (s *Service) Rollback(ctx context.Context, vaultID, documentID, versionID string) (*models.Document, error) {
	version, err := s.db.GetVersion(ctx, versionID)
	if err != nil {
		return nil, fmt.Errorf("get version: %w", err)
	}
	if version == nil {
		return nil, fmt.Errorf("version not found: %s", versionID)
	}

	versionDocID, err := models.RecordIDString(version.Document)
	if err != nil {
		return nil, fmt.Errorf("extract document ID from version: %w", err)
	}
	if versionDocID != documentID {
		return nil, fmt.Errorf("version %s does not belong to document %s", versionID, documentID)
	}

	doc, err := s.db.GetDocumentByID(ctx, documentID)
	if err != nil {
		return nil, fmt.Errorf("get document: %w", err)
	}
	if doc == nil {
		return nil, fmt.Errorf("document not found: %s", documentID)
	}

	// Go through normal save pipeline
	updated, err := s.Create(ctx, models.DocumentInput{
		VaultID: vaultID,
		Path:    doc.Path,
		Content: version.Content,
		Source:  models.SourceRollback,
	})
	if err != nil {
		return nil, fmt.Errorf("apply rollback: %w", err)
	}

	return updated, nil
}

// VersionContent resolves the content for a version ID, or the current document
// content if versionID is nil.
func (s *Service) VersionContent(ctx context.Context, documentID string, versionID *string) (string, error) {
	if versionID != nil {
		v, err := s.db.GetVersion(ctx, *versionID)
		if err != nil {
			return "", fmt.Errorf("get version: %w", err)
		}
		if v == nil {
			return "", fmt.Errorf("version not found: %s", *versionID)
		}
		return v.Content, nil
	}

	doc, err := s.db.GetDocumentByID(ctx, documentID)
	if err != nil {
		return "", fmt.Errorf("get document: %w", err)
	}
	if doc == nil {
		return "", fmt.Errorf("document not found: %s", documentID)
	}
	return doc.Content, nil
}
