package web

import (
	"bytes"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer/html"
)

var md goldmark.Markdown

func init() {
	md = goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			extension.Typographer,
		),
		goldmark.WithRendererOptions(
			html.WithUnsafe(),
		),
	)
}

// RenderMarkdown converts markdown content to HTML.
func RenderMarkdown(content string) (string, error) {
	var buf bytes.Buffer
	if err := md.Convert([]byte(content), &buf); err != nil {
		return "", err
	}
	return buf.String(), nil
}
