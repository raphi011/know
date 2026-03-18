package epub

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	goepub "github.com/go-shiori/go-epub"
	"github.com/vincent-petithory/dataurl"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	gmhtml "github.com/yuin/goldmark/renderer/html"
)

// Chapter represents a single document to include in the EPUB.
type Chapter struct {
	Title   string // chapter/document title
	Content string // raw markdown
	Path    string // vault path (for ordering)
}

// Image is a resolved image ready to embed.
type Image struct {
	VaultPath string // original reference path (e.g. "/images/photo.png")
	Filename  string // unique filename for EPUB internal use
	Data      []byte // raw image bytes
	MimeType  string // e.g. "image/png"
}

// Options configures EPUB generation.
type Options struct {
	Title  string
	Author string
}

var md = goldmark.New(
	goldmark.WithExtensions(
		extension.Table,
		extension.TaskList,
		extension.Strikethrough,
		extension.Linkify,
	),
	goldmark.WithRendererOptions(
		gmhtml.WithUnsafe(),
	),
)

// imageRefRe matches markdown image references: ![alt](path)
var imageRefRe = regexp.MustCompile(`!\[[^\]]*\]\(([^)]+)\)`)

// htmlImgRe matches HTML img src attributes.
var htmlImgRe = regexp.MustCompile(`<img\s[^>]*src="([^"]+)"`)

// imgSrcRe matches img src attributes for rewriting.
var imgSrcRe = regexp.MustCompile(`(<img\s[^>]*?src=")([^"]+)(")`)

// Generate creates an EPUB from the given chapters and images.
func Generate(opts Options, chapters []Chapter, images []Image) ([]byte, error) {
	if opts.Title == "" {
		return nil, fmt.Errorf("title is required")
	}
	if opts.Author == "" {
		opts.Author = "Know"
	}

	e, err := goepub.NewEpub(opts.Title)
	if err != nil {
		return nil, fmt.Errorf("create epub: %w", err)
	}
	e.SetAuthor(opts.Author)

	// Add images via data URIs (no temp files needed).
	pathToInternal := make(map[string]string)
	for _, img := range images {
		uri := dataurl.New(img.Data, img.MimeType).String()
		internalPath, err := e.AddImage(uri, img.Filename)
		if err != nil {
			return nil, fmt.Errorf("add image %s: %w", img.Filename, err)
		}
		pathToInternal[img.VaultPath] = internalPath
	}

	// Add each chapter as a section.
	for _, ch := range chapters {
		html, err := markdownToHTML(ch.Content)
		if err != nil {
			return nil, fmt.Errorf("convert chapter %q: %w", ch.Title, err)
		}
		html = rewriteImageSrc(html, pathToInternal)
		if _, err := e.AddSection(html, ch.Title, "", ""); err != nil {
			return nil, fmt.Errorf("add section %q: %w", ch.Title, err)
		}
	}

	var buf bytes.Buffer
	if _, err := e.WriteTo(&buf); err != nil {
		return nil, fmt.Errorf("write epub: %w", err)
	}
	return buf.Bytes(), nil
}

// ExtractImagePaths returns all image paths referenced in the markdown content.
// This includes both markdown image syntax ![alt](path) and HTML <img src="path">.
func ExtractImagePaths(content string) []string {
	seen := make(map[string]bool)
	var paths []string

	for _, m := range imageRefRe.FindAllStringSubmatch(content, -1) {
		path := m[1]
		if !seen[path] {
			seen[path] = true
			paths = append(paths, path)
		}
	}
	for _, m := range htmlImgRe.FindAllStringSubmatch(content, -1) {
		path := m[1]
		if !seen[path] {
			seen[path] = true
			paths = append(paths, path)
		}
	}
	return paths
}

func markdownToHTML(content string) (string, error) {
	var buf bytes.Buffer
	if err := md.Convert([]byte(content), &buf); err != nil {
		return "", fmt.Errorf("goldmark convert: %w", err)
	}
	return buf.String(), nil
}

// rewriteImageSrc replaces image source paths in HTML with EPUB-internal paths.
func rewriteImageSrc(html string, pathMap map[string]string) string {
	return imgSrcRe.ReplaceAllStringFunc(html, func(match string) string {
		sub := imgSrcRe.FindStringSubmatch(match)
		if len(sub) < 4 {
			return match
		}
		src := sub[2]
		if internal, ok := pathMap[src]; ok {
			return sub[1] + internal + sub[3]
		}
		// Try without leading slash for relative paths.
		if strings.HasPrefix(src, "/") {
			if internal, ok := pathMap[src[1:]]; ok {
				return sub[1] + internal + sub[3]
			}
		}
		return match
	})
}
