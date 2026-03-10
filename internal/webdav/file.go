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

	"github.com/raphi011/knowhow/internal/asset"
	"github.com/raphi011/knowhow/internal/document"
	"github.com/raphi011/knowhow/internal/models"
)

// readFile provides read-only access to a document's content.
type readFile struct {
	name    string
	doc     *models.Document
	reader  *bytes.Reader
	closeFn func()
}

func newReadFile(name string, doc *models.Document) *readFile {
	return &readFile{
		name:   name,
		doc:    doc,
		reader: bytes.NewReader([]byte(doc.Content)),
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
		size:        int64(len(f.doc.Content)),
		modTime:     f.doc.UpdatedAt,
		contentType: markdownContentType,
	}
	if f.doc.ContentHash != nil {
		fi.etag = `"` + *f.doc.ContentHash + `"`
	}
	return fi, nil
}

// writeFile buffers writes and triggers the document pipeline on Close().
type writeFile struct {
	name       string
	vaultID    string
	docService *document.Service
	buf     bytes.Buffer
	modTime time.Time
}

func newWriteFile(name, vaultID string, docService *document.Service, initial []byte, modTime time.Time) *writeFile {
	wf := &writeFile{
		name:       name,
		vaultID:    vaultID,
		docService: docService,
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
// We must create the document even when empty, otherwise PROPFIND → 404
// triggers Finder error -43.
func (f *writeFile) Close() error {
	content := f.buf.String()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := f.docService.Create(ctx, models.DocumentInput{
		VaultID: f.vaultID,
		Path:    f.name,
		Content: content,
		Source:  models.SourceManual,
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

	end := d.pos + count
	if end > len(d.entries) {
		end = len(d.entries)
	}
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
// files (._*, .DS_Store) so Finder doesn't abort the whole drag-and-drop.
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
	asset  *models.Asset
	reader *bytes.Reader
}

func newAssetReadFile(name string, a *models.Asset) *assetReadFile {
	return &assetReadFile{
		name:   name,
		asset:  a,
		reader: bytes.NewReader(a.Data),
	}
}

func (f *assetReadFile) Read(p []byte) (int, error) { return f.reader.Read(p) }
func (f *assetReadFile) Write([]byte) (int, error)  { return 0, os.ErrPermission }
func (f *assetReadFile) Seek(offset int64, whence int) (int64, error) {
	return f.reader.Seek(offset, whence)
}
func (f *assetReadFile) Close() error                           { return nil }
func (f *assetReadFile) Readdir(int) ([]fs.FileInfo, error) { return nil, os.ErrInvalid }
func (f *assetReadFile) Stat() (fs.FileInfo, error) {
	return &fileInfo{
		name:        path.Base(f.name),
		size:        int64(f.asset.Size),
		modTime:     f.asset.UpdatedAt,
		contentType: f.asset.MimeType,
		etag:        `"` + f.asset.ContentHash + `"`,
	}, nil
}

// assetWriteFile buffers writes and stores the asset on Close().
type assetWriteFile struct {
	name     string
	vaultID  string
	assetSvc *asset.Service
	buf      bytes.Buffer
	modTime  time.Time
}

func newAssetWriteFile(name, vaultID string, assetSvc *asset.Service, modTime time.Time) *assetWriteFile {
	return &assetWriteFile{
		name:     name,
		vaultID:  vaultID,
		assetSvc: assetSvc,
		modTime:  modTime,
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
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := f.assetSvc.Create(ctx, models.AssetInput{
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
