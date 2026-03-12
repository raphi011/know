package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileList_Add_AcceptedExtensions(t *testing.T) {
	dir := t.TempDir()

	mdFile := filepath.Join(dir, "notes.md")
	os.WriteFile(mdFile, []byte("# Notes\nSome content\n"), 0o644)
	pngFile := filepath.Join(dir, "img.png")
	os.WriteFile(pngFile, []byte("fakepng"), 0o644)

	fl := &FileList{}
	if reason := fl.Add(mdFile); reason != "" {
		t.Fatalf("unexpected rejection: %s", reason)
	}
	if reason := fl.Add(pngFile); reason != "" {
		t.Fatalf("unexpected rejection: %s", reason)
	}

	if fl.Len() != 2 {
		t.Fatalf("expected 2 files, got %d", fl.Len())
	}
	if fl.files[0].Name() != "notes.md" {
		t.Errorf("first file = %s, want notes.md", fl.files[0].Name())
	}
	if fl.files[1].Name() != "img.png" {
		t.Errorf("second file = %s, want img.png", fl.files[1].Name())
	}
}

func TestFileList_Add_RejectedExtensions(t *testing.T) {
	dir := t.TempDir()

	goFile := filepath.Join(dir, "main.go")
	os.WriteFile(goFile, []byte("package main"), 0o644)
	zipFile := filepath.Join(dir, "archive.zip")
	os.WriteFile(zipFile, []byte("fakearchive"), 0o644)

	fl := &FileList{}

	if reason := fl.Add(goFile); reason == "" {
		t.Error("expected rejection for .go file")
	} else if !strings.Contains(reason, ".go") {
		t.Errorf("rejection should mention .go, got: %s", reason)
	}

	if reason := fl.Add(zipFile); reason == "" {
		t.Error("expected rejection for .zip file")
	}

	if fl.Len() != 0 {
		t.Errorf("expected 0 files (rejected extensions), got %d", fl.Len())
	}
}

func TestFileList_Add_Duplicate(t *testing.T) {
	dir := t.TempDir()
	mdFile := filepath.Join(dir, "notes.md")
	os.WriteFile(mdFile, []byte("# Notes\n"), 0o644)

	fl := &FileList{}
	fl.Add(mdFile)
	fl.Add(mdFile)

	if fl.Len() != 1 {
		t.Errorf("expected 1 file (duplicate ignored), got %d", fl.Len())
	}
}

func TestFileList_Add_WhitespacePadding(t *testing.T) {
	dir := t.TempDir()
	mdFile := filepath.Join(dir, "notes.md")
	os.WriteFile(mdFile, []byte("# Notes\n"), 0o644)

	fl := &FileList{}
	fl.Add("  " + mdFile + "  ")

	if fl.Len() != 1 {
		t.Fatalf("expected 1 file after trimmed add, got %d", fl.Len())
	}
}

func TestFileList_Remove(t *testing.T) {
	fl := &FileList{
		files: []Attachment{
			{Path: "a.md", AbsPath: "/a.md"},
			{Path: "b.md", AbsPath: "/b.md"},
			{Path: "c.md", AbsPath: "/c.md"},
		},
		selected: 2,
	}

	fl.Remove(1)

	if fl.Len() != 2 {
		t.Fatalf("expected 2 files after removal, got %d", fl.Len())
	}
	if fl.files[1].Path != "c.md" {
		t.Errorf("second file should be c.md, got %s", fl.files[1].Path)
	}
	// Selected was 2, now there are only 2 items (0,1), so selected should clamp to 1
	if fl.selected != 1 {
		t.Errorf("selected = %d, want 1", fl.selected)
	}
}

func TestFileList_Remove_LastItem(t *testing.T) {
	fl := &FileList{
		files:    []Attachment{{Path: "a.md", AbsPath: "/a.md"}},
		selected: 0,
	}

	fl.Remove(0)

	if fl.Len() != 0 {
		t.Errorf("expected 0 files, got %d", fl.Len())
	}
	if fl.Focused() {
		t.Error("should not be focused after removing last item")
	}
}

func TestFileList_Remove_OutOfBounds(t *testing.T) {
	fl := &FileList{
		files:    []Attachment{{Path: "a.md", AbsPath: "/a.md"}},
		selected: 0,
	}

	fl.Remove(-1)
	fl.Remove(5)

	if fl.Len() != 1 {
		t.Errorf("out-of-bounds Remove should be no-op, got %d files", fl.Len())
	}
}

