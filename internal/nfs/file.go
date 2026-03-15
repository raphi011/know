package nfs

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"time"

	billy "github.com/go-git/go-billy/v5"

	"github.com/raphi011/know/internal/document"
	"github.com/raphi011/know/internal/models"
)

// errClosed is returned when writing to a file that has already been closed.
var errClosed = fmt.Errorf("file already closed: %w", os.ErrClosed)

// fileInfo implements os.FileInfo for NFS entries.
type fileInfo struct {
	name    string
	size    int64
	modTime time.Time
	isDir   bool
}

func (fi *fileInfo) Name() string       { return fi.name }
func (fi *fileInfo) Size() int64        { return fi.size }
func (fi *fileInfo) ModTime() time.Time { return fi.modTime }
func (fi *fileInfo) IsDir() bool        { return fi.isDir }
func (fi *fileInfo) Sys() any           { return nil }

func (fi *fileInfo) Mode() fs.FileMode {
	if fi.isDir {
		return fs.ModeDir | 0755
	}
	return 0644
}

// readFile wraps document content in a seekable reader implementing billy.File.
type readFile struct {
	name    string
	r       *bytes.Reader
	modTime time.Time
}

func newReadFile(name string, content []byte, modTime time.Time) *readFile {
	return &readFile{
		name:    name,
		r:       bytes.NewReader(content),
		modTime: modTime,
	}
}

func (f *readFile) Name() string                                 { return f.name }
func (f *readFile) Read(p []byte) (int, error)                   { return f.r.Read(p) }
func (f *readFile) ReadAt(p []byte, off int64) (int, error)      { return f.r.ReadAt(p, off) }
func (f *readFile) Seek(offset int64, whence int) (int64, error) { return f.r.Seek(offset, whence) }
func (f *readFile) Close() error                                 { return nil }
func (f *readFile) Lock() error                                  { return nil }
func (f *readFile) Unlock() error                                { return nil }
func (f *readFile) Truncate(_ int64) error                       { return billy.ErrReadOnly }

func (f *readFile) Write(_ []byte) (int, error) {
	return 0, billy.ErrReadOnly
}

// writeFile buffers writes in memory and saves on Close via docService.Create.
type writeFile struct {
	name       string
	path       string
	vaultID    string
	docService *document.Service
	logger     *slog.Logger
	buf        bytes.Buffer
	closed     bool
}

func (f *writeFile) Name() string { return f.name }

func (f *writeFile) Write(p []byte) (int, error) {
	if f.closed {
		return 0, errClosed
	}
	if f.buf.Len()+len(p) > maxDocSize {
		return 0, fmt.Errorf("document too large (max %d bytes): %w", maxDocSize, os.ErrPermission)
	}
	return f.buf.Write(p)
}

func (f *writeFile) Read(_ []byte) (int, error)            { return 0, billy.ErrNotSupported }
func (f *writeFile) ReadAt(_ []byte, _ int64) (int, error) { return 0, billy.ErrNotSupported }
func (f *writeFile) Seek(_ int64, _ int) (int64, error)    { return 0, billy.ErrNotSupported }
func (f *writeFile) Lock() error                           { return nil }
func (f *writeFile) Unlock() error                         { return nil }
func (f *writeFile) Truncate(_ int64) error                { f.buf.Reset(); return nil }

func (f *writeFile) Close() error {
	if f.closed {
		return nil
	}
	f.closed = true

	content := f.buf.String()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := f.docService.Create(ctx, models.DocumentInput{
		VaultID: f.vaultID,
		Path:    f.path,
		Content: content,
	})
	if err != nil {
		f.logger.Error("failed to save document on close",
			"path", f.path, "vault", f.vaultID, "error", err)
		return fmt.Errorf("close %s: %w", f.path, err)
	}

	f.logger.Info("document saved", "path", f.path, "vault", f.vaultID, "size", len(content))
	return nil
}

// nopFile is a no-op file used for directories and unsupported operations.
type nopFile struct {
	name string
}

func (f *nopFile) Name() string                          { return f.name }
func (f *nopFile) Write(_ []byte) (int, error)           { return 0, billy.ErrReadOnly }
func (f *nopFile) Read(_ []byte) (int, error)            { return 0, billy.ErrNotSupported }
func (f *nopFile) ReadAt(_ []byte, _ int64) (int, error) { return 0, billy.ErrNotSupported }
func (f *nopFile) Seek(_ int64, _ int) (int64, error)    { return 0, billy.ErrNotSupported }
func (f *nopFile) Close() error                          { return nil }
func (f *nopFile) Lock() error                           { return nil }
func (f *nopFile) Unlock() error                         { return nil }
func (f *nopFile) Truncate(_ int64) error                { return billy.ErrReadOnly }
