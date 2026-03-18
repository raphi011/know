package sshd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path"
	"strings"
	"time"

	"github.com/pkg/sftp"

	"github.com/raphi011/know/internal/auth"
	"github.com/raphi011/know/internal/db"
	"github.com/raphi011/know/internal/file"
	"github.com/raphi011/know/internal/models"
	"github.com/raphi011/know/internal/vault"
)

// maxDocSize is the maximum allowed document size via SFTP (10 MB).
const maxDocSize = 10 << 20

// handler satisfies the sftp.Handlers callback interfaces for FileGet, FilePut, FileCmd, and FileList.
type handler struct {
	dbClient   *db.Client
	docService *file.Service
	vaultSvc   *vault.Service
	ac         auth.AuthContext
}

func newHandler(dbClient *db.Client, docService *file.Service, vaultSvc *vault.Service, ac auth.AuthContext) *handler {
	return &handler{
		dbClient:   dbClient,
		docService: docService,
		vaultSvc:   vaultSvc,
		ac:         ac,
	}
}

// Handlers returns the sftp.Handlers struct for use with sftp.NewRequestServer.
func (h *handler) Handlers() sftp.Handlers {
	return sftp.Handlers{
		FileGet:  h,
		FilePut:  h,
		FileCmd:  h,
		FileList: h,
	}
}

// parsePath splits an SFTP path into vault name and document path within the vault.
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
// Logs the underlying error and maps it to os.ErrNotExist or os.ErrPermission.
func (h *handler) resolveVault(ctx context.Context, vaultName string) (string, error) {
	id, err := auth.ResolveVault(ctx, h.ac, h.vaultSvc, vaultName)
	if err != nil {
		slog.Warn("sftp: vault resolution failed", "vault", vaultName, "error", err)
		if errors.Is(err, os.ErrNotExist) {
			return "", os.ErrNotExist
		}
		return "", os.ErrPermission
	}
	return id, nil
}

// Fileread returns an io.ReaderAt for the requested file.
func (h *handler) Fileread(r *sftp.Request) (io.ReaderAt, error) {
	vaultName, docPath := parsePath(r.Filepath)
	if vaultName == "" {
		return nil, os.ErrInvalid
	}

	vaultID, err := h.resolveVault(r.Context(), vaultName)
	if err != nil {
		return nil, os.ErrPermission
	}

	doc, err := h.dbClient.GetFileByPath(r.Context(), vaultID, docPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", r.Filepath, err)
	}
	if doc == nil {
		return nil, os.ErrNotExist
	}

	content, err := h.docService.ReadFileContent(r.Context(), doc)
	if err != nil {
		return nil, fmt.Errorf("read content %s: %w", r.Filepath, err)
	}
	return bytes.NewReader([]byte(content)), nil
}

// Filewrite returns an io.WriterAt that buffers data in memory. On Close,
// the buffered content is saved via docService.Create using a background
// context with a 30s timeout. The sftp package detects the io.Closer
// implementation and calls Close() after all writes complete.
func (h *handler) Filewrite(r *sftp.Request) (io.WriterAt, error) {
	vaultName, docPath := parsePath(r.Filepath)
	if vaultName == "" {
		return nil, os.ErrInvalid
	}

	if !isMarkdownFile(docPath) {
		return nil, errNotMarkdown
	}

	vaultID, err := h.resolveVault(r.Context(), vaultName)
	if err != nil {
		return nil, os.ErrPermission
	}

	return &writeBuffer{
		path:       docPath,
		vaultID:    vaultID,
		docService: h.docService,
	}, nil
}

