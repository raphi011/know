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

	"github.com/raphi011/knowhow/internal/asset"
	"github.com/raphi011/knowhow/internal/db"
	"github.com/raphi011/knowhow/internal/document"
	"github.com/raphi011/knowhow/internal/models"
	"github.com/raphi011/knowhow/internal/vault"
)

const markdownContentType = "text/markdown; charset=utf-8"

// FS implements webdav.FileSystem backed by document, asset, and vault services.
type FS struct {
	vaultID      string
	db           *db.Client
	docService   *document.Service
	assetSvc     *asset.Service
	vaultSvc     *vault.Service
	pending      *pendingSet
	virtualFiles []VirtualFile
}

// NewFS creates a WebDAV filesystem for the given vault.
func NewFS(vaultID string, db *db.Client, docService *document.Service, assetSvc *asset.Service, vaultSvc *vault.Service, pending *pendingSet) *FS {
	return &FS{
		vaultID:      vaultID,
		db:           db,
		docService:   docService,
		assetSvc:     assetSvc,
		vaultSvc:     vaultSvc,
		pending:      pending,
		virtualFiles: defaultVirtualFiles(db),
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

	// Silently accept macOS metadata files (._*, .DS_Store) — discard on write
	if isOSMetadataFile(name) {
		if flag&(os.O_WRONLY|os.O_RDWR|os.O_CREATE) != 0 {
			return newNopFile(name), nil
		}
		return nil, os.ErrNotExist
	}

	// Virtual files are read-only computed views
	if vf := f.findVirtualFile(name); f.isVirtualFilePath(name) && vf != nil {
		if flag&(os.O_WRONLY|os.O_RDWR|os.O_CREATE) != 0 {
			return nil, os.ErrPermission
		}
		content, err := vf.Generate(ctx, f.vaultID)
		if err != nil {
			return nil, fmt.Errorf("open %s: %w", name, err)
		}
		return newVirtualReadFile(name, content), nil
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
			// Open for writing — PUT is a full replacement, don't pre-load existing content
			return newWriteFile(name, f.vaultID, f.docService, nil, doc.UpdatedAt, false, f.pending), nil
		}
		return newReadFile(name, doc), nil
	}

	// Try as asset
	if flag&(os.O_WRONLY|os.O_RDWR) != 0 {
		// For write mode, only need metadata (avoid loading full binary data)
		assetMeta, err := f.assetSvc.GetMeta(ctx, f.vaultID, name)
		if err != nil {
			return nil, fmt.Errorf("open %s: %w", name, err)
		}
		if assetMeta != nil {
			return newAssetWriteFile(name, f.vaultID, f.assetSvc, assetMeta.UpdatedAt, false, f.pending), nil
		}
	} else {
		assetObj, err := f.assetSvc.Get(ctx, f.vaultID, name)
		if err != nil {
			return nil, fmt.Errorf("open %s: %w", name, err)
		}
		if assetObj != nil {
			return newAssetReadFile(name, assetObj), nil
		}
	}

	// Try as folder
	folder, err := f.db.GetFolderByPath(ctx, f.vaultID, name)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", name, err)
	}
	if folder != nil {
		return f.openDir(ctx, name, folder.CreatedAt)
	}

	// Check pending set before creating — file may have been claimed already
	if f.pending.Has(name) {
		if flag&(os.O_WRONLY|os.O_RDWR|os.O_CREATE) != 0 {
			// Subsequent write to a pending file (the real content PUT)
			if models.IsImageFile(name) {
				return newAssetWriteFile(name, f.vaultID, f.assetSvc, time.Now(), false, f.pending), nil
			}
			return newWriteFile(name, f.vaultID, f.docService, nil, time.Now(), false, f.pending), nil
		}
		// Read-only open on a pending path — return empty read file
		return newEmptyReadFile(name), nil
	}

	// Not found — if creating, open for write
	if flag&os.O_CREATE != 0 {
		if models.IsImageFile(name) {
			return newAssetWriteFile(name, f.vaultID, f.assetSvc, time.Now(), true, f.pending), nil
		}
		if !isMarkdownFile(name) {
			return nil, errNotMarkdown
		}
		return newWriteFile(name, f.vaultID, f.docService, nil, time.Now(), true, f.pending), nil
	}

	return nil, os.ErrNotExist
}

