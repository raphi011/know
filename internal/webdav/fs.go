// Package webdav provides a WebDAV filesystem backed by know's
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

	"github.com/raphi011/know/internal/db"
	"github.com/raphi011/know/internal/file"
	"github.com/raphi011/know/internal/models"
	"github.com/raphi011/know/internal/vault"
)

const markdownContentType = "text/markdown; charset=utf-8"

// FS implements webdav.FileSystem backed by the unified file service.
type FS struct {
	vaultID      string
	db           *db.Client
	fileSvc      *file.Service
	vaultSvc     *vault.Service
	pending      *pendingSet
	virtualFiles []VirtualFile
}

// NewFS creates a WebDAV filesystem for the given vault.
func NewFS(vaultID string, db *db.Client, fileSvc *file.Service, vaultSvc *vault.Service, pending *pendingSet) *FS {
	return &FS{
		vaultID:      vaultID,
		db:           db,
		fileSvc:      fileSvc,
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

	// Single lookup — documents, assets, and folders are all in the file table
	if flag&(os.O_WRONLY|os.O_RDWR) != 0 {
		// Write mode — only need metadata to check existence
		meta, err := f.fileSvc.GetMeta(ctx, f.vaultID, name)
		if err != nil {
			return nil, fmt.Errorf("open %s: %w", name, err)
		}
		if meta != nil {
			if meta.IsFolder {
				return nil, fmt.Errorf("open %s: %w", name, os.ErrPermission)
			}
			if isMarkdownFile(name) {
				return newWriteFile(name, f.vaultID, f.fileSvc, nil, meta.UpdatedAt, false, f.pending), nil
			}
			return newAssetWriteFile(name, f.vaultID, f.fileSvc, meta.UpdatedAt, false, f.pending), nil
		}
	} else {
		// Read mode — need full content
		doc, err := f.db.GetFileByPath(ctx, f.vaultID, name)
		if err != nil {
			return nil, fmt.Errorf("open %s: %w", name, err)
		}
		if doc != nil {
			if doc.IsFolder {
				return f.openDir(ctx, name, doc.UpdatedAt)
			}
			if isMarkdownFile(name) {
				return newReadFile(name, doc), nil
			}
			return newAssetReadFile(name, doc), nil
		}
	}

	// Check pending set before creating — file may have been claimed already
	if f.pending.Has(name) {
		if flag&(os.O_WRONLY|os.O_RDWR|os.O_CREATE) != 0 {
			// Subsequent write to a pending file (the real content PUT)
			if models.IsImageFile(name) {
				return newAssetWriteFile(name, f.vaultID, f.fileSvc, time.Now(), false, f.pending), nil
			}
			return newWriteFile(name, f.vaultID, f.fileSvc, nil, time.Now(), false, f.pending), nil
		}
		// Read-only open on a pending path — return empty read file
		return newEmptyReadFile(name), nil
	}

	// Not found — if creating, open for write
	if flag&os.O_CREATE != 0 {
		if models.IsImageFile(name) {
			return newAssetWriteFile(name, f.vaultID, f.fileSvc, time.Now(), true, f.pending), nil
		}
		if !isMarkdownFile(name) {
			return newNopFile(name), nil
		}
		return newWriteFile(name, f.vaultID, f.fileSvc, nil, time.Now(), true, f.pending), nil
	}

	return nil, os.ErrNotExist
}

// RemoveAll removes a file (document) or directory (folder) and all children.
func (f *FS) RemoveAll(ctx context.Context, name string) error {
	name = normalizeName(name)
	if name == "/" {
		return fmt.Errorf("cannot remove root directory")
	}

	// OS metadata and unsupported file types are never stored — nothing to remove
	if isNonStoredFile(name) {
		return nil
	}

	// Clean up pending entry if present
	wasPending := f.pending.Remove(name)

	// Virtual files are read-only
	if f.isVirtualFilePath(name) {
		return os.ErrPermission
	}

	// Single lookup — documents and assets are in the same file table
	meta, err := f.fileSvc.GetMeta(ctx, f.vaultID, name)
	if err != nil {
		return fmt.Errorf("remove %s: %w", name, err)
	}
	if meta != nil {
		if meta.IsFolder {
			if err := f.vaultSvc.DeleteFolder(ctx, f.vaultID, name); err != nil {
				return fmt.Errorf("remove folder %s: %w", name, err)
			}
			return nil
		}
		if err := f.fileSvc.Delete(ctx, f.vaultID, name); err != nil {
			return fmt.Errorf("remove %s: %w", name, err)
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

	// OS metadata and unsupported file types are never stored — nothing to rename.
	// Note: only oldName is checked. Renaming a supported file *to* an unsupported
	// extension is caught later by errNotMarkdown, protecting data integrity.
	if isNonStoredFile(oldName) {
		return nil
	}

	// Virtual files are read-only
	if f.isVirtualFilePath(oldName) || f.isVirtualFilePath(newName) {
		return os.ErrPermission
	}

	// Single lookup — documents, assets, and folders are in the same file table
	meta, err := f.fileSvc.GetMeta(ctx, f.vaultID, oldName)
	if err != nil {
		return fmt.Errorf("rename %s: %w", oldName, err)
	}
	if meta != nil {
		if meta.IsFolder {
			if err := f.vaultSvc.MoveFolder(ctx, f.vaultID, oldName, newName); err != nil {
				return fmt.Errorf("rename folder %s to %s: %w", oldName, newName, err)
			}
			return nil
		}
		if isMarkdownFile(oldName) && !isMarkdownFile(newName) {
			return errNotMarkdown
		}
		if models.IsImageFile(oldName) && !models.IsImageFile(newName) {
			return fmt.Errorf("cannot rename asset to non-image file: %w", os.ErrPermission)
		}
		if _, err := f.fileSvc.Move(ctx, f.vaultID, oldName, newName); err != nil {
			return fmt.Errorf("rename %s to %s: %w", oldName, newName, err)
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

	// OS metadata and unsupported file types are never stored — always report not found
	if isNonStoredFile(name) {
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

	// Single lookup — documents, assets, and folders are in the same file table
	meta, err := f.fileSvc.GetMeta(ctx, f.vaultID, name)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", name, err)
	}
	if meta != nil {
		if meta.IsFolder {
			return &fileInfo{
				name:    path.Base(name),
				isDir:   true,
				modTime: meta.UpdatedAt,
			}, nil
		}
		ct := meta.MimeType
		if isMarkdownFile(name) {
			ct = markdownContentType
		}
		return &fileInfo{
			name:        path.Base(name),
			size:        int64(meta.Size),
			modTime:     meta.UpdatedAt,
			isDir:       false,
			contentType: ct,
			etag:        contentHashETag(meta.ContentHash),
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

	// Append "/" for non-root paths so the DB filter matches files under
	// this folder (root already ends with "/").
	folderFilter := dirPath
	if folderFilter != "/" {
		folderFilter += "/"
	}
	// List immediate child files (lightweight meta query — no content loaded)
	isNotFolder := false
	metas, err := f.db.ListFileMetas(ctx, db.ListFilesFilter{
		VaultID:  f.vaultID,
		Folder:   &folderFilter,
		IsFolder: &isNotFolder,
		Limit:    10000,
	})
	if err != nil {
		return nil, fmt.Errorf("list files in %s: %w", dirPath, err)
	}
	for _, meta := range metas {
		// Only include immediate children, not nested files
		rel := strings.TrimPrefix(meta.Path, folderFilter)
		if dirPath == "/" {
			rel = strings.TrimPrefix(meta.Path, "/")
		}
		if strings.Contains(rel, "/") {
			continue // nested file, skip
		}
		ct := meta.MimeType
		if isMarkdownFile(meta.Path) {
			ct = markdownContentType
		}
		entries = append(entries, &fileInfo{
			name:        path.Base(meta.Path),
			size:        int64(meta.Size),
			modTime:     meta.UpdatedAt,
			isDir:       false,
			contentType: ct,
			etag:        contentHashETag(meta.ContentHash),
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

// isUnsupportedFile returns true for files that are neither markdown, image,
// nor OS metadata. Extensionless paths and dotfiles (e.g. ".claude", ".foam")
// are excluded — path.Ext treats dotfiles as having an extension equal to
// the entire name, but they are really extensionless hidden files/folders.
func isUnsupportedFile(name string) bool {
	ext := path.Ext(name)
	if ext == "" {
		return false
	}
	base := path.Base(name)
	if base == ext {
		// Dotfile: ".claude", ".foam" — path.Ext sees the whole name as extension.
		return false
	}
	return !isMarkdownFile(name) && !models.IsImageFile(name) && !isOSMetadataFile(name)
}

// isNonStoredFile returns true for files that are never persisted to the DB:
// OS metadata files (._*, .DS_Store) and unsupported file types (.pdf, .txt, etc.).
func isNonStoredFile(name string) bool {
	return isOSMetadataFile(name) || isUnsupportedFile(name)
}

// contentHashETag returns a quoted ETag from a content hash pointer, or empty string if nil.
func contentHashETag(hash *string) string {
	if hash == nil {
		return ""
	}
	return `"` + *hash + `"`
}

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
