// Package webdav provides a WebDAV filesystem backed by knowhow's
// document and vault services. Files map to documents, directories
// map to vault folders. Writing a file triggers the full document
// pipeline (parse → embed → link → chunk).
package webdav

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path"
	"strings"
	"time"

	"golang.org/x/net/webdav"

	"github.com/raphi011/knowhow/internal/db"
	"github.com/raphi011/knowhow/internal/document"
	"github.com/raphi011/knowhow/internal/vault"
)

// FS implements webdav.FileSystem backed by document and vault services.
type FS struct {
	vaultID    string
	db         *db.Client
	docService *document.Service
	vaultSvc   *vault.Service
}

// NewFS creates a WebDAV filesystem for the given vault.
func NewFS(vaultID string, db *db.Client, docService *document.Service, vaultSvc *vault.Service) *FS {
	return &FS{
		vaultID:    vaultID,
		db:         db,
		docService: docService,
		vaultSvc:   vaultSvc,
	}
}

// Mkdir creates a directory (folder) in the vault.
func (f *FS) Mkdir(ctx context.Context, name string, perm os.FileMode) error {
	name = normalizeName(name)
	if name == "/" {
		return nil // root always exists
	}
	_, err := f.vaultSvc.CreateFolder(ctx, f.vaultID, name)
	if err != nil {
		return fmt.Errorf("mkdir %s: %w", name, err)
	}
	return nil
}

// OpenFile opens a file (document) or directory (folder) for reading or writing.
func (f *FS) OpenFile(ctx context.Context, name string, flag int, perm os.FileMode) (webdav.File, error) {
	name = normalizeName(name)

	// Check if it's a directory first
	if name == "/" {
		return f.openRootDir(ctx)
	}

	// Try as document
	doc, err := f.db.GetDocumentByPath(ctx, f.vaultID, name)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", name, err)
	}

	if doc != nil {
		// Existing document
		if flag&(os.O_WRONLY|os.O_RDWR) != 0 {
			if !isMarkdownFile(name) {
				return nil, errNotMarkdown
			}
			// Open for writing — start with existing content
			return newWriteFile(name, f.vaultID, f.docService, []byte(doc.Content), doc.UpdatedAt), nil
		}
		return newReadFile(name, doc), nil
	}

	// Try as folder
	folder, err := f.db.GetFolderByPath(ctx, f.vaultID, name)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", name, err)
	}
	if folder != nil {
		return f.openDir(ctx, name, folder.CreatedAt)
	}

	// Not found — if creating, open for write
	if flag&os.O_CREATE != 0 {
		if !isMarkdownFile(name) {
			return nil, errNotMarkdown
		}
		return newWriteFile(name, f.vaultID, f.docService, nil, time.Now()), nil
	}

	return nil, os.ErrNotExist
}

// RemoveAll removes a file (document) or directory (folder) and all children.
func (f *FS) RemoveAll(ctx context.Context, name string) error {
	name = normalizeName(name)
	if name == "/" {
		return fmt.Errorf("cannot remove root directory")
	}

	// Try as document first
	doc, err := f.db.GetDocumentByPath(ctx, f.vaultID, name)
	if err != nil {
		return fmt.Errorf("remove %s: %w", name, err)
	}
	if doc != nil {
		if err := f.docService.Delete(ctx, f.vaultID, name); err != nil {
			return fmt.Errorf("remove document %s: %w", name, err)
		}
		return nil
	}

	// Try as folder
	folder, err := f.db.GetFolderByPath(ctx, f.vaultID, name)
	if err != nil {
		return fmt.Errorf("remove %s: %w", name, err)
	}
	if folder != nil {
		if err := f.vaultSvc.DeleteFolder(ctx, f.vaultID, name); err != nil {
			return fmt.Errorf("remove folder %s: %w", name, err)
		}
		return nil
	}

	return os.ErrNotExist
}

// Rename moves a file or directory.
func (f *FS) Rename(ctx context.Context, oldName, newName string) error {
	oldName = normalizeName(oldName)
	newName = normalizeName(newName)

	// Try as document first
	doc, err := f.db.GetDocumentByPath(ctx, f.vaultID, oldName)
	if err != nil {
		return fmt.Errorf("rename %s: %w", oldName, err)
	}
	if doc != nil {
		if !isMarkdownFile(newName) {
			return errNotMarkdown
		}
		if _, err := f.docService.Move(ctx, f.vaultID, oldName, newName); err != nil {
			return fmt.Errorf("rename %s to %s: %w", oldName, newName, err)
		}
		return nil
	}

	// Try as folder
	folder, err := f.db.GetFolderByPath(ctx, f.vaultID, oldName)
	if err != nil {
		return fmt.Errorf("rename %s: %w", oldName, err)
	}
	if folder != nil {
		if err := f.vaultSvc.MoveFolder(ctx, f.vaultID, oldName, newName); err != nil {
			return fmt.Errorf("rename folder %s to %s: %w", oldName, newName, err)
		}
		return nil
	}

	return os.ErrNotExist
}