// RemoveAll removes a file (document) or directory (folder) and all children.
func (f *FS) RemoveAll(ctx context.Context, name string) error {
	name = normalizeName(name)
	if name == "/" {
		return fmt.Errorf("cannot remove root directory")
	}

	// macOS metadata files are never stored — nothing to remove
	if isOSMetadataFile(name) {
		return nil
	}

	// Clean up pending entry if present
	wasPending := f.pending.Remove(name)

	// Virtual files are read-only
	if f.isVirtualFilePath(name) {
		return os.ErrPermission
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

	// Try as asset
	assetMeta, err := f.assetSvc.GetMeta(ctx, f.vaultID, name)
	if err != nil {
		return fmt.Errorf("remove %s: %w", name, err)
	}
	if assetMeta != nil {
		if err := f.assetSvc.Delete(ctx, f.vaultID, name); err != nil {
			return fmt.Errorf("remove asset %s: %w", name, err)
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

	// File was only in the pending set (not yet persisted to DB)
	if wasPending {
		return nil
	}

	return os.ErrNotExist
}

// Rename moves a file or directory.
func (f *FS) Rename(ctx context.Context, oldName, newName string) error {
	oldName = normalizeName(oldName)
	newName = normalizeName(newName)

	// macOS metadata files are never stored — nothing to rename
	if isOSMetadataFile(oldName) {
		return nil
	}

	// Virtual files are read-only
	if f.isVirtualFilePath(oldName) || f.isVirtualFilePath(newName) {
		return os.ErrPermission
	}

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

	// Try as asset
	assetMeta, err := f.assetSvc.GetMeta(ctx, f.vaultID, oldName)
	if err != nil {
		return fmt.Errorf("rename %s: %w", oldName, err)
	}
	if assetMeta != nil {
		if !models.IsImageFile(newName) {
			return fmt.Errorf("cannot rename asset to non-image file: %w", os.ErrPermission)
		}
		if err := f.assetSvc.Move(ctx, f.vaultID, oldName, newName); err != nil {
			return fmt.Errorf("rename asset %s to %s: %w", oldName, newName, err)
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

	// Pending file exists in memory but not in DB — cannot rename
	if f.pending.Has(oldName) {
		return fmt.Errorf("rename %s: %w", oldName, os.ErrPermission)
	}

	return os.ErrNotExist
}

// Stat returns file info for a file or directory.
func (f *FS) Stat(ctx context.Context, name string) (os.FileInfo, error) {
	name = normalizeName(name)

	if name == "/" {
		return &fileInfo{name: "/", isDir: true, modTime: time.Now()}, nil
	}

	// macOS metadata files are never stored — always report not found
	if isOSMetadataFile(name) {
		return nil, os.ErrNotExist
	}

	// Virtual files report size 0 in stat to avoid expensive content generation.
	// The actual Content-Length is set when the file is opened for reading.
	if f.isVirtualFilePath(name) {
		return &fileInfo{
			name:        path.Base(name),
			modTime:     time.Now(),
			contentType: markdownContentType,
		}, nil
	}

	// Try as document (lightweight meta query — no content loaded)
	meta, err := f.db.GetDocumentMetaByPath(ctx, f.vaultID, name)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", name, err)
	}
	if meta != nil {
		fi := &fileInfo{
			name:        path.Base(name),
			size:        int64(meta.ContentLength),
			modTime:     meta.UpdatedAt,
			isDir:       false,
			contentType: markdownContentType,
		}
		if meta.ContentHash != nil {
			fi.etag = `"` + *meta.ContentHash + `"`
		}
		return fi, nil
	}

	// Try as asset
	assetMeta, err := f.assetSvc.GetMeta(ctx, f.vaultID, name)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", name, err)
	}
	if assetMeta != nil {
		return &fileInfo{
			name:        path.Base(name),
			size:        int64(assetMeta.Size),
			modTime:     assetMeta.UpdatedAt,
			isDir:       false,
			contentType: assetMeta.MimeType,
			etag:        `"` + assetMeta.ContentHash + `"`,
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

	// Check pending set — file was claimed but not yet persisted to DB
	if f.pending.Has(name) {
		fi := &fileInfo{
			name:    path.Base(name),
			modTime: time.Now(),
		}
		if isMarkdownFile(name) {
			fi.contentType = markdownContentType
		} else if models.IsImageFile(name) {
			fi.contentType = models.MimeTypeFromExt(name)
		}
		return fi, nil
	}

	return nil, os.ErrNotExist
}

// openRootDir returns a directory file listing the root contents.
func (f *FS) openRootDir(ctx context.Context) (webdav.File, error) {
	entries, err := f.listDirEntries(ctx, "/")
	if err != nil {
		return nil, fmt.Errorf("open root dir: %w", err)
	}
	return newDirFile("/", time.Now(), entries), nil
}

// openDir returns a directory file listing its contents.
func (f *FS) openDir(ctx context.Context, name string, modTime time.Time) (webdav.File, error) {
	entries, err := f.listDirEntries(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("open dir %s: %w", name, err)
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
		fi := &fileInfo{
			name:        path.Base(meta.Path),
			size:        int64(meta.ContentLength),
			modTime:     meta.UpdatedAt,
			isDir:       false,
			contentType: markdownContentType,
		}
		if meta.ContentHash != nil {
			fi.etag = `"` + *meta.ContentHash + `"`
		}
		entries = append(entries, fi)
	}

	// List immediate child assets
	assetMetas, err := f.assetSvc.ListMetas(ctx, f.vaultID, &folderFilter)
	if err != nil {
		return nil, fmt.Errorf("list assets in %s: %w", dirPath, err)
	}
	for _, am := range assetMetas {
		rel := strings.TrimPrefix(am.Path, folderFilter)
		if dirPath == "/" {
			rel = strings.TrimPrefix(am.Path, "/")
		}
		if strings.Contains(rel, "/") {
			continue // nested asset, skip
		}
		entries = append(entries, &fileInfo{
			name:        path.Base(am.Path),
			size:        int64(am.Size),
			modTime:     am.UpdatedAt,
			isDir:       false,
			contentType: am.MimeType,
			etag:        `"` + am.ContentHash + `"`,
		})
	}

	// Append virtual files to root directory listing
	if dirPath == "/" {
		for _, vf := range f.virtualFiles {
			entries = append(entries, &fileInfo{
				name:        vf.Name(),
				modTime:     time.Now(),
				contentType: markdownContentType,
			})
		}
	}

	return entries, nil
}

// isMarkdownFile returns true if the file has a .md extension (case-insensitive).
func isMarkdownFile(name string) bool {
	return strings.EqualFold(path.Ext(name), ".md")
}

// isOSMetadataFile returns true for macOS resource forks (._*) and .DS_Store.
func isOSMetadataFile(name string) bool {
	base := path.Base(name)
	return strings.HasPrefix(base, "._") || base == ".DS_Store"
}

// errNotMarkdown is returned when a non-markdown, non-image file is created or renamed to.
var errNotMarkdown = fmt.Errorf("only markdown (.md) and image files are allowed: %w", os.ErrPermission)

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
// It optionally implements webdav.ContentTyper (to avoid opening files to
// sniff MIME type) and webdav.ETager (to return content-hash-based ETags
// instead of the default ModTime+Size heuristic).
type fileInfo struct {
	name        string
	size        int64
	modTime     time.Time
	isDir       bool
	contentType string
	etag        string
}

func (fi *fileInfo) Name() string { return fi.name }
func (fi *fileInfo) Size() int64  { return fi.size }
func (fi *fileInfo) Mode() fs.FileMode {
	if fi.isDir {
		return fs.ModeDir | 0755
	}
	return 0644
}
func (fi *fileInfo) ModTime() time.Time { return fi.modTime }
func (fi *fileInfo) IsDir() bool        { return fi.isDir }
func (fi *fileInfo) Sys() any           { return nil }

// ContentType implements webdav.ContentTyper.
func (fi *fileInfo) ContentType(_ context.Context) (string, error) {
	if fi.contentType == "" {
		return "", webdav.ErrNotImplemented
	}
	return fi.contentType, nil
}

// ETag implements webdav.ETager.
func (fi *fileInfo) ETag(_ context.Context) (string, error) {
	if fi.etag == "" {
		return "", webdav.ErrNotImplemented
	}
	return fi.etag, nil
}
