// Package nfs provides an NFSv3 server for accessing know documents.
// Authentication is localhost-only with no per-user auth (NFSv3 AUTH_UNIX
// doesn't map to Know tokens). All accessible vaults appear as top-level
// directories.
package nfs

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path"
	"strings"
	"time"

	billy "github.com/go-git/go-billy/v5"

	"github.com/raphi011/know/internal/auth"
	"github.com/raphi011/know/internal/db"
	"github.com/raphi011/know/internal/document"
	"github.com/raphi011/know/internal/models"
	"github.com/raphi011/know/internal/vault"
)

// epoch is used as the modTime for virtual directories (root, vault roots)
// to avoid returning time.Now() which defeats NFS client caching.
var epoch = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

// maxDocSize is the maximum allowed document size via NFS (10 MB).
const maxDocSize = 10 << 20

// maxListEntries is the maximum number of documents returned in a directory listing.
const maxListEntries = 10000

// FS implements billy.Filesystem backed by Know's document and vault services.
// The root directory lists all accessible vaults; subdirectories are vault contents.
type FS struct {
	dbClient   *db.Client
	docService *document.Service
	vaultSvc   *vault.Service
	ac         auth.AuthContext
	logger     *slog.Logger
}

// NewFS creates a new billy.Filesystem backed by Know services.
func NewFS(dbClient *db.Client, docService *document.Service, vaultSvc *vault.Service, ac auth.AuthContext, logger *slog.Logger) billy.Filesystem {
	return &FS{
		dbClient:   dbClient,
		docService: docService,
		vaultSvc:   vaultSvc,
		ac:         ac,
		logger:     logger,
	}
}

// parsePath splits a path into vault name and document path within the vault.
// "/" → ("", "")
// "/myvault" → ("myvault", "/")
// "/myvault/notes/foo.md" → ("myvault", "/notes/foo.md")
func parsePath(p string) (vaultName, docPath string) {
	p = path.Clean(p)
	if p == "." || p == "/" || p == "" {
		return "", ""
	}

	p = strings.TrimPrefix(p, "/")
	parts := strings.SplitN(p, "/", 2)
	vaultName = parts[0]

	if len(parts) == 1 {
		return vaultName, "/"
	}

	docPath = "/" + parts[1]
	return vaultName, docPath
}

// resolveVault looks up a vault by name and checks access.
func (f *FS) resolveVault(ctx context.Context, vaultName string) (string, error) {
	id, err := auth.ResolveVault(ctx, f.ac, f.vaultSvc, vaultName)
	if err != nil {
		f.logger.Warn("vault resolution failed", "vault", vaultName, "error", err)
		if os.IsNotExist(err) {
			return "", os.ErrNotExist
		}
		if os.IsPermission(err) {
			return "", os.ErrPermission
		}
		return "", fmt.Errorf("resolve vault %s: %w", vaultName, err)
	}
	return id, nil
}

// Create creates the named file with mode 0666, truncating it if it already exists.
func (f *FS) Create(filename string) (billy.File, error) {
	return f.OpenFile(filename, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0666)
}

// Open opens the named file for reading.
func (f *FS) Open(filename string) (billy.File, error) {
	return f.OpenFile(filename, os.O_RDONLY, 0)
}

