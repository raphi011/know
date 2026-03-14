package parser

import (
	"testing"
)

func TestExtractQueryBlocks_SingleBlock(t *testing.T) {
	content := "# My Doc\n\nSome text.\n\n```know\nFROM /projects\nWHERE labels CONTAIN \"go\"\nSHOW title, labels, updated_at\nSORT updated_at DESC\nLIMIT 10\n```\n\nMore text."

	blocks := ExtractQueryBlocks(content)
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}

	b := blocks[0]
	if b.Folder == nil || *b.Folder != "/projects" {
		t.Errorf("folder = %v, want /projects", b.Folder)
	}
	if len(b.Conditions) != 1 {
		t.Fatalf("expected 1 condition, got %d", len(b.Conditions))
	}
	if b.Conditions[0].Field != "labels" || b.Conditions[0].Op != OpContain || b.Conditions[0].Value != "go" {
		t.Errorf("condition = %+v", b.Conditions[0])
	}
	if len(b.ShowFields) != 3 || b.ShowFields[0] != "title" {
		t.Errorf("show = %v", b.ShowFields)
	}
	if b.SortField != "updated_at" || b.SortDesc != true {
		t.Errorf("sort = %s desc=%v", b.SortField, b.SortDesc)
	}
	if b.Limit != 10 {
		t.Errorf("limit = %d, want 10", b.Limit)
	}
}

func TestExtractQueryBlocks_NoBlocks(t *testing.T) {
	content := "# Just markdown\n\n```go\nfmt.Println(\"hello\")\n```"
	blocks := ExtractQueryBlocks(content)
	if len(blocks) != 0 {
		t.Errorf("expected 0 blocks, got %d", len(blocks))
	}
}

func TestExtractQueryBlocks_MultipleBlocks(t *testing.T) {
	content := "```know\nFROM /a\n```\n\ntext\n\n```know\nWHERE type = \"note\"\nSHOW title, path, labels\n```"
	blocks := ExtractQueryBlocks(content)
	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(blocks))
	}
	if blocks[0].Folder == nil || *blocks[0].Folder != "/a" {
		t.Errorf("block 0 folder = %v", blocks[0].Folder)
	}
	if blocks[1].Folder != nil {
		t.Errorf("block 1 folder should be nil, got %v", blocks[1].Folder)
	}
}

func TestExtractQueryBlocks_DefaultValues(t *testing.T) {
	content := "```know\nFROM /docs\n```"
	blocks := ExtractQueryBlocks(content)
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	b := blocks[0]
	// Defaults: SHOW title,path; SORT title ASC; LIMIT 50
	if len(b.ShowFields) != 2 || b.ShowFields[0] != "title" || b.ShowFields[1] != "path" {
		t.Errorf("default show = %v, want [title path]", b.ShowFields)
	}
	if b.SortField != "title" || b.SortDesc != false {
		t.Errorf("default sort = %s desc=%v", b.SortField, b.SortDesc)
	}
	if b.Limit != 50 {
		t.Errorf("default limit = %d, want 50", b.Limit)
	}
}

func TestExtractQueryBlocks_WhereConditions(t *testing.T) {
	content := "```know\nWHERE labels CONTAIN \"go\"\nWHERE type = \"note\"\nWHERE title CONTAINS \"setup\"\n```"
	blocks := ExtractQueryBlocks(content)
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	if len(blocks[0].Conditions) != 3 {
		t.Fatalf("expected 3 conditions, got %d", len(blocks[0].Conditions))
	}
	c := blocks[0].Conditions
	if c[0].Op != OpContain {
		t.Errorf("c[0].Op = %v, want OpContain", c[0].Op)
	}
	if c[1].Op != OpEqual {
		t.Errorf("c[1].Op = %v, want OpEqual", c[1].Op)
	}
	if c[2].Op != OpContains {
		t.Errorf("c[2].Op = %v, want OpContains", c[2].Op)
	}
}

func TestExtractQueryBlocks_FormatDetection(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    QueryFormat
	}{
		{"no SHOW = list", "```know\nFROM /a\n```", FormatList},
		{"2 fields = list", "```know\nSHOW title, path\n```", FormatList},
		{"3+ fields = table", "```know\nSHOW title, path, labels\n```", FormatTable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blocks := ExtractQueryBlocks(tt.content)
			if len(blocks) != 1 {
				t.Fatalf("expected 1 block, got %d", len(blocks))
			}
			if blocks[0].Format != tt.want {
				t.Errorf("format = %v, want %v", blocks[0].Format, tt.want)
			}
		})
	}
}

func TestExtractQueryBlocks_MalformedBlock(t *testing.T) {
	content := "```know\nGARBAGE nonsense\n```"
	blocks := ExtractQueryBlocks(content)
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	if blocks[0].Error == "" {
		t.Error("expected error for malformed block")
	}
}

func TestExtractQueryBlocks_IndexTracking(t *testing.T) {
	content := "prefix\n\n```know\nFROM /a\n```"
	blocks := ExtractQueryBlocks(content)
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	// Index should be byte offset of the opening ```
	expected := len("prefix\n\n")
	if blocks[0].Index != expected {
		t.Errorf("index = %d, want %d", blocks[0].Index, expected)
	}
}
