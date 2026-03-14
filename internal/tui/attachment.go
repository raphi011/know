package tui

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/raphi011/know/internal/models"
)

// FileType classifies an attachment by content kind.
type FileType int

const (
	FileTypeUnknown FileType = iota
	FileTypeText
	FileTypeImage
	FileTypeBinary
)

// String returns a human-readable representation of the file type.
func (ft FileType) String() string {
	switch ft {
	case FileTypeText:
		return "text"
	case FileTypeImage:
		return "image"
	case FileTypeBinary:
		return "binary"
	default:
		return "unknown"
	}
}

// 1MB per text file. Server-side request limit is 50MB total (see agent/handler.go).
const maxAttachmentSize = 1 << 20

// 10MB per image file. Vision APIs accept larger payloads than text.
const maxImageAttachmentSize = 10 << 20

// Attachment holds a resolved file reference with its content.
type Attachment struct {
	Path     string // original path as provided (before resolution)
	AbsPath  string // resolved absolute path
	Content  string // file text (for text files) or base64-encoded data (for images)
	MimeType string
	Language string // code fence language hint (e.g. "go", "python")
	Type     FileType
	Size     int64
	Error    string // non-empty if file couldn't be read
}

// Name returns the basename of the resolved path.
func (a Attachment) Name() string {
	return filepath.Base(a.AbsPath)
}

// LineCount returns the number of lines in the attachment content.
// A trailing newline does not count as an extra line.
func (a Attachment) LineCount() int {
	if a.Content == "" {
		return 0
	}
	n := strings.Count(a.Content, "\n")
	if !strings.HasSuffix(a.Content, "\n") {
		n++
	}
	return n
}

// resolveAttachments expands paths, reads files, and classifies them.
func resolveAttachments(refs []string) []Attachment {
	attachments := make([]Attachment, 0, len(refs))
	for _, ref := range refs {
		att := resolveOne(ref)
		attachments = append(attachments, att)
	}
	return attachments
}

func resolveOne(ref string) Attachment {
	expanded, err := expandPath(ref)
	if err != nil {
		return Attachment{Path: ref, Error: err.Error()}
	}

	abs, err := filepath.Abs(expanded)
	if err != nil {
		return Attachment{Path: ref, Error: fmt.Sprintf("resolve path: %v", err)}
	}

	info, err := os.Stat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return Attachment{Path: ref, AbsPath: abs, Error: fmt.Sprintf("file not found: %s", ref)}
		}
		return Attachment{Path: ref, AbsPath: abs, Error: fmt.Sprintf("cannot access %s: %v", ref, err)}
	}
	if info.IsDir() {
		return Attachment{Path: ref, AbsPath: abs, Error: fmt.Sprintf("path is a directory: %s", ref)}
	}

	ext := strings.ToLower(filepath.Ext(abs))
	fileType := classifyFile(ext)
	att := Attachment{
		Path:    ref,
		AbsPath: abs,
		Type:    fileType,
		Size:    info.Size(),
	}

	if fileType == FileTypeBinary {
		att.Error = fmt.Sprintf("binary file not supported: %s", ref)
		return att
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		att.Error = fmt.Sprintf("read file: %v", err)
		return att
	}

	if len(data) == 0 {
		att.Error = fmt.Sprintf("file is empty: %s", ref)
		return att
	}

	if fileType == FileTypeImage {
		if int64(len(data)) > maxImageAttachmentSize {
			att.Error = fmt.Sprintf("file too large (%s, max %s): %s", formatSize(int64(len(data))), formatSize(maxImageAttachmentSize), ref)
			return att
		}
		att.Content = base64.StdEncoding.EncodeToString(data)
		att.MimeType = mimeForExt(ext)
		return att
	}

	if int64(len(data)) > maxAttachmentSize {
		att.Error = fmt.Sprintf("file too large (%s, max %s): %s", formatSize(int64(len(data))), formatSize(maxAttachmentSize), ref)
		return att
	}

	att.Content = string(data)
	att.Language = langForExt(ext)
	att.MimeType = mimeForExt(ext)
	return att
}