// OpenFile opens the named file with specified flags.
func (f *FS) OpenFile(filename string, flag int, _ os.FileMode) (billy.File, error) {
	vaultName, docPath := parsePath(filename)

	// Root directory
	if vaultName == "" {
		return &nopFile{name: "/"}, nil
	}

	ctx := context.Background()

	vaultID, err := f.resolveVault(ctx, vaultName)
	if err != nil {
		return nil, err
	}

	// Vault root
	if docPath == "/" {
		return &nopFile{name: vaultName}, nil
	}

	isWrite := flag&(os.O_WRONLY|os.O_RDWR|os.O_CREATE|os.O_TRUNC) != 0

	if isWrite {
		if !isMarkdownFile(docPath) {
			return nil, errNotMarkdown
		}
		return &writeFile{
			name:       path.Base(docPath),
			path:       docPath,
			vaultID:    vaultID,
			docService: f.docService,
			logger:     f.logger,
		}, nil
	}

	// Read path: try document first
	doc, err := f.dbClient.GetDocumentByPath(ctx, vaultID, docPath)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", filename, err)
	}
	if doc != nil {
		return newReadFile(path.Base(docPath), []byte(doc.Content), doc.UpdatedAt), nil
	}

	// Try as folder
	folder, err := f.dbClient.GetFolderByPath(ctx, vaultID, docPath)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", filename, err)
	}
	if folder != nil {
		return &nopFile{name: path.Base(docPath)}, nil
	}

	return nil, os.ErrNotExist
}

// Stat returns a FileInfo describing the named file.
func (f *FS) Stat(filename string) (os.FileInfo, error) {
	vaultName, docPath := parsePath(filename)

	// Root directory
	if vaultName == "" {
		return &fileInfo{name: "/", isDir: true, modTime: epoch}, nil
	}

	ctx := context.Background()

	vaultID, err := f.resolveVault(ctx, vaultName)
	if err != nil {
		return nil, err
	}

	// Vault root
	if docPath == "/" {
		return &fileInfo{name: vaultName, isDir: true, modTime: epoch}, nil
	}

	// Try as document
	meta, err := f.dbClient.GetDocumentMetaByPath(ctx, vaultID, docPath)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", filename, err)
	}
	if meta != nil {
		return &fileInfo{
			name:    path.Base(docPath),
			size:    int64(meta.ContentLength),
			modTime: meta.UpdatedAt,
		}, nil
	}

	// Try as folder
	folder, err := f.dbClient.GetFolderByPath(ctx, vaultID, docPath)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", filename, err)
	}
	if folder != nil {
		return &fileInfo{
			name:    path.Base(docPath),
			isDir:   true,
			modTime: folder.CreatedAt,
		}, nil
	}

	return nil, os.ErrNotExist
}

// Rename renames (moves) oldpath to newpath.
func (f *FS) Rename(oldpath, newpath string) error {
	oldVault, oldDoc := parsePath(oldpath)
	newVault, newDoc := parsePath(newpath)

	if oldVault == "" || newVault == "" {
		return os.ErrPermission
	}
	if oldVault != newVault {
		return fmt.Errorf("cross-vault rename not supported: %w", os.ErrPermission)
	}

	ctx := context.Background()

	vaultID, err := f.resolveVault(ctx, oldVault)
	if err != nil {
		return err
	}

	// Try as document
	doc, err := f.dbClient.GetDocumentByPath(ctx, vaultID, oldDoc)
	if err != nil {
		return fmt.Errorf("rename %s: %w", oldpath, err)
	}
	if doc != nil {
		if !isMarkdownFile(newDoc) {
			return errNotMarkdown
		}
		_, err = f.docService.Move(ctx, vaultID, oldDoc, newDoc)
		if err != nil {
			return fmt.Errorf("rename %s to %s: %w", oldpath, newpath, err)
		}
		return nil
	}

	// Try as folder
	folder, err := f.dbClient.GetFolderByPath(ctx, vaultID, oldDoc)
	if err != nil {
		return fmt.Errorf("rename %s: %w", oldpath, err)
	}
	if folder != nil {
		if err := f.vaultSvc.MoveFolder(ctx, vaultID, oldDoc, newDoc); err != nil {
			return fmt.Errorf("rename folder %s to %s: %w", oldpath, newpath, err)
		}
		return nil
	}

	return os.ErrNotExist
}

