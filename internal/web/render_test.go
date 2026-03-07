package web

import (
	"strings"
	"testing"
)

func TestRenderMarkdown_Basic(t *testing.T) {
	html, err := RenderMarkdown("# Hello\n\nWorld")
	if err != nil {
		t.Fatalf("RenderMarkdown() error: %v", err)
	}
	if !strings.Contains(html, "<h1>Hello</h1>") {
		t.Errorf("expected <h1>Hello</h1>, got %q", html)
	}
	if !strings.Contains(html, "<p>World</p>") {
		t.Errorf("expected <p>World</p>, got %q", html)
	}
}

func TestRenderMarkdown_GFM(t *testing.T) {
	html, err := RenderMarkdown("- [x] done\n- [ ] todo")
	if err != nil {
		t.Fatalf("RenderMarkdown() error: %v", err)
	}
	if !strings.Contains(html, "checked") {
		t.Errorf("expected GFM checkbox, got %q", html)
	}
}

func TestRenderMarkdown_CodeBlock(t *testing.T) {
	html, err := RenderMarkdown("```go\nfunc main() {}\n```")
	if err != nil {
		t.Fatalf("RenderMarkdown() error: %v", err)
	}
	if !strings.Contains(html, "<code") {
		t.Errorf("expected code block, got %q", html)
	}
}

func TestRenderMarkdown_Empty(t *testing.T) {
	html, err := RenderMarkdown("")
	if err != nil {
		t.Fatalf("RenderMarkdown() error: %v", err)
	}
	if html != "" {
		t.Errorf("expected empty string, got %q", html)
	}
}
