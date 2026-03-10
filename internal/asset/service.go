package asset

import (
	"context"
	"fmt"
	"net/http"

	"github.com/raphi011/knowhow/internal/db"
	"github.com/raphi011/knowhow/internal/event"
	"github.com/raphi011/knowhow/internal/models"
)

// Service manages asset lifecycle: validate, store, serve.
type Service struct {
	db  *db.Client
	bus *event.Bus
}

// NewService creates a new asset service.
func NewService(db *db.Client, bus *event.Bus) *Service {
	return &Service{db: db, bus: bus}
}

// Create validates and upserts an asset.
func (s *Service) Create(ctx context.Context, input models.AssetInput) (*models.Asset, error) {
	if len(input.Data) == 0 {
		return nil, fmt.Errorf("empty asset data")
	}

	if input.MimeType == "" {
		input.MimeType = models.MimeTypeFromExt(input.Path)
	}
	if input.MimeType == "" {
		input.MimeType = http.DetectContentType(input.Data)
	}

	input.Path = models.NormalizePath(input.Path)

	asset, err := s.db.UpsertAsset(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("create: %w", err)
	}

	if s.bus != nil {
		vaultID := models.BareID("vault", input.VaultID)
		s.bus.Publish(event.ChangeEvent{
			Type:    "asset.created",
			VaultID: vaultID,
			Payload: event.DocumentPayload{
				Path:        input.Path,
				ContentHash: asset.ContentHash,
			},
		})
	}

	return asset, nil
}

// Get returns the full asset (including data) by vault+path.
func (s *Service) Get(ctx context.Context, vaultID, path string) (*models.Asset, error) {
	path = models.NormalizePath(path)
	asset, err := s.db.GetAssetByPath(ctx, vaultID, path)
	if err != nil {
		return nil, fmt.Errorf("get: %w", err)
	}
	return asset, nil
}

// GetMeta returns lightweight asset metadata (no data).
func (s *Service) GetMeta(ctx context.Context, vaultID, path string) (*models.AssetMeta, error) {
	path = models.NormalizePath(path)
	meta, err := s.db.GetAssetMetaByPath(ctx, vaultID, path)
	if err != nil {
		return nil, fmt.Errorf("get meta: %w", err)
	}
	return meta, nil
}

// Delete removes an asset by vault+path.
func (s *Service) Delete(ctx context.Context, vaultID, path string) error {
	path = models.NormalizePath(path)
	if err := s.db.DeleteAsset(ctx, vaultID, path); err != nil {
		return fmt.Errorf("delete: %w", err)
	}

	if s.bus != nil {
		s.bus.Publish(event.ChangeEvent{
			Type:    "asset.deleted",
			VaultID: models.BareID("vault", vaultID),
			Payload: event.DocumentPayload{
				Path: path,
			},
		})
	}

	return nil
}

// Move renames an asset from oldPath to newPath.
func (s *Service) Move(ctx context.Context, vaultID, oldPath, newPath string) error {
	oldPath = models.NormalizePath(oldPath)
	newPath = models.NormalizePath(newPath)
	if err := s.db.MoveAsset(ctx, vaultID, oldPath, newPath); err != nil {
		return fmt.Errorf("move: %w", err)
	}

	if s.bus != nil {
		s.bus.Publish(event.ChangeEvent{
			Type:    "asset.moved",
			VaultID: models.BareID("vault", vaultID),
			Payload: event.DocumentPayload{
				Path:    newPath,
				OldPath: oldPath,
			},
		})
	}

	return nil
}

// ListMetas returns lightweight metadata for assets in a vault, optionally filtered by folder.
func (s *Service) ListMetas(ctx context.Context, vaultID string, folder *string) ([]models.AssetMeta, error) {
	metas, err := s.db.ListAssetMetas(ctx, vaultID, folder)
	if err != nil {
		return nil, fmt.Errorf("list metas: %w", err)
	}
	return metas, nil
}