// Remove removes the named file or directory.
func (f *FS) Remove(filename string) error {
	vaultName, docPath := parsePath(filename)
	if vaultName == "" {
		return os.ErrPermission
	}

	ctx := context.Background()

	vaultID, err := f.resolveVault(ctx, vaultName)
	if err != nil {
		return err
	}

	if docPath == "/" {
		return fmt.Errorf("cannot remove vault root: %w", os.ErrPermission)
	}

	// Try as document
	doc, err := f.dbClient.GetDocumentByPath(ctx, vaultID, docPath)
	if err != nil {
		return fmt.Errorf("remove %s: %w", filename, err)
	}
	if doc != nil {
		if err := f.docService.Delete(ctx, vaultID, docPath); err != nil {
			return fmt.Errorf("remove %s: %w", filename, err)
		}
		return nil
	}

	// Try as folder
	folder, err := f.dbClient.GetFolderByPath(ctx, vaultID, docPath)
	if err != nil {
		return fmt.Errorf("remove %s: %w", filename, err)
	}
	if folder != nil {
		if err := f.vaultSvc.DeleteFolder(ctx, vaultID, docPath); err != nil {
			return fmt.Errorf("remove folder %s: %w", filename, err)
		}
		return nil
	}

	return os.ErrNotExist
}

// Join joins path elements.
func (f *FS) Join(elem ...string) string {
	return path.Join(elem...)
}

// ReadDir reads the directory named by dirname and returns a sorted list of entries.
func (f *FS) ReadDir(dirname string) ([]os.FileInfo, error) {
	vaultName, docPath := parsePath(dirname)

	ctx := context.Background()

	if vaultName == "" {
		return f.listVaults(ctx)
	}

	vaultID, err := f.resolveVault(ctx, vaultName)
	if err != nil {
		return nil, err
	}

	return f.listDirEntries(ctx, vaultID, docPath)
}

// MkdirAll creates a directory path.
func (f *FS) MkdirAll(filename string, _ os.FileMode) error {
	vaultName, docPath := parsePath(filename)
	if vaultName == "" {
		return fmt.Errorf("cannot create vault via NFS: %w", os.ErrPermission)
	}

	ctx := context.Background()

	vaultID, err := f.resolveVault(ctx, vaultName)
	if err != nil {
		return err
	}

	if docPath == "/" {
		return nil // vault root always exists
	}

	_, err = f.vaultSvc.CreateFolder(ctx, vaultID, docPath)
	if err != nil {
		return fmt.Errorf("mkdir %s: %w", filename, err)
	}
	return nil
}

// TempFile is not supported.
func (f *FS) TempFile(_, _ string) (billy.File, error) {
	return nil, billy.ErrNotSupported
}

// Lstat returns file info (no symlinks, so same as Stat).
func (f *FS) Lstat(filename string) (os.FileInfo, error) {
	return f.Stat(filename)
}

// Symlink is not supported.
func (f *FS) Symlink(_, _ string) error {
	return billy.ErrNotSupported
}

// Readlink is not supported.
func (f *FS) Readlink(_ string) (string, error) {
	return "", billy.ErrNotSupported
}

// Chroot returns a chrooted filesystem.
func (f *FS) Chroot(p string) (billy.Filesystem, error) {
	return &chrootFS{parent: f, root: p}, nil
}

// Root returns the root path of the filesystem.
func (f *FS) Root() string {
	return "/"
}

// listVaults returns all accessible vaults as directory entries.
func (f *FS) listVaults(ctx context.Context) ([]os.FileInfo, error) {
	vaults, err := f.vaultSvc.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list vaults: %w", err)
	}

	var entries []os.FileInfo
	for _, v := range vaults {
		id, err := models.RecordIDString(v.ID)
		if err != nil {
			f.logger.Warn("skipping vault with invalid ID", "vault", v.Name, "error", err)
			continue
		}
		if err := auth.CheckVaultRole(f.ac, id, models.RoleRead); err != nil {
			continue
		}
		entries = append(entries, &fileInfo{
			name:    v.Name,
			isDir:   true,
			modTime: v.CreatedAt,
		})
	}

	return entries, nil
}