// Stat returns file info for a file or directory.
func (f *FS) Stat(ctx context.Context, name string) (os.FileInfo, error) {
	name = normalizeName(name)

	if name == "/" {
		return &fileInfo{name: "/", isDir: true, modTime: time.Now()}, nil
	}

	// Try as document (lightweight meta query — no content loaded)
	meta, err := f.db.GetDocumentMetaByPath(ctx, f.vaultID, name)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", name, err)
	}
	if meta != nil {
		return &fileInfo{
			name:    path.Base(name),
			size:    int64(meta.ContentLength),
			modTime: meta.UpdatedAt,
			isDir:   false,
		}, nil
	}

	// Try as folder
	folder, err := f.db.GetFolderByPath(ctx, f.vaultID, name)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", name, err)
	}
	if folder != nil {
		return &fileInfo{
			name:    path.Base(name),
			isDir:   true,
			modTime: folder.CreatedAt,
		}, nil
	}

	return nil, os.ErrNotExist
}

// openRootDir returns a directory file listing the root contents.
func (f *FS) openRootDir(ctx context.Context) (webdav.File, error) {
	entries, err := f.listDirEntries(ctx, "/")
	if err != nil {
		return nil, err
	}
	return newDirFile("/", time.Now(), entries), nil
}

// openDir returns a directory file listing its contents.
func (f *FS) openDir(ctx context.Context, name string, modTime time.Time) (webdav.File, error) {
	entries, err := f.listDirEntries(ctx, name)
	if err != nil {
		return nil, err
	}
	return newDirFile(name, modTime, entries), nil
}

// listDirEntries returns the immediate children (folders + documents) of a directory.
func (f *FS) listDirEntries(ctx context.Context, dirPath string) ([]os.FileInfo, error) {
	var entries []os.FileInfo

	// List immediate child folders
	childFolders, err := f.vaultSvc.ListFolders(ctx, f.vaultID, &dirPath)
	if err != nil {
		return nil, fmt.Errorf("list folders in %s: %w", dirPath, err)
	}
	for _, folder := range childFolders {
		entries = append(entries, &fileInfo{
			name:    path.Base(folder.Path),
			isDir:   true,
			modTime: folder.CreatedAt,
		})
	}

	// Append "/" for non-root paths so the DB filter matches documents under
	// this folder (root already ends with "/").
	folderFilter := dirPath
	if folderFilter != "/" {
		folderFilter += "/"
	}
	// List immediate child documents (lightweight meta query — no content loaded)
	metas, err := f.db.ListDocumentMetas(ctx, db.ListDocumentsFilter{
		VaultID: f.vaultID,
		Folder:  &folderFilter,
		Limit:   10000,
	})
	if err != nil {
		return nil, fmt.Errorf("list documents in %s: %w", dirPath, err)
	}
	for _, meta := range metas {
		// Only include immediate children, not nested docs
		rel := strings.TrimPrefix(meta.Path, folderFilter)
		if dirPath == "/" {
			rel = strings.TrimPrefix(meta.Path, "/")
		}
		if strings.Contains(rel, "/") {
			continue // nested doc, skip
		}
		entries = append(entries, &fileInfo{
			name:    path.Base(meta.Path),
			size:    int64(meta.ContentLength),
			modTime: meta.UpdatedAt,
			isDir:   false,
		})
	}

	return entries, nil
}

// isMarkdownFile returns true if the file has a .md extension (case-insensitive).
func isMarkdownFile(name string) bool {
	return strings.EqualFold(path.Ext(name), ".md")
}

// errNotMarkdown is returned when a non-markdown file is created or renamed to.
var errNotMarkdown = fmt.Errorf("only markdown files (.md) are allowed: %w", os.ErrPermission)

// normalizeName cleans up a WebDAV path to match our internal path format.
func normalizeName(name string) string {
	name = path.Clean(name)
	if name == "." || name == "" {
		return "/"
	}
	if !strings.HasPrefix(name, "/") {
		name = "/" + name
	}
	return name
}

// fileInfo implements os.FileInfo for WebDAV entries.
type fileInfo struct {
	name    string
	size    int64
	modTime time.Time
	isDir   bool
}

func (fi *fileInfo) Name() string        { return fi.name }
func (fi *fileInfo) Size() int64          { return fi.size }
func (fi *fileInfo) Mode() fs.FileMode    { if fi.isDir { return fs.ModeDir | 0755 }; return 0644 }
func (fi *fileInfo) ModTime() time.Time   { return fi.modTime }
func (fi *fileInfo) IsDir() bool          { return fi.isDir }
func (fi *fileInfo) Sys() any             { return nil }