func TestFileList_RemoveSelected(t *testing.T) {
	fl := &FileList{
		files: []Attachment{
			{Path: "a.md", AbsPath: "/a.md"},
			{Path: "b.md", AbsPath: "/b.md"},
		},
		selected: 1,
	}

	fl.RemoveSelected()

	if fl.Len() != 1 {
		t.Fatalf("expected 1 file after RemoveSelected, got %d", fl.Len())
	}
	if fl.files[0].Path != "a.md" {
		t.Errorf("remaining file should be a.md, got %s", fl.files[0].Path)
	}
}

func TestFileList_RemoveSelected_NotFocused(t *testing.T) {
	fl := &FileList{
		files:    []Attachment{{Path: "a.md", AbsPath: "/a.md"}},
		selected: -1,
	}

	fl.RemoveSelected()

	if fl.Len() != 1 {
		t.Error("RemoveSelected when not focused should be no-op")
	}
}

func TestFileList_Clear(t *testing.T) {
	fl := &FileList{
		files: []Attachment{
			{Path: "a.md", AbsPath: "/a.md"},
			{Path: "b.md", AbsPath: "/b.md"},
		},
		selected: 1,
	}

	files := fl.Clear()
	if len(files) != 2 {
		t.Fatalf("Clear should return 2 files, got %d", len(files))
	}
	if fl.Len() != 0 {
		t.Error("list should be empty after Clear")
	}
	if fl.Focused() {
		t.Error("should not be focused after Clear")
	}
}

func TestFileList_Navigation(t *testing.T) {
	fl := &FileList{
		files: []Attachment{
			{Path: "a.md", AbsPath: "/a.md"},
			{Path: "b.md", AbsPath: "/b.md"},
			{Path: "c.md", AbsPath: "/c.md"},
		},
	}

	fl.Focus()
	if fl.selected != 2 {
		t.Errorf("Focus should select last item (2), got %d", fl.selected)
	}

	// Up from last
	fl.Up()
	if fl.selected != 1 {
		t.Errorf("Up: selected = %d, want 1", fl.selected)
	}

	// Up again
	fl.Up()
	if fl.selected != 0 {
		t.Errorf("Up: selected = %d, want 0", fl.selected)
	}

	// Up at top — should clamp
	fl.Up()
	if fl.selected != 0 {
		t.Errorf("Up at top: selected = %d, want 0 (clamped)", fl.selected)
	}

	// Down
	fl.Down()
	if fl.selected != 1 {
		t.Errorf("Down: selected = %d, want 1", fl.selected)
	}

	// Down to last
	fl.Down()
	if !fl.AtBottom() {
		t.Error("should be at bottom")
	}

	// Down at bottom — should clamp
	fl.Down()
	if fl.selected != 2 {
		t.Errorf("Down at bottom: selected = %d, want 2 (clamped)", fl.selected)
	}
}

func TestFileList_Navigation_NotFocused(t *testing.T) {
	fl := &FileList{
		files: []Attachment{
			{Path: "a.md", AbsPath: "/a.md"},
			{Path: "b.md", AbsPath: "/b.md"},
		},
		selected: -1,
	}

	fl.Up()
	fl.Down()

	if fl.Focused() {
		t.Error("Up/Down on blurred list should not change focus state")
	}
}

func TestFileList_AtBottom_Empty(t *testing.T) {
	fl := &FileList{}
	if fl.AtBottom() {
		t.Error("AtBottom should be false for empty list")
	}
}

func TestFileList_AtBottom_NotFocused(t *testing.T) {
	fl := &FileList{
		files:    []Attachment{{Path: "a.md", AbsPath: "/a.md"}},
		selected: -1,
	}
	if fl.AtBottom() {
		t.Error("AtBottom should be false when not focused")
	}
}

func TestFileList_View(t *testing.T) {
	fl := &FileList{
		files: []Attachment{
			{Path: "notes.md", AbsPath: "/notes.md", Type: FileTypeText, Content: "line1\nline2\nline3\n"},
			{Path: "img.png", AbsPath: "/img.png", Type: FileTypeImage, Size: 1536},
		},
		selected: 0,
	}

	view := fl.View(60)
	if view == "" {
		t.Fatal("view should not be empty")
	}
	if !strings.Contains(view, "notes.md") {
		t.Error("view should contain notes.md")
	}
	if !strings.Contains(view, "img.png") {
		t.Error("view should contain img.png")
	}
	if !strings.Contains(view, "[x]") {
		t.Error("view should contain [x] markers")
	}
}

func TestFileList_View_Empty(t *testing.T) {
	fl := &FileList{}
	if fl.View(80) != "" {
		t.Error("empty file list should return empty view")
	}
}

func TestFileList_FocusEmpty(t *testing.T) {
	fl := &FileList{}
	fl.Focus()
	if fl.Focused() {
		t.Error("should not be able to focus empty list")
	}
}