// listDirEntries returns the immediate children (folders + documents) of a directory.
func (f *FS) listDirEntries(ctx context.Context, vaultID, dirPath string) ([]os.FileInfo, error) {
	var entries []os.FileInfo

	// List immediate child folders
	childFolders, err := f.vaultSvc.ListFolders(ctx, vaultID, &dirPath)
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

	// List immediate child documents
	folderFilter := dirPath
	if folderFilter != "/" {
		folderFilter += "/"
	}
	metas, err := f.dbClient.ListDocumentMetas(ctx, db.ListDocumentsFilter{
		VaultID: vaultID,
		Folder:  &folderFilter,
		Limit:   maxListEntries,
	})
	if err != nil {
		return nil, fmt.Errorf("list documents in %s: %w", dirPath, err)
	}
	for _, meta := range metas {
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

// chrootFS wraps an FS with a path prefix for the billy.Chroot interface.
type chrootFS struct {
	parent *FS
	root   string
}

// resolve joins the chroot root with the given path and validates that the
// result stays within the chroot boundary (defense-in-depth against path
// traversal via ".." components).
func (c *chrootFS) resolve(filename string) (string, error) {
	joined := path.Join(c.root, filename)
	if !strings.HasPrefix(joined, c.root) {
		return "", fmt.Errorf("path escapes chroot boundary: %w", os.ErrPermission)
	}
	return joined, nil
}

func (c *chrootFS) Create(filename string) (billy.File, error) {
	p, err := c.resolve(filename)
	if err != nil {
		return nil, err
	}
	return c.parent.Create(p)
}

func (c *chrootFS) Open(filename string) (billy.File, error) {
	p, err := c.resolve(filename)
	if err != nil {
		return nil, err
	}
	return c.parent.Open(p)
}

func (c *chrootFS) OpenFile(filename string, flag int, perm os.FileMode) (billy.File, error) {
	p, err := c.resolve(filename)
	if err != nil {
		return nil, err
	}
	return c.parent.OpenFile(p, flag, perm)
}

func (c *chrootFS) Stat(filename string) (os.FileInfo, error) {
	p, err := c.resolve(filename)
	if err != nil {
		return nil, err
	}
	return c.parent.Stat(p)
}

func (c *chrootFS) Rename(oldpath, newpath string) error {
	op, err := c.resolve(oldpath)
	if err != nil {
		return err
	}
	np, err := c.resolve(newpath)
	if err != nil {
		return err
	}
	return c.parent.Rename(op, np)
}

func (c *chrootFS) Remove(filename string) error {
	p, err := c.resolve(filename)
	if err != nil {
		return err
	}
	return c.parent.Remove(p)
}

func (c *chrootFS) Join(elem ...string) string { return path.Join(elem...) }

func (c *chrootFS) ReadDir(dirname string) ([]os.FileInfo, error) {
	p, err := c.resolve(dirname)
	if err != nil {
		return nil, err
	}
	return c.parent.ReadDir(p)
}

func (c *chrootFS) MkdirAll(filename string, perm os.FileMode) error {
	p, err := c.resolve(filename)
	if err != nil {
		return err
	}
	return c.parent.MkdirAll(p, perm)
}

func (c *chrootFS) TempFile(dir, prefix string) (billy.File, error) {
	return nil, billy.ErrNotSupported
}

func (c *chrootFS) Lstat(filename string) (os.FileInfo, error) {
	p, err := c.resolve(filename)
	if err != nil {
		return nil, err
	}
	return c.parent.Lstat(p)
}

func (c *chrootFS) Symlink(target, link string) error { return billy.ErrNotSupported }

func (c *chrootFS) Readlink(link string) (string, error) { return "", billy.ErrNotSupported }

func (c *chrootFS) Chroot(p string) (billy.Filesystem, error) {
	return &chrootFS{parent: c.parent, root: path.Join(c.root, p)}, nil
}

func (c *chrootFS) Root() string { return c.root }
