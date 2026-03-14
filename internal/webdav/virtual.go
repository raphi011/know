package webdav

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"
	"time"

	"golang.org/x/net/webdav"

	"github.com/raphi011/know/internal/db"
)

// VirtualFile generates dynamic markdown content from DB state.
// Virtual files appear as read-only entries in the root directory listing.
type VirtualFile interface {
	Name() string                                                 // e.g. ".labels.md"
	Generate(ctx context.Context, vaultID string) (string, error) // returns markdown content
}

// findVirtualFile returns the VirtualFile matching the given root-level path, or nil.
func (f *FS) findVirtualFile(name string) VirtualFile {
	base := path.Base(name)
	for _, vf := range f.virtualFiles {
		if vf.Name() == base {
			return vf
		}
	}
	return nil
}

// isVirtualFilePath returns true if the path refers to a virtual file at the root level.
func (f *FS) isVirtualFilePath(name string) bool {
	return path.Dir(name) == "/" && f.findVirtualFile(name) != nil
}

// virtualReadFile provides read-only access to generated virtual content.
type virtualReadFile struct {
	name    string
	content string
	reader  *bytes.Reader
}

func newVirtualReadFile(name, content string) *virtualReadFile {
	return &virtualReadFile{
		name:    name,
		content: content,
		reader:  bytes.NewReader([]byte(content)),
	}
}

func (f *virtualReadFile) Read(p []byte) (int, error) { return f.reader.Read(p) }
func (f *virtualReadFile) Write([]byte) (int, error)  { return 0, os.ErrPermission }
func (f *virtualReadFile) Seek(offset int64, whence int) (int64, error) {
	return f.reader.Seek(offset, whence)
}
func (f *virtualReadFile) Close() error                       { return nil }
func (f *virtualReadFile) Readdir(int) ([]fs.FileInfo, error) { return nil, os.ErrInvalid }
func (f *virtualReadFile) Stat() (fs.FileInfo, error) {
	return &fileInfo{
		name:        path.Base(f.name),
		size:        int64(len(f.content)),
		modTime:     time.Now(),
		contentType: markdownContentType,
	}, nil
}

// labelsFile generates a markdown overview of all labels and their documents.
type labelsFile struct {
	db *db.Client
}

func (lf *labelsFile) Name() string { return ".labels.md" }

func (lf *labelsFile) Generate(ctx context.Context, vaultID string) (string, error) {
	labels, err := lf.db.ListLabels(ctx, vaultID)
	if err != nil {
		return "", fmt.Errorf("generate labels: %w", err)
	}

	if len(labels) == 0 {
		return "# Labels\n\nNo labels found.\n", nil
	}

	var b strings.Builder
	b.WriteString("# Labels\n")

	for _, label := range labels {
		docs, err := lf.db.ListDocuments(ctx, db.ListDocumentsFilter{
			VaultID: vaultID,
			Labels:  []string{label},
			Limit:   1000,
		})
		if err != nil {
			return "", fmt.Errorf("generate labels: list docs for %q: %w", label, err)
		}

		fmt.Fprintf(&b, "\n## %s\n\n", label)
		if len(docs) == 0 {
			b.WriteString("No documents.\n")
			continue
		}
		for _, doc := range docs {
			title := doc.Title
			if title == "" {
				title = path.Base(doc.Path)
			}
			fmt.Fprintf(&b, "- [%s](%s)\n", title, doc.Path)
		}
	}

	return b.String(), nil
}

// recentFile generates a markdown table of recently updated documents.
type recentFile struct {
	db *db.Client
}

func (rf *recentFile) Name() string { return ".recent.md" }

func (rf *recentFile) Generate(ctx context.Context, vaultID string) (string, error) {
	docs, err := rf.db.ListDocuments(ctx, db.ListDocumentsFilter{
		VaultID: vaultID,
		OrderBy: db.OrderByUpdatedAtDesc,
		Limit:   50,
	})
	if err != nil {
		return "", fmt.Errorf("generate recent: %w", err)
	}

	var b strings.Builder
	b.WriteString("# Recently Updated\n\n")

	if len(docs) == 0 {
		b.WriteString("No documents found.\n")
		return b.String(), nil
	}

	b.WriteString("| Document | Path | Modified |\n")
	b.WriteString("|----------|------|----------|\n")

	for _, doc := range docs {
		title := doc.Title
		if title == "" {
			title = path.Base(doc.Path)
		}
		modified := doc.UpdatedAt.Format(time.DateOnly)
		fmt.Fprintf(&b, "| %s | %s | %s |\n", title, doc.Path, modified)
	}

	return b.String(), nil
}

// defaultVirtualFiles returns the standard set of virtual files for a vault.
func defaultVirtualFiles(dbClient *db.Client) []VirtualFile {
	return []VirtualFile{
		&labelsFile{db: dbClient},
		&recentFile{db: dbClient},
	}
}

var (
	_ io.ReadSeeker = (*virtualReadFile)(nil)
	_ webdav.File   = (*virtualReadFile)(nil)
)
