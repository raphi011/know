package parser

import (
	"testing"
)

func TestParseQueryBlock_ListBasic(t *testing.T) {
	block := parseQueryBlock("LIST FROM /docs LIMIT 10")
	assertNoError(t, block)
	assertEqual(t, "format", FormatList, block.Format)
	assertFolder(t, "/docs", block.Folder)
	assertEqual(t, "limit", 10, block.Limit)
	assertEqual(t, "fields", 0, len(block.Fields))
}

func TestParseQueryBlock_ListWithField(t *testing.T) {
	block := parseQueryBlock("LIST title FROM /docs")
	assertNoError(t, block)
	assertEqual(t, "format", FormatList, block.Format)
	assertEqual(t, "fields", 1, len(block.Fields))
	assertEqual(t, "field name", "title", block.Fields[0].Name)
}

func TestParseQueryBlock_ListWithoutID(t *testing.T) {
	block := parseQueryBlock("LIST WITHOUT ID title")
	assertNoError(t, block)
	assertEqual(t, "format", FormatList, block.Format)
	assertEqual(t, "withoutID", true, block.WithoutID)
	assertEqual(t, "fields", 1, len(block.Fields))
	assertEqual(t, "field name", "title", block.Fields[0].Name)
}

func TestParseQueryBlock_TableWithAliases(t *testing.T) {
	block := parseQueryBlock(`TABLE title, labels AS "Tags" FROM /`)
	assertNoError(t, block)
	assertEqual(t, "format", FormatTable, block.Format)
	assertEqual(t, "fields", 2, len(block.Fields))
	assertEqual(t, "field[0].name", "title", block.Fields[0].Name)
	assertEqual(t, "field[0].alias", "", block.Fields[0].Alias)
	assertEqual(t, "field[1].name", "labels", block.Fields[1].Name)
	assertEqual(t, "field[1].alias", "Tags", block.Fields[1].Alias)
	assertFolder(t, "/", block.Folder)
}

func TestParseQueryBlock_TableWithoutID(t *testing.T) {
	block := parseQueryBlock(`TABLE WITHOUT ID title AS "Name", labels`)
	assertNoError(t, block)
	assertEqual(t, "format", FormatTable, block.Format)
	assertEqual(t, "withoutID", true, block.WithoutID)
	assertEqual(t, "fields", 2, len(block.Fields))
	assertEqual(t, "field[0].alias", "Name", block.Fields[0].Alias)
}

func TestParseQueryBlock_Task(t *testing.T) {
	block := parseQueryBlock(`TASK WHERE status = "open" SORT due_date ASC`)
	assertNoError(t, block)
	assertEqual(t, "format", FormatTask, block.Format)
	assertEqual(t, "conditions", 1, len(block.Conditions))
	assertEqual(t, "cond field", "status", block.Conditions[0].Field)
	assertEqual(t, "cond op", OpEqual, block.Conditions[0].Op)
	assertEqual(t, "cond value", "open", block.Conditions[0].Value)
	assertEqual(t, "sort field", "due_date", block.SortField)
	assertEqual(t, "sort desc", false, block.SortDesc)
}

func TestParseQueryBlock_TaskFromFolder(t *testing.T) {
	block := parseQueryBlock("TASK FROM /daily")
	assertNoError(t, block)
	assertEqual(t, "format", FormatTask, block.Format)
	assertFolder(t, "/daily", block.Folder)
}

func TestParseQueryBlock_MultipleWhere(t *testing.T) {
	block := parseQueryBlock(`LIST WHERE labels CONTAIN "go" WHERE status = "active"`)
	assertNoError(t, block)
	assertEqual(t, "conditions", 2, len(block.Conditions))
	assertEqual(t, "cond[0].op", OpContain, block.Conditions[0].Op)
	assertEqual(t, "cond[1].op", OpEqual, block.Conditions[1].Op)
}