// expandPath expands ~/... paths to the user's home directory.
func expandPath(p string) (string, error) {
	if strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("expand home directory: %w", err)
		}
		return filepath.Join(home, p[2:]), nil
	}
	return p, nil
}

// classifyFile returns the file type based on extension.
func classifyFile(ext string) FileType {
	switch ext {
	case ".go", ".py", ".js", ".ts", ".tsx", ".jsx", ".rs", ".rb", ".java",
		".kt", ".kts", ".c", ".h", ".cpp", ".hpp", ".cs", ".swift", ".scala",
		".sh", ".bash", ".zsh", ".fish", ".ps1",
		".md", ".txt", ".rst", ".adoc",
		".json", ".yaml", ".yml", ".toml", ".xml", ".csv", ".tsv",
		".html", ".css", ".scss", ".less", ".sass",
		".sql", ".graphql", ".gql",
		".proto", ".tf", ".hcl",
		".env", ".ini", ".cfg", ".conf",
		".dockerfile", ".makefile",
		".lua", ".vim", ".el",
		".r", ".jl", ".m", ".mm",
		".zig", ".nim", ".v", ".d",
		".gitignore", ".gitattributes", ".editorconfig",
		".mod", ".sum", ".lock":
		return FileTypeText
	case ".png", ".jpg", ".jpeg", ".gif", ".webp":
		return FileTypeImage
	default:
		return FileTypeBinary
	}
}

// langForExt maps file extensions to code fence language hints.
func langForExt(ext string) string {
	m := map[string]string{
		".go":      "go",
		".py":      "python",
		".js":      "javascript",
		".ts":      "typescript",
		".tsx":     "tsx",
		".jsx":     "jsx",
		".rs":      "rust",
		".rb":      "ruby",
		".java":    "java",
		".kt":      "kotlin",
		".kts":     "kotlin",
		".c":       "c",
		".h":       "c",
		".cpp":     "cpp",
		".hpp":     "cpp",
		".cs":      "csharp",
		".swift":   "swift",
		".scala":   "scala",
		".sh":      "bash",
		".bash":    "bash",
		".zsh":     "zsh",
		".fish":    "fish",
		".ps1":     "powershell",
		".md":      "markdown",
		".json":    "json",
		".yaml":    "yaml",
		".yml":     "yaml",
		".toml":    "toml",
		".xml":     "xml",
		".html":    "html",
		".css":     "css",
		".scss":    "scss",
		".less":    "less",
		".sql":     "sql",
		".graphql": "graphql",
		".gql":     "graphql",
		".proto":   "protobuf",
		".tf":      "terraform",
		".hcl":     "hcl",
		".lua":     "lua",
		".vim":     "vim",
		".r":       "r",
		".jl":      "julia",
		".zig":     "zig",
		".nim":     "nim",
	}
	if lang, ok := m[ext]; ok {
		return lang
	}
	return ""
}

// mimeForExt returns a MIME type for the given extension.
func mimeForExt(ext string) string {
	switch ext {
	case ".json":
		return "application/json"
	case ".xml":
		return "application/xml"
	case ".html":
		return "text/html"
	case ".css":
		return "text/css"
	case ".js":
		return "text/javascript"
	case ".md":
		return "text/markdown"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	default:
		return "text/plain"
	}
}

// formatSize returns a human-readable file size (e.g. "1.2 MB", "450 KB").
func formatSize(bytes int64) string {
	switch {
	case bytes >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(1<<20))
	case bytes >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(1<<10))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

// toAttachmentType converts a TUI FileType to a wire-format AttachmentType.
func toAttachmentType(ft FileType) models.AttachmentType {
	switch ft {
	case FileTypeImage:
		return models.AttachmentTypeImage
	case FileTypeText:
		return models.AttachmentTypeText
	default:
		return models.AttachmentTypeText
	}
}

// looksLikeFilePath returns true if the string appears to be a filesystem path
// (single-line, starts with /, ~/, ./, or ../).
func looksLikeFilePath(s string) bool {
	if strings.Contains(s, "\n") {
		return false
	}
	return strings.HasPrefix(s, "/") ||
		strings.HasPrefix(s, "~/") ||
		strings.HasPrefix(s, "./") ||
		strings.HasPrefix(s, "../")
}
