package vault

import (
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/raphaelgruber/memcp-go/internal/db"
	"github.com/raphaelgruber/memcp-go/internal/models"
)

// Service manages vault CRUD and derived folder listing.
type Service struct {
	db *db.Client
}

// NewService creates a new vault service.
func NewService(db *db.Client) *Service {
	return &Service{db: db}
}

func (s *Service) Create(ctx context.Context, userID string, input models.VaultInput) (*models.Vault, error) {
	return s.db.CreateVault(ctx, userID, input)
}

func (s *Service) Get(ctx context.Context, id string) (*models.Vault, error) {
	return s.db.GetVault(ctx, id)
}

func (s *Service) GetByName(ctx context.Context, name string) (*models.Vault, error) {
	return s.db.GetVaultByName(ctx, name)
}

func (s *Service) List(ctx context.Context) ([]models.Vault, error) {
	return s.db.ListVaults(ctx)
}

func (s *Service) Delete(ctx context.Context, id string) error {
	return s.db.DeleteVault(ctx, id)
}

// ListFolders derives virtual folders from document paths in a vault.
func (s *Service) ListFolders(ctx context.Context, vaultID string, parentPath *string) ([]models.Folder, error) {
	paths, err := s.db.ListDocumentPaths(ctx, vaultID)
	if err != nil {
		return nil, fmt.Errorf("list document paths: %w", err)
	}

	return deriveFolders(paths, parentPath), nil
}

// deriveFolders computes virtual folders from document paths.
func deriveFolders(paths []string, parentPath *string) []models.Folder {
	prefix := "/"
	if parentPath != nil {
		prefix = *parentPath
		if !strings.HasSuffix(prefix, "/") {
			prefix += "/"
		}
	}

	// Count docs per immediate child folder
	folderCounts := make(map[string]int)
	for _, p := range paths {
		if !strings.HasPrefix(p, prefix) {
			continue
		}

		// Get the relative part after prefix
		rel := strings.TrimPrefix(p, prefix)
		if rel == "" {
			continue
		}

		// Extract the first path segment (immediate child folder)
		parts := strings.SplitN(rel, "/", 2)
		if len(parts) < 2 {
			// This is a file directly in the parent folder, not a subfolder
			continue
		}

		folderPath := prefix + parts[0]
		folderCounts[folderPath]++
	}

	folders := make([]models.Folder, 0, len(folderCounts))
	for fp, count := range folderCounts {
		folders = append(folders, models.Folder{
			Path:     fp,
			Name:     path.Base(fp),
			DocCount: count,
		})
	}
	return folders
}