func TestParseQueryBlock_ContainsOperator(t *testing.T) {
	block := parseQueryBlock(`LIST WHERE title CONTAINS "setup"`)
	assertNoError(t, block)
	assertEqual(t, "conditions", 1, len(block.Conditions))
	assertEqual(t, "cond op", OpContains, block.Conditions[0].Op)
}

func TestParseQueryBlock_SortDesc(t *testing.T) {
	block := parseQueryBlock("LIST SORT updated_at DESC")
	assertNoError(t, block)
	assertEqual(t, "sort field", "updated_at", block.SortField)
	assertEqual(t, "sort desc", true, block.SortDesc)
}

func TestParseQueryBlock_SortDefaultAsc(t *testing.T) {
	block := parseQueryBlock("LIST SORT title")
	assertNoError(t, block)
	assertEqual(t, "sort field", "title", block.SortField)
	assertEqual(t, "sort desc", false, block.SortDesc)
}

func TestParseQueryBlock_Defaults(t *testing.T) {
	block := parseQueryBlock("LIST")
	assertNoError(t, block)
	assertEqual(t, "sort field", "title", block.SortField)
	assertEqual(t, "sort desc", false, block.SortDesc)
	assertEqual(t, "limit", 50, block.Limit)
	assertEqual(t, "fields", 0, len(block.Fields))
}

func TestParseQueryBlock_Errors(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"empty", ""},
		{"no format keyword", "FROM /projects"},
		{"duplicate FROM", "LIST FROM /a FROM /b"},
		{"duplicate SORT", "LIST SORT title SORT path"},
		{"duplicate LIMIT", "LIST LIMIT 5 LIMIT 10"},
		{"unexpected token", "LIST FROM /a GARBAGE"},
		{"unterminated string", `LIST WHERE title = "hello`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			block := parseQueryBlock(tt.input)
			if block.Error == "" {
				t.Errorf("expected error for input %q", tt.input)
			}
		})
	}
}

func TestParseQueryBlock_CaseInsensitive(t *testing.T) {
	block := parseQueryBlock(`list from /docs where labels contain "go" sort title desc limit 5`)
	assertNoError(t, block)
	assertEqual(t, "format", FormatList, block.Format)
	assertFolder(t, "/docs", block.Folder)
	assertEqual(t, "conditions", 1, len(block.Conditions))
	assertEqual(t, "sort desc", true, block.SortDesc)
	assertEqual(t, "limit", 5, block.Limit)
}

func TestParseQueryBlock_Multiline(t *testing.T) {
	block := parseQueryBlock("TABLE title, labels\nFROM /projects\nWHERE labels CONTAIN \"go\"\nSORT updated_at DESC\nLIMIT 10")
	assertNoError(t, block)
	assertEqual(t, "format", FormatTable, block.Format)
	assertEqual(t, "fields", 2, len(block.Fields))
	assertFolder(t, "/projects", block.Folder)
	assertEqual(t, "conditions", 1, len(block.Conditions))
	assertEqual(t, "sort field", "updated_at", block.SortField)
	assertEqual(t, "sort desc", true, block.SortDesc)
	assertEqual(t, "limit", 10, block.Limit)
}

func TestParseQueryBlock_TableNoFields(t *testing.T) {
	block := parseQueryBlock("TABLE FROM /docs")
	assertNoError(t, block)
	assertEqual(t, "format", FormatTable, block.Format)
	assertEqual(t, "fields", 0, len(block.Fields))
	assertFolder(t, "/docs", block.Folder)
}

// --- helpers ---

func assertNoError(t *testing.T, block QueryBlock) {
	t.Helper()
	if block.Error != "" {
		t.Fatalf("unexpected error: %s", block.Error)
	}
}

func assertEqual[T comparable](t *testing.T, name string, want, got T) {
	t.Helper()
	if got != want {
		t.Errorf("%s: got %v, want %v", name, got, want)
	}
}

func assertFolder(t *testing.T, want string, got *string) {
	t.Helper()
	if got == nil {
		t.Fatalf("folder: got nil, want %q", want)
	}
	if *got != want {
		t.Errorf("folder: got %q, want %q", *got, want)
	}
}
