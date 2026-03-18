package ftp

import (
	"context"
	"log/slog"
	"path"
	"strings"
	"time"

	"github.com/raphi011/know/internal/auth"
	"github.com/raphi011/know/internal/db"
	"github.com/raphi011/know/internal/models"
)

// maxListEntries is the maximum number of entries in a directory listing.
const maxListEntries = 10000

// dirEntry represents a single entry in a directory listing.
type dirEntry struct {
	name    string
	size    int64
	modTime time.Time
	isDir   bool
}

// listDir returns directory entries for the given path.
func (s *session) listDir(target string) ([]dirEntry, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	vaultName, docPath := parsePath(target)

	// Root: list accessible vaults
	if vaultName == "" {
		return s.listVaults(ctx)
	}

	vaultID, err := s.resolveVault(ctx, vaultName)
	if err != nil {
		return nil, err
	}

	return s.listDirEntries(ctx, vaultID, docPath)
}

// listVaults returns all accessible vaults as directory entries.
func (s *session) listVaults(ctx context.Context) ([]dirEntry, error) {
	vaults, err := s.vaultSvc.List(ctx)
	if err != nil {
		return nil, err
	}

	var entries []dirEntry
	for _, v := range vaults {
		id, err := models.RecordIDString(v.ID)
		if err != nil {
			slog.Warn("ftp: skipping vault with invalid ID", "vault", v.Name, "error", err)
			continue
		}
		if err := auth.CheckVaultRole(*s.ac, id, models.RoleRead); err != nil {
			continue
		}
		entries = append(entries, dirEntry{
			name:    v.Name,
			modTime: v.CreatedAt,
			isDir:   true,
		})
	}

	return entries, nil
}

// listDirEntries returns the immediate children (folders + documents) of a directory.
func (s *session) listDirEntries(ctx context.Context, vaultID, dirPath string) ([]dirEntry, error) {
	var entries []dirEntry

	// List immediate child folders
	childFolders, err := s.vaultSvc.ListFolders(ctx, vaultID, &dirPath)
	if err != nil {
		return nil, err
	}
	for _, folder := range childFolders {
		entries = append(entries, dirEntry{
			name:    path.Base(folder.Path),
			isDir:   true,
			modTime: folder.CreatedAt,
		})
	}

	// List immediate child documents
	folderFilter := dirPath
	if folderFilter != "/" {
		folderFilter += "/"
	}
	isNotFolder := false
	metas, err := s.dbClient.ListFileMetas(ctx, db.ListFilesFilter{
		VaultID:  vaultID,
		Folder:   &folderFilter,
		IsFolder: &isNotFolder,
		Limit:    maxListEntries,
	})
	if err != nil {
		return nil, err
	}
	for _, meta := range metas {
		rel := strings.TrimPrefix(meta.Path, folderFilter)
		if dirPath == "/" {
			rel = strings.TrimPrefix(meta.Path, "/")
		}
		if strings.Contains(rel, "/") {
			continue // nested doc, skip
		}
		entries = append(entries, dirEntry{
			name:    path.Base(meta.Path),
			size:    int64(meta.Size),
			modTime: meta.UpdatedAt,
		})
	}

	return entries, nil
}
