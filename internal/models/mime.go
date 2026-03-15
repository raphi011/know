package models

import (
	"path"
	"strings"
)

// MimeFolderType is the MIME type used for folder entries.
const MimeFolderType = "inode/directory"

// MimeMarkdown is the MIME type for markdown files.
const MimeMarkdown = "text/markdown"

// knownExtensions maps lowercase file extensions to MIME types.
var knownExtensions = map[string]string{
	// Images
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
	".svg":  "image/svg+xml",
	".webp": "image/webp",
	// Documents
	".pdf": "application/pdf",
	// Audio
	".mp3":  "audio/mpeg",
	".wav":  "audio/wav",
	".ogg":  "audio/ogg",
	".flac": "audio/flac",
	".m4a":  "audio/mp4",
	".aac":  "audio/aac",
	".opus": "audio/opus",
	".weba": "audio/webm",
	// Markdown
	".md":       "text/markdown",
	".markdown": "text/markdown",
	// Plain text
	".txt": "text/plain",
}

// IsImageFile returns true if the file has a supported image extension.
func IsImageFile(name string) bool {
	mime := MimeTypeFromExt(name)
	return strings.HasPrefix(mime, "image/")
}

// IsAudioFile returns true if the file has a supported audio extension.
func IsAudioFile(name string) bool {
	mime := MimeTypeFromExt(name)
	return strings.HasPrefix(mime, "audio/")
}

// IsPDFFile returns true if the file has a .pdf extension.
func IsPDFFile(name string) bool {
	return MimeTypeFromExt(name) == "application/pdf"
}

// IsMarkdownFile returns true if the file has a markdown extension.
func IsMarkdownFile(name string) bool {
	return MimeTypeFromExt(name) == MimeMarkdown
}

// MimeTypeFromExt returns the MIME type for a file based on its extension.
// Returns empty string for unsupported extensions.
func MimeTypeFromExt(name string) string {
	ext := strings.ToLower(path.Ext(name))
	return knownExtensions[ext]
}

// DetectMimeType returns the MIME type for a file path, falling back to
// "application/octet-stream" for unknown extensions.
func DetectMimeType(filePath string) string {
	if mime := MimeTypeFromExt(filePath); mime != "" {
		return mime
	}
	return "application/octet-stream"
}
