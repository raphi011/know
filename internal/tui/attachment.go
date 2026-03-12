package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
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

// 1MB per file. Server-side request limit is 5MB total (see agent/handler.go).
const maxAttachmentSize = 1 << 20

// Attachment holds a resolved @-reference with its file content.
type Attachment struct {
	Path     string   // original path from @-reference
	AbsPath  string   // resolved absolute path
	Content  string   // file text content (only for text files)
	MimeType string
	Language string   // code fence language hint (e.g. "go", "python")
	Type     FileType
	Size     int64
	Error    string   // non-empty if file couldn't be read
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

// atRefRegex matches @-prefixed file paths. Supports:
// - relative: @./foo.go, @../bar/baz.py
// - absolute: @/usr/local/file.txt
// - tilde: @~/Documents/notes.md
// - bare: @file.go (must contain a dot to avoid matching @mentions)
//
// Paths with spaces or unicode are not matched; use a relative path
// like @./path\ with\ spaces. Extensionless filenames (e.g. @Makefile)
// are not matched; use @./Makefile instead.
//
// The regex requires @ to be preceded by start-of-string or whitespace
// to avoid matching email-like patterns (e.g. user@config.yaml).
var atRefRegex = regexp.MustCompile(`(?:^|\s)@((?:\.\.?/[\w./_\-]+)|(?:~/[\w./_\-]+)|(?:/[\w./_\-]+)|(?:[\w.\-]+\.[\w]+))`)

// parseAtRefs extracts @-prefixed file paths from input text.
func parseAtRefs(input string) []string {
	matches := atRefRegex.FindAllStringSubmatch(input, -1)
	seen := make(map[string]struct{}, len(matches))
	var refs []string
	for _, m := range matches {
		ref := m[1]
		if _, ok := seen[ref]; ok {
			continue
		}
		seen[ref] = struct{}{}
		refs = append(refs, ref)
	}
	return refs
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
	if fileType == FileTypeImage {
		att.Error = fmt.Sprintf("image attachments not yet supported: %s", ref)
		return att
	}

	data, err := os.ReadFile(abs)
	if err != nil {
		att.Error = fmt.Sprintf("read file: %v", err)
		return att
	}

	if int64(len(data)) > maxAttachmentSize {
		att.Error = fmt.Sprintf("file too large (%d bytes, max %d): %s", len(data), maxAttachmentSize, ref)
		return att
	}

	if len(data) == 0 {
		att.Error = fmt.Sprintf("file is empty: %s", ref)
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
	case ".png", ".jpg", ".jpeg", ".gif", ".svg", ".webp", ".bmp", ".ico":
		return FileTypeImage
	default:
		return FileTypeBinary
	}
}

// langForExt maps file extensions to code fence language hints.
func langForExt(ext string) string {
	m := map[string]string{
		".go":     "go",
		".py":     "python",
		".js":     "javascript",
		".ts":     "typescript",
		".tsx":    "tsx",
		".jsx":    "jsx",
		".rs":     "rust",
		".rb":     "ruby",
		".java":   "java",
		".kt":     "kotlin",
		".kts":    "kotlin",
		".c":      "c",
		".h":      "c",
		".cpp":    "cpp",
		".hpp":    "cpp",
		".cs":     "csharp",
		".swift":  "swift",
		".scala":  "scala",
		".sh":     "bash",
		".bash":   "bash",
		".zsh":    "zsh",
		".fish":   "fish",
		".ps1":    "powershell",
		".md":     "markdown",
		".json":   "json",
		".yaml":   "yaml",
		".yml":    "yaml",
		".toml":   "toml",
		".xml":    "xml",
		".html":   "html",
		".css":    "css",
		".scss":   "scss",
		".less":   "less",
		".sql":    "sql",
		".graphql": "graphql",
		".gql":    "graphql",
		".proto":  "protobuf",
		".tf":     "terraform",
		".hcl":    "hcl",
		".lua":    "lua",
		".vim":    "vim",
		".r":      "r",
		".jl":     "julia",
		".zig":    "zig",
		".nim":    "nim",
	}
	if lang, ok := m[ext]; ok {
		return lang
	}
	return ""
}

// mimeForExt returns a MIME type for the given extension, defaulting to text/plain.
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
	default:
		return "text/plain"
	}
}

// stripAtRefs removes @path tokens from input, cleaning up extra whitespace.
func stripAtRefs(input string, refs []string) string {
	result := input
	for _, ref := range refs {
		result = strings.ReplaceAll(result, "@"+ref, "")
	}
	// Collapse multiple spaces into one and trim
	result = strings.Join(strings.Fields(result), " ")
	return result
}
