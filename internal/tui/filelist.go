package tui

import (
	"fmt"
	"path/filepath"
	"strings"
)

// acceptedExtensions lists file types that can be attached via drag-and-drop.
// This gate applies only to the drag-and-drop entry point; resolveOne() itself
// accepts all text/image types from classifyFile().
var acceptedExtensions = map[string]bool{
	".md": true, ".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true,
}

// FileList manages a list of file attachments above the text input.
// Files are added via drag-and-drop (paste) and can be removed with
// keyboard navigation.
//
// Focus state is represented by the selected field alone:
// selected >= 0 means focused at that index, -1 means not focused.
type FileList struct {
	files    []Attachment
	selected int // >= 0: focused at index, -1: not focused
}

// Add resolves a file path and appends it to the list.
// Returns a user-visible rejection reason, or "" on success.
// Duplicate paths are silently ignored (returns "").
func (fl *FileList) Add(path string) string {
	path = strings.TrimSpace(path)

	ext := strings.ToLower(filepath.Ext(path))
	if !acceptedExtensions[ext] {
		return fmt.Sprintf("unsupported file type %s (accepted: .md, .png, .jpg, .jpeg, .gif, .webp)", ext)
	}

	att := resolveOne(path)

	// Ignore duplicates (only when AbsPath was successfully resolved)
	if att.AbsPath != "" {
		for _, existing := range fl.files {
			if existing.AbsPath == att.AbsPath {
				return ""
			}
		}
	}

	fl.files = append(fl.files, att)
	return ""
}

// Remove deletes the file at the given index and adjusts selection.
func (fl *FileList) Remove(index int) {
	if index < 0 || index >= len(fl.files) {
		return
	}
	fl.files = append(fl.files[:index], fl.files[index+1:]...)

	if len(fl.files) == 0 {
		fl.selected = -1
		return
	}
	if fl.selected >= len(fl.files) {
		fl.selected = len(fl.files) - 1
	}
}

// RemoveSelected deletes the currently selected file.
// No-op if the list is not focused.
func (fl *FileList) RemoveSelected() {
	if fl.selected < 0 {
		return
	}
	fl.Remove(fl.selected)
}

// Clear returns all files and empties the list.
func (fl *FileList) Clear() []Attachment {
	files := fl.files
	fl.files = nil
	fl.selected = -1
	return files
}

// Focus activates keyboard navigation on the file list,
// selecting the last item (closest to the input).
func (fl *FileList) Focus() {
	if len(fl.files) == 0 {
		return
	}
	fl.selected = len(fl.files) - 1
}

// Blur deactivates keyboard navigation.
func (fl *FileList) Blur() {
	fl.selected = -1
}

// Focused returns whether the file list has keyboard focus.
func (fl *FileList) Focused() bool { return fl.selected >= 0 && len(fl.files) > 0 }

// Len returns the number of attached files.
func (fl *FileList) Len() int { return len(fl.files) }

// AtBottom returns true if the selection is on the last item.
func (fl *FileList) AtBottom() bool {
	return fl.selected >= 0 && fl.selected == len(fl.files)-1
}

// Up moves selection towards the first item (clamped).
func (fl *FileList) Up() {
	if fl.selected <= 0 {
		return
	}
	fl.selected--
}

// Down moves selection towards the last item (clamped).
func (fl *FileList) Down() {
	if fl.selected < 0 || fl.selected >= len(fl.files)-1 {
		return
	}
	fl.selected++
}

// View renders the file list. Returns empty string when no files are attached.
func (fl *FileList) View(width int) string {
	if len(fl.files) == 0 {
		return ""
	}

	var sb strings.Builder
	for i, att := range fl.files {
		label := formatFileLabel(att)
		marker := "[x]"

		// Right-align the [x] marker
		// 6 = PaddingLeft(2) + len("[x]") + 1 space
		padding := max(width-6-len([]rune(label))-len(marker), 1)
		line := label + strings.Repeat(" ", padding) + marker

		if fl.selected >= 0 && i == fl.selected {
			sb.WriteString(fileListSelectedStyle.Render(line))
		} else {
			sb.WriteString(fileListItemStyle.Render(line))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// formatFileLabel returns a display string like "notes.md (42 lines)" or "screenshot.png (1.2 MB)".
func formatFileLabel(att Attachment) string {
	name := att.Name()
	if att.Error != "" {
		return fmt.Sprintf("%s (error: %s)", name, att.Error)
	}
	if att.Type == FileTypeImage {
		return fmt.Sprintf("%s (%s)", name, formatSize(att.Size))
	}
	return fmt.Sprintf("%s (%d lines)", name, att.LineCount())
}
