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
	FileTypeText   FileType = iota
	FileTypeImage
	FileTypeBinary
)

const maxAttachmentSize = 1 << 20 // 1MB

// Attachment holds a resolved @-reference with its file content.
type Attachment struct {
	Path     string   // original path from @-reference
	AbsPath  string   // resolved absolute path
	Name     string   // basename
	Content  string   // file text content (only for text files)
	MimeType string
	Language string   // code fence language hint (e.g. "go", "python")
	Type     FileType
	Size     int64
	Error    string   // non-empty if file couldn't be read
}

// LineCount returns the number of lines in the attachment content.
func (a Attachment) LineCount() int {
	if a.Content == "" {
		return 0
	}
	return strings.Count(a.Content, "\n") + 1
}

// atRefRegex matches @-prefixed file paths. Supports:
// - relative: @./foo.go, @../bar/baz.py
// - absolute: @/usr/local/file.txt
// - tilde: @~/Documents/notes.md
// - bare: @file.go (must contain a dot to avoid matching @mentions)
var atRefRegex = regexp.MustCompile(`@((?:\.\.?/[\w./_\-]+)|(?:~/[\w./_\-]+)|(?:/[\w./_\-]+)|(?:[\w.\-]+\.[\w]+))`)

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
	expanded := expandPath(ref)

	abs, err := filepath.Abs(expanded)
	if err != nil {
		return Attachment{Path: ref, Error: fmt.Sprintf("resolve path: %v", err)}
	}

	info, err := os.Stat(abs)
	if err != nil {
		return Attachment{Path: ref, AbsPath: abs, Error: fmt.Sprintf("file not found: %s", ref)}
	}
	if info.IsDir() {
		return Attachment{Path: ref, AbsPath: abs, Error: fmt.Sprintf("path is a directory: %s", ref)}
	}

	ext := strings.ToLower(filepath.Ext(abs))
	fileType := classifyFile(ext)
	att := Attachment{
		Path:    ref,
		AbsPath: abs,
		Name:    filepath.Base(abs),
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

	if info.Size() > maxAttachmentSize {
		att.Error = fmt.Sprintf("file too large (%d bytes, max %d): %s", info.Size(), maxAttachmentSize, ref)
		return att
	}

	data, err := os.ReadFile(abs)
	if err != nil {
		att.Error = fmt.Sprintf("read file: %v", err)
		return att
	}

	att.Content = string(data)
	att.Language = langForExt(ext)
	att.MimeType = mimeForExt(ext)
	return att
}

// expandPath expands ~ to the user's home directory.
func expandPath(p string) string {
	if strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return p
		}
		return filepath.Join(home, p[2:])
	}
	return p
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

// mimeForExt returns a MIME type for known text extensions.
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
