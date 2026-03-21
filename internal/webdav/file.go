package webdav

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path"
	"time"

	"github.com/raphi011/know/internal/blob"
	"github.com/raphi011/know/internal/file"
	"github.com/raphi011/know/internal/models"
)

// readFile provides read-only access to a document's content.
type readFile struct {
	name        string
	size        int64
	modTime     time.Time
	contentHash *string
	reader      *bytes.Reader
}

func newReadFile(name string, content []byte, modTime time.Time, contentHash *string) *readFile {
	return &readFile{
		name:        name,
		size:        int64(len(content)),
		modTime:     modTime,
		contentHash: contentHash,
		reader:      bytes.NewReader(content),
	}
}

// newEmptyReadFile returns a read file with no content. Used for pending paths
// that have been claimed but not yet written — allows PROPFIND and GET to succeed.
func newEmptyReadFile(name string) *readFile {
	return &readFile{
		name:    name,
		modTime: time.Now(),
		reader:  bytes.NewReader(nil),
	}
}

func (f *readFile) Read(p []byte) (int, error) { return f.reader.Read(p) }
func (f *readFile) Write([]byte) (int, error)  { return 0, os.ErrPermission }
func (f *readFile) Seek(offset int64, whence int) (int64, error) {
	return f.reader.Seek(offset, whence)
}
func (f *readFile) Close() error { return nil }
func (f *readFile) Readdir(count int) ([]fs.FileInfo, error) {
	return nil, os.ErrInvalid
}
func (f *readFile) Stat() (fs.FileInfo, error) {
	fi := &fileInfo{
		name:        path.Base(f.name),
		size:        f.size,
		modTime:     f.modTime,
		contentType: markdownContentType,
	}
	if f.contentHash != nil {
		fi.etag = `"` + *f.contentHash + `"`
	}
	return fi, nil
}

// writeFile buffers writes and triggers the document pipeline on Close().
type writeFile struct {
	name       string
	vaultID    string
	docService *file.Service
	pending    *pendingSet
	isNew      bool
	buf        bytes.Buffer
	modTime    time.Time
}

func newWriteFile(name, vaultID string, docService *file.Service, initial []byte, modTime time.Time, isNew bool, pending *pendingSet) *writeFile {
	wf := &writeFile{
		name:       name,
		vaultID:    vaultID,
		docService: docService,
		pending:    pending,
		isNew:      isNew,
		modTime:    modTime,
	}
	if initial != nil {
		// bytes.Buffer.Write never returns a non-nil error; safe to ignore.
		wf.buf.Write(initial)
	}
	return wf
}

func (f *writeFile) Read([]byte) (int, error)           { return 0, os.ErrPermission }
func (f *writeFile) Readdir(int) ([]fs.FileInfo, error) { return nil, os.ErrInvalid }
func (f *writeFile) Seek(int64, int) (int64, error)     { return 0, os.ErrPermission }

func (f *writeFile) Write(p []byte) (int, error) {
	return f.buf.Write(p)
}

// Close persists the document. Finder sends an initial PUT with
// Content-Length=0 to "claim" the file, then verifies with PROPFIND.
// To avoid ghost empty documents when the copy is aborted mid-way,
// empty content on new files is deferred to the pending set instead of
// being written to the DB. Non-empty content always persists immediately.
func (f *writeFile) Close() error {
	content := f.buf.String()

	// New file with empty content — add to pending set, skip DB write.
	// Finder will send the real content in a subsequent PUT.
	if content == "" && f.isNew {
		f.pending.Add(f.name)
		slog.Debug("webdav: document deferred to pending set", "path", f.name, "vault", f.vaultID)
		return nil
	}

	// Real content arrived — remove from pending and persist to DB.
	f.pending.Remove(f.name)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := f.docService.Create(ctx, models.FileInput{
		VaultID: f.vaultID,
		Path:    f.name,
		Content: content,
	})
	if err != nil {
		slog.Error("webdav: failed to save document on close",
			"path", f.name, "vault", f.vaultID, "error", err)
		return fmt.Errorf("close %s: %w", f.name, err)
	}

	slog.Info("webdav: document saved", "path", f.name, "vault", f.vaultID, "size", len(content))
	return nil
}

func (f *writeFile) Stat() (fs.FileInfo, error) {
	return &fileInfo{
		name:        path.Base(f.name),
		size:        int64(f.buf.Len()),
		modTime:     f.modTime,
		contentType: markdownContentType,
	}, nil
}

// dirFile provides directory listing.
type dirFile struct {
	name    string
	modTime time.Time
	entries []fs.FileInfo
	pos     int
}

func newDirFile(name string, modTime time.Time, entries []fs.FileInfo) *dirFile {
	return &dirFile{name: name, modTime: modTime, entries: entries}
}

func (d *dirFile) Read([]byte) (int, error)       { return 0, os.ErrInvalid }
func (d *dirFile) Write([]byte) (int, error)      { return 0, os.ErrPermission }
func (d *dirFile) Seek(int64, int) (int64, error) { return 0, os.ErrInvalid }
func (d *dirFile) Close() error                   { return nil }

func (d *dirFile) Readdir(count int) ([]fs.FileInfo, error) {
	if count <= 0 {
		entries := d.entries[d.pos:]
		d.pos = len(d.entries)
		return entries, nil
	}

	if d.pos >= len(d.entries) {
		return nil, io.EOF
	}

	end := min(d.pos+count, len(d.entries))
	entries := d.entries[d.pos:end]
	d.pos = end

	if d.pos >= len(d.entries) {
		return entries, io.EOF
	}
	return entries, nil
}