// Filecmd handles file commands: Mkdir, Remove, Rmdir, Rename, Setstat.
func (h *handler) Filecmd(r *sftp.Request) error {
	vaultName, docPath := parsePath(r.Filepath)

	switch r.Method {
	case "Mkdir":
		if vaultName == "" {
			return fmt.Errorf("cannot create vault via SFTP: %w", os.ErrPermission)
		}
		vaultID, err := h.resolveVault(r.Context(), vaultName)
		if err != nil {
			return os.ErrPermission
		}
		if docPath == "/" {
			return nil // vault root always exists
		}
		_, err = h.vaultSvc.CreateFolder(r.Context(), vaultID, docPath)
		if err != nil {
			return fmt.Errorf("mkdir %s: %w", docPath, err)
		}
		return nil

	case "Remove":
		if vaultName == "" {
			return os.ErrPermission
		}
		vaultID, err := h.resolveVault(r.Context(), vaultName)
		if err != nil {
			return os.ErrPermission
		}
		if err := h.docService.Delete(r.Context(), vaultID, docPath); err != nil {
			return fmt.Errorf("remove %s: %w", docPath, err)
		}
		return nil

	case "Rmdir":
		if vaultName == "" {
			return os.ErrPermission
		}
		vaultID, err := h.resolveVault(r.Context(), vaultName)
		if err != nil {
			return os.ErrPermission
		}
		if docPath == "/" {
			return fmt.Errorf("cannot remove vault root: %w", os.ErrPermission)
		}
		if err := h.vaultSvc.DeleteFolder(r.Context(), vaultID, docPath); err != nil {
			return fmt.Errorf("rmdir %s: %w", docPath, err)
		}
		return nil

	case "Rename":
		targetVault, targetPath := parsePath(r.Target)
		if vaultName == "" || targetVault == "" {
			return os.ErrPermission
		}
		if vaultName != targetVault {
			return fmt.Errorf("cross-vault rename not supported: %w", os.ErrPermission)
		}
		vaultID, err := h.resolveVault(r.Context(), vaultName)
		if err != nil {
			return os.ErrPermission
		}

		// Try as document first
		doc, err := h.dbClient.GetFileByPath(r.Context(), vaultID, docPath)
		if err != nil {
			return fmt.Errorf("rename %s: %w", docPath, err)
		}
		if doc != nil {
			if !isMarkdownFile(targetPath) {
				return errNotMarkdown
			}
			_, err = h.docService.Move(r.Context(), vaultID, docPath, targetPath)
			if err != nil {
				return fmt.Errorf("rename %s to %s: %w", docPath, targetPath, err)
			}
			return nil
		}

		// Try as folder
		folder, err := h.dbClient.GetFolderByPath(r.Context(), vaultID, docPath)
		if err != nil {
			return fmt.Errorf("rename %s: %w", docPath, err)
		}
		if folder != nil {
			if err := h.vaultSvc.MoveFolder(r.Context(), vaultID, docPath, targetPath); err != nil {
				return fmt.Errorf("rename folder %s to %s: %w", docPath, targetPath, err)
			}
			return nil
		}

		return os.ErrNotExist

	case "Setstat":
		return nil // no-op, we don't support setting attributes

	default:
		return fmt.Errorf("unsupported command: %s", r.Method)
	}
}

// Filelist handles List and Stat requests.
func (h *handler) Filelist(r *sftp.Request) (sftp.ListerAt, error) {
	vaultName, docPath := parsePath(r.Filepath)

	switch r.Method {
	case "List":
		return h.listDir(r.Context(), vaultName, docPath)

	case "Stat":
		if vaultName == "" {
			// Root directory
			return &listAt{entries: []os.FileInfo{
				&fileInfo{name: "/", isDir: true, modTime: time.Now()},
			}}, nil
		}

		vaultID, err := h.resolveVault(r.Context(), vaultName)
		if err != nil {
			return nil, os.ErrPermission
		}

		if docPath == "/" {
			// Vault root
			return &listAt{entries: []os.FileInfo{
				&fileInfo{name: vaultName, isDir: true, modTime: time.Now()},
			}}, nil
		}

		// Try as document
		meta, err := h.dbClient.GetFileMetaByPath(r.Context(), vaultID, docPath)
		if err != nil {
			return nil, fmt.Errorf("stat %s: %w", docPath, err)
		}
		if meta != nil {
			return &listAt{entries: []os.FileInfo{
				&fileInfo{
					name:    path.Base(docPath),
					size:    int64(meta.Size),
					modTime: meta.UpdatedAt,
				},
			}}, nil
		}

		// Try as folder
		folder, err := h.dbClient.GetFolderByPath(r.Context(), vaultID, docPath)
		if err != nil {
			return nil, fmt.Errorf("stat %s: %w", docPath, err)
		}
		if folder != nil {
			return &listAt{entries: []os.FileInfo{
				&fileInfo{
					name:    path.Base(docPath),
					isDir:   true,
					modTime: folder.CreatedAt,
				},
			}}, nil
		}

		return nil, os.ErrNotExist

	default:
		return nil, fmt.Errorf("unsupported list method: %s", r.Method)
	}
}

