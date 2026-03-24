package parser

import (
	"log/slog"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"go.abhg.dev/goldmark/frontmatter"
	"go.abhg.dev/goldmark/wikilink"
)

// mdParser is the shared goldmark parser with all extensions enabled.
// Using a single instance avoids repeated allocation and ensures consistent behavior.
var mdParser goldmark.Markdown

func init() {
	mdParser = goldmark.New(
		goldmark.WithExtensions(
			extension.TaskList,
			extension.Linkify,
			&wikilink.Extender{},
			&frontmatter.Extender{},
		),
	)
}

// parseAST parses markdown content into a goldmark AST using the shared parser.
// Returns the AST root, source bytes, and frontmatter metadata.
func parseAST(content string) (ast.Node, []byte, map[string]any) {
	source := []byte(content)
	reader := text.NewReader(source)
	pc := parser.NewContext()
	doc := mdParser.Parser().Parse(reader, parser.WithContext(pc))

	var fm map[string]any
	if data := frontmatter.Get(pc); data != nil {
		if err := data.Decode(&fm); err != nil {
			slog.Debug("frontmatter decode failed, treating as no frontmatter", "error", err)
			fm = nil
		}
	}

	return doc, source, fm
}
