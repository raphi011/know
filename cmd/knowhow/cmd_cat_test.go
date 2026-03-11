package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestShellQuote(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "simple", in: "simple", want: "'simple'"},
		{name: "empty", in: "", want: "''"},
		{name: "single quote", in: "it's", want: `'it'\''s'`},
		{name: "only quote", in: "'", want: `''\'''`},
		{name: "multiple quotes", in: "'''", want: `''\'''\'''\'''`},
		{name: "spaces", in: "file with spaces.md", want: "'file with spaces.md'"},
		{name: "backticks", in: "`whoami`", want: "'`whoami`'"},
		{name: "dollar expansion", in: "$(rm -rf /)", want: "'$(rm -rf /)'"},
		{name: "semicolon", in: "file;ls", want: "'file;ls'"},
		{name: "newline", in: "a\nb", want: "'a\nb'"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shellQuote(tt.in)
			if got != tt.want {
				t.Errorf("shellQuote(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestViewWithCommand(t *testing.T) {
	outFile := filepath.Join(t.TempDir(), "output")
	content := "hello world\nline two"

	// viewWithCommand runs: sh -c "VIEWER QUOTED_TMPFILE"
	// Using redirect so the viewer reads TMPFILE and writes to our outFile.
	viewer := "cat >" + shellQuote(outFile) + " <"

	err := viewWithCommand(viewer, ".md", content)
	if err != nil {
		t.Fatalf("viewWithCommand: %v", err)
	}

	got, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if string(got) != content {
		t.Errorf("got %q, want %q", string(got), content)
	}
}

func TestViewWithCommand_Error(t *testing.T) {
	err := viewWithCommand("false", ".md", "content")
	if err == nil {
		t.Fatal("expected error from failing viewer command")
	}
}