// listDir returns directory entries for the given vault/path.
func (h *handler) listDir(ctx context.Context, vaultName, docPath string) (sftp.ListerAt, error) {
	if vaultName == "" {
		// Root: list accessible vaults
		return h.listVaults(ctx)
	}

	vaultID, err := h.resolveVault(ctx, vaultName)
	if err != nil {
		return nil, os.ErrPermission
	}

	entries, err := h.listDirEntries(ctx, vaultID, docPath)
	if err != nil {
		return nil, fmt.Errorf("list %s: %w", docPath, err)
	}

	return &listAt{entries: entries}, nil
}

// listVaults returns all vaults the user can access as directory entries.
func (h *handler) listVaults(ctx context.Context) (sftp.ListerAt, error) {
	vaults, err := h.vaultSvc.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list vaults: %w", err)
	}

	var entries []os.FileInfo
	for _, v := range vaults {
		id, err := models.RecordIDString(v.ID)
		if err != nil {
			slog.Warn("sftp: skipping vault with invalid ID", "vault", v.Name, "error", err)
			continue
		}
		if err := auth.CheckVaultRole(h.ac, id, models.RoleRead); err != nil {
			continue
		}
		entries = append(entries, &fileInfo{
			name:    v.Name,
			isDir:   true,
			modTime: v.CreatedAt,
		})
	}

	return &listAt{entries: entries}, nil
}

// maxListEntries is the maximum number of documents returned in a directory listing.
const maxListEntries = 10000

// listDirEntries returns the immediate children (folders + documents) of a directory.
// Similar approach to webdav/fs.go:listDirEntries.
func (h *handler) listDirEntries(ctx context.Context, vaultID, dirPath string) ([]os.FileInfo, error) {
	var entries []os.FileInfo

	// List immediate child folders
	childFolders, err := h.vaultSvc.ListFolders(ctx, vaultID, &dirPath)
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

	// List immediate child documents (exclude folders to avoid duplicates)
	folderFilter := dirPath
	if folderFilter != "/" {
		folderFilter += "/"
	}
	isNotFolder := false
	metas, err := h.dbClient.ListFileMetas(ctx, db.ListFilesFilter{
		VaultID:  vaultID,
		Folder:   &folderFilter,
		IsFolder: &isNotFolder,
		Limit:    maxListEntries,
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
			size:    int64(meta.Size),
			modTime: meta.UpdatedAt,
		})
	}

	return entries, nil
}

// writeBuffer implements io.WriterAt and io.Closer. It buffers data in memory
// up to maxDocSize. On Close, it saves the document via the document service.
type writeBuffer struct {
	path       string
	vaultID    string
	docService *file.Service
	buf        []byte
}

func (w *writeBuffer) WriteAt(p []byte, off int64) (int, error) {
	if off < 0 {
		return 0, fmt.Errorf("negative offset: %w", os.ErrInvalid)
	}
	end := off + int64(len(p))
	if end > maxDocSize {
		return 0, fmt.Errorf("document too large (max %d bytes): %w", maxDocSize, os.ErrPermission)
	}
	if int(end) > len(w.buf) {
		newBuf := make([]byte, int(end))
		copy(newBuf, w.buf)
		w.buf = newBuf
	}
	copy(w.buf[off:], p)
	return len(p), nil
}

func (w *writeBuffer) Close() error {
	content := string(w.buf)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := w.docService.Create(ctx, models.FileInput{
		VaultID: w.vaultID,
		Path:    w.path,
		Content: content,
	})
	if err != nil {
		slog.Error("sftp: failed to save document on close",
			"path", w.path, "vault", w.vaultID, "error", err)
		return fmt.Errorf("close %s: %w", w.path, err)
	}

	slog.Info("sftp: document saved", "path", w.path, "vault", w.vaultID, "size", len(content))
	return nil
}

// listAt implements sftp.ListerAt for a static list of file infos.
type listAt struct {
	entries []os.FileInfo
}

func (l *listAt) ListAt(ls []os.FileInfo, offset int64) (int, error) {
	if offset >= int64(len(l.entries)) {
		return 0, io.EOF
	}

	n := copy(ls, l.entries[offset:])
	if int(offset)+n >= len(l.entries) {
		return n, io.EOF
	}
	return n, nil
}

// fileInfo implements os.FileInfo for SFTP entries.
type fileInfo struct {
	name    string
	size    int64
	modTime time.Time
	isDir   bool
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

// isMarkdownFile returns true if the file has a .md extension (case-insensitive).
func isMarkdownFile(name string) bool {
	return strings.EqualFold(path.Ext(name), ".md")
}

// errNotMarkdown is returned when a non-markdown file is created or renamed to.
var errNotMarkdown = fmt.Errorf("only markdown files (.md) are allowed: %w", os.ErrPermission)
