package epub

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

func TestMarkdownToHTML(t *testing.T) {
	tests := []struct {
		name     string
		markdown string
		contains []string
	}{
		{
			name:     "heading and paragraph",
			markdown: "# Hello\n\nWorld",
			contains: []string{"<h1>Hello</h1>", "<p>World</p>"},
		},
		{
			name:     "table",
			markdown: "| A | B |\n|---|---|\n| 1 | 2 |",
			contains: []string{"<table>", "<td>1</td>", "<td>2</td>"},
		},
		{
			name:     "task list",
			markdown: "- [x] Done\n- [ ] Todo",
			contains: []string{`<input`, `checked`},
		},
		{
			name:     "strikethrough",
			markdown: "~~deleted~~",
			contains: []string{"<del>deleted</del>"},
		},
		{
			name:     "image reference",
			markdown: "![photo](/images/test.png)",
			contains: []string{`<img src="/images/test.png"`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			html, err := markdownToHTML(tt.markdown)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			for _, want := range tt.contains {
				if !strings.Contains(html, want) {
					t.Errorf("HTML missing %q\ngot: %s", want, html)
				}
			}
		})
	}
}

func TestExtractImagePaths(t *testing.T) {
	content := `# Doc

![photo](/images/photo.png)
![logo](./logo.svg)
![external](https://example.com/img.jpg)

<img src="/assets/banner.webp" alt="banner">

Regular [link](/not-an-image) should not match.
`

	paths := ExtractImagePaths(content)
	want := []string{
		"/images/photo.png",
		"./logo.svg",
		"https://example.com/img.jpg",
		"/assets/banner.webp",
	}

	if len(paths) != len(want) {
		t.Fatalf("got %d paths, want %d: %v", len(paths), len(want), paths)
	}
	for i, p := range paths {
		if p != want[i] {
			t.Errorf("path[%d] = %q, want %q", i, p, want[i])
		}
	}
}

func TestExtractImagePaths_NoDuplicates(t *testing.T) {
	content := "![a](/img.png)\n![b](/img.png)"
	paths := ExtractImagePaths(content)
	if len(paths) != 1 {
		t.Errorf("got %d paths, want 1 (deduped): %v", len(paths), paths)
	}
}

func TestRewriteImageSrc(t *testing.T) {
	html := `<p><img src="/images/photo.png" alt="photo"></p><p><img src="unknown.png" alt="x"></p>`
	pathMap := map[string]string{
		"/images/photo.png": "../images/img0001.png",
	}

	result := rewriteImageSrc(html, pathMap)

	if !strings.Contains(result, `src="../images/img0001.png"`) {
		t.Errorf("expected rewritten src, got: %s", result)
	}
	// Unknown image should remain unchanged.
	if !strings.Contains(result, `src="unknown.png"`) {
		t.Errorf("expected unknown src unchanged, got: %s", result)
	}
}

func TestRewriteImageSrc_LeadingSlashFallback(t *testing.T) {
	html := `<img src="/images/photo.png" alt="photo">`
	pathMap := map[string]string{
		"images/photo.png": "../images/img0001.png",
	}

	result := rewriteImageSrc(html, pathMap)

	if !strings.Contains(result, `src="../images/img0001.png"`) {
		t.Errorf("expected leading-slash fallback to match, got: %s", result)
	}
}

func TestGenerate_SingleChapter(t *testing.T) {
	data, err := Generate(
		Options{Title: "Test Book", Author: "Test Author"},
		[]Chapter{{Title: "Chapter 1", Content: "# Hello\n\nWorld", Path: "/ch1.md"}},
		nil,
	)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	assertValidEPUB(t, data)
}

func TestGenerate_MultipleChapters(t *testing.T) {
	chapters := []Chapter{
		{Title: "Chapter 1", Content: "First chapter", Path: "/ch1.md"},
		{Title: "Chapter 2", Content: "Second chapter", Path: "/ch2.md"},
		{Title: "Chapter 3", Content: "Third chapter", Path: "/ch3.md"},
	}

	data, err := Generate(
		Options{Title: "Multi-Chapter Book"},
		chapters,
		nil,
	)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	assertValidEPUB(t, data)
}

func TestGenerate_WithImages(t *testing.T) {
	// 1x1 red pixel PNG
	pngData := []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d,
		0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53, 0xde, 0x00, 0x00, 0x00,
		0x0c, 0x49, 0x44, 0x41, 0x54, 0x08, 0xd7, 0x63, 0xf8, 0xcf, 0xc0, 0x00,
		0x00, 0x00, 0x02, 0x00, 0x01, 0xe2, 0x21, 0xbc, 0x33, 0x00, 0x00, 0x00,
		0x00, 0x49, 0x45, 0x4e, 0x44, 0xae, 0x42, 0x60, 0x82,
	}

	chapters := []Chapter{
		{Title: "With Image", Content: "# Doc\n\n![test](/images/test.png)", Path: "/doc.md"},
	}
	images := []Image{
		{VaultPath: "/images/test.png", Filename: "test.png", Data: pngData, MimeType: "image/png"},
	}

	data, err := Generate(Options{Title: "Image Book"}, chapters, images)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	assertValidEPUB(t, data)

	// Verify image is in the ZIP.
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	found := false
	for _, f := range r.File {
		if strings.Contains(f.Name, "test.png") {
			found = true
			break
		}
	}
	if !found {
		t.Error("image test.png not found in EPUB")
	}
}

func TestGenerate_EmptyTitle(t *testing.T) {
	_, err := Generate(Options{}, nil, nil)
	if err == nil {
		t.Fatal("expected error for empty title")
	}
}

func TestGenerate_DefaultAuthor(t *testing.T) {
	data, err := Generate(
		Options{Title: "No Author"},
		[]Chapter{{Title: "Ch", Content: "text", Path: "/ch.md"}},
		nil,
	)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	assertValidEPUB(t, data)
}

func assertValidEPUB(t *testing.T, data []byte) {
	t.Helper()

	if len(data) == 0 {
		t.Fatal("EPUB data is empty")
	}

	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("not a valid ZIP: %v", err)
	}

	requiredFiles := map[string]bool{
		"META-INF/container.xml": false,
	}

	for _, f := range r.File {
		if _, ok := requiredFiles[f.Name]; ok {
			requiredFiles[f.Name] = true
		}
	}

	for name, found := range requiredFiles {
		if !found {
			t.Errorf("required EPUB file %q not found", name)
		}
	}
}
