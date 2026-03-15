package parser

import (
	"maps"

	"github.com/yuin/goldmark"
	meta "github.com/yuin/goldmark-meta"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"go.abhg.dev/goldmark/wikilink"
)

// mdParser is the shared goldmark parser with all extensions enabled.
// Using a single instance avoids repeated allocation and ensures consistent behavior.
var mdParser goldmark.Markdown

func init() {
	mdParser = goldmark.New(
		goldmark.WithExtensions(
			extension.TaskList,
			&wikilink.Extender{},
			meta.Meta,
		),
	)
}

// parseAST parses markdown content into a goldmark AST using the shared parser.
// Returns the AST root, source bytes, and frontmatter metadata extracted by goldmark-meta.
func parseAST(content string) (ast.Node, []byte, map[string]any) {
	source := []byte(content)
	reader := text.NewReader(source)
	pc := parser.NewContext()
	doc := mdParser.Parser().Parse(reader, parser.WithContext(pc))

	var fm map[string]any
	if raw := meta.Get(pc); raw != nil {
		fm = make(map[string]any, len(raw))
		maps.Copy(fm, raw)
	}

	return doc, source, fm
}
