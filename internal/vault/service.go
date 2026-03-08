package vault

import (
	"context"
	"fmt"
	"os"

	"github.com/raphi011/knowhow/internal/auth"
	"github.com/raphi011/knowhow/internal/db"
	"github.com/raphi011/knowhow/internal/models"
)

// Service manages vault CRUD and folder operations.
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

// ResolveByName implements auth.VaultResolver.
func (s *Service) ResolveByName(ctx context.Context, name string) (*auth.VaultInfo, error) {
	v, err := s.GetByName(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("resolve vault %q: %w", name, err)
	}
	if v == nil {
		return nil, fmt.Errorf("vault not found: %w", os.ErrNotExist)
	}
	id, err := models.RecordIDString(v.ID)
	if err != nil {
		return nil, fmt.Errorf("extract vault ID: %w", err)
	}
	return &auth.VaultInfo{ID: id, Name: v.Name}, nil
}

func (s *Service) List(ctx context.Context) ([]models.Vault, error) {
	return s.db.ListVaults(ctx)
}

func (s *Service) Delete(ctx context.Context, id string) error {
	return s.db.DeleteVault(ctx, id)
}

// ListFolders returns all folders in a vault. If parentPath is provided, returns
// only immediate children of that path using a DB-level prefix filter (avoids
// loading all folders in the vault).
func (s *Service) ListFolders(ctx context.Context, vaultID string, parentPath *string) ([]models.Folder, error) {
	if parentPath != nil {
		folders, err := s.db.ListChildFolders(ctx, vaultID, *parentPath)
		if err != nil {
			return nil, fmt.Errorf("list child folders: %w", err)
		}
		return folders, nil
	}

	folders, err := s.db.ListFolders(ctx, vaultID)
	if err != nil {
		return nil, fmt.Errorf("list folders: %w", err)
	}
	return folders, nil
}

// CreateFolder creates a folder and all its ancestor folders.
func (s *Service) CreateFolder(ctx context.Context, vaultID, folderPath string) (*models.Folder, error) {
	folderPath = models.NormalizePath(folderPath)

	// EnsureFolderPath creates the target folder + all ancestors in one batch
	if err := s.db.EnsureFolderPath(ctx, vaultID, folderPath); err != nil {
		return nil, fmt.Errorf("ensure folder path: %w", err)
	}
	folder, err := s.db.GetFolderByPath(ctx, vaultID, folderPath)
	if err != nil {
		return nil, fmt.Errorf("get created folder: %w", err)
	}
	if folder == nil {
		return nil, fmt.Errorf("folder not found after creation: %s", folderPath)
	}
	return folder, nil
}

// DeleteFolder deletes a folder, all child folders, and all documents under the folder prefix.
func (s *Service) DeleteFolder(ctx context.Context, vaultID, folderPath string) error {
	folderPath = models.NormalizePath(folderPath)

	// Delete child documents
	prefix := folderPath + "/"
	if _, err := s.db.DeleteDocumentsByPrefix(ctx, vaultID, prefix); err != nil {
		return fmt.Errorf("delete folder documents: %w", err)
	}

	// Delete folder records (self + children)
	if err := s.db.DeleteFolder(ctx, vaultID, folderPath); err != nil {
		return fmt.Errorf("delete folder: %w", err)
	}

	return nil
}

// MoveFolder moves a folder and all its children (folders + documents) from oldPath to newPath.
func (s *Service) MoveFolder(ctx context.Context, vaultID, oldPath, newPath string) error {
	oldPath = models.NormalizePath(oldPath)
	newPath = models.NormalizePath(newPath)

	// Move folder records
	if _, err := s.db.MoveFoldersByPrefix(ctx, vaultID, oldPath, newPath); err != nil {
		return fmt.Errorf("move folder records: %w", err)
	}

	// Move documents
	if _, err := s.db.MoveDocumentsByPrefix(ctx, vaultID, oldPath+"/", newPath+"/"); err != nil {
		return fmt.Errorf("move folder documents: %w", err)
	}

	// Ensure destination ancestor folders exist
	if err := s.db.EnsureFolderPath(ctx, vaultID, newPath); err != nil {
		return fmt.Errorf("ensure destination folders: %w", err)
	}

	return nil
}