func (d *dirFile) Stat() (fs.FileInfo, error) {
	return &fileInfo{
		name:    path.Base(d.name),
		isDir:   true,
		modTime: d.modTime,
	}, nil
}

// nopFile silently accepts and discards writes. Used for macOS metadata
// files (._*, .DS_Store) and unsupported file types (.pdf, .txt, .docx, etc.)
// so Finder doesn't abort batch drag-and-drop operations.
type nopFile struct {
	name    string
	modTime time.Time
}

func newNopFile(name string) *nopFile {
	return &nopFile{name: name, modTime: time.Now()}
}
func (f *nopFile) Read([]byte) (int, error)       { return 0, io.EOF }
func (f *nopFile) Write(p []byte) (int, error)    { return len(p), nil }
func (f *nopFile) Seek(int64, int) (int64, error) { return 0, nil }
func (f *nopFile) Close() error {
	slog.Debug("webdav: discarded OS metadata file", "path", f.name)
	return nil
}
func (f *nopFile) Readdir(int) ([]fs.FileInfo, error) { return nil, os.ErrInvalid }
func (f *nopFile) Stat() (fs.FileInfo, error) {
	return &fileInfo{name: path.Base(f.name), modTime: f.modTime}, nil
}

// assetReadFile provides read-only access to an asset's binary data.
type assetReadFile struct {
	name   string
	asset  *models.File
	reader *bytes.Reader
}

func newAssetReadFile(ctx context.Context, name string, a *models.File, blobStore blob.Store) (*assetReadFile, error) {
	if a.Hash == nil {
		return nil, fmt.Errorf("asset %s has no content hash", name)
	}
	rc, err := blobStore.Get(ctx, *a.Hash)
	if err != nil {
		return nil, fmt.Errorf("read blob for %s: %w", name, err)
	}
	data, err := io.ReadAll(rc)
	rc.Close()
	if err != nil {
		return nil, fmt.Errorf("read blob data for %s: %w", name, err)
	}
	return &assetReadFile{
		name:   name,
		asset:  a,
		reader: bytes.NewReader(data),
	}, nil
}

func (f *assetReadFile) Read(p []byte) (int, error) { return f.reader.Read(p) }
func (f *assetReadFile) Write([]byte) (int, error)  { return 0, os.ErrPermission }
func (f *assetReadFile) Seek(offset int64, whence int) (int64, error) {
	return f.reader.Seek(offset, whence)
}
func (f *assetReadFile) Close() error                       { return nil }
func (f *assetReadFile) Readdir(int) ([]fs.FileInfo, error) { return nil, os.ErrInvalid }
func (f *assetReadFile) Stat() (fs.FileInfo, error) {
	return &fileInfo{
		name:        path.Base(f.name),
		size:        int64(f.reader.Len()),
		modTime:     f.asset.UpdatedAt,
		contentType: f.asset.MimeType,
		etag:        contentHashETag(f.asset.Hash),
	}, nil
}

// assetWriteFile buffers writes and stores the asset on Close().
type assetWriteFile struct {
	name    string
	vaultID string
	fileSvc *file.Service
	pending *pendingSet
	isNew   bool
	buf     bytes.Buffer
	modTime time.Time
}

func newAssetWriteFile(name, vaultID string, fileSvc *file.Service, modTime time.Time, isNew bool, pending *pendingSet) *assetWriteFile {
	return &assetWriteFile{
		name:    name,
		vaultID: vaultID,
		fileSvc: fileSvc,
		pending: pending,
		isNew:   isNew,
		modTime: modTime,
	}
}

func (f *assetWriteFile) Read([]byte) (int, error)           { return 0, os.ErrPermission }
func (f *assetWriteFile) Readdir(int) ([]fs.FileInfo, error) { return nil, os.ErrInvalid }
func (f *assetWriteFile) Seek(int64, int) (int64, error)     { return 0, os.ErrPermission }

func (f *assetWriteFile) Write(p []byte) (int, error) {
	return f.buf.Write(p)
}

func (f *assetWriteFile) Close() error {
	data := f.buf.Bytes()

	// New asset with empty content — add to pending set, skip DB write.
	if len(data) == 0 && f.isNew {
		f.pending.Add(f.name)
		slog.Debug("webdav: asset deferred to pending set", "path", f.name, "vault", f.vaultID)
		return nil
	}

	// Real content arrived — remove from pending and persist to DB.
	f.pending.Remove(f.name)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := f.fileSvc.Create(ctx, models.FileInput{
		VaultID: f.vaultID,
		Path:    f.name,
		Data:    data,
	})
	if err != nil {
		slog.Error("webdav: failed to save asset on close",
			"path", f.name, "vault", f.vaultID, "error", err)
		return fmt.Errorf("close %s: %w", f.name, err)
	}

	slog.Info("webdav: asset saved", "path", f.name, "vault", f.vaultID, "size", len(data))
	return nil
}

func (f *assetWriteFile) Stat() (fs.FileInfo, error) {
	return &fileInfo{
		name:        path.Base(f.name),
		size:        int64(f.buf.Len()),
		modTime:     f.modTime,
		contentType: models.MimeTypeFromExt(f.name),
	}, nil
}
