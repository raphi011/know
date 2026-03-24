package browse

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/raphi011/know/internal/apiclient"
	"github.com/raphi011/know/internal/models"
	"github.com/raphi011/know/internal/tui/pick"
)

type tagViewState int

const (
	tagStateList tagViewState = iota
	tagStateFiles
)

// tagsLoadedMsg is sent when the tag list has been fetched.
type tagsLoadedMsg struct {
	labels []models.LabelCount
	err    error
}

// tagFilesLoadedMsg is sent when files for a selected tag have been fetched.
type tagFilesLoadedMsg struct {
	files []models.FileEntry
	err   error
}

type tagsModel struct {
	state       tagViewState
	tagPicker   pick.Model
	filePicker  pick.Model
	tags        []models.LabelCount
	selectedTag string
	loaded      bool
	statusErr   string
	client      *apiclient.Client
	vaultID     string
}

func newTagsModel(client *apiclient.Client, vaultID string) tagsModel {
	return tagsModel{
		state:      tagStateList,
		tagPicker:  pick.NewModel(nil),
		filePicker: pick.NewModel(nil),
		client:     client,
		vaultID:    vaultID,
	}
}

func (t tagsModel) loadTags() tea.Cmd {
	client := t.client
	vaultID := t.vaultID
	return func() tea.Msg {
		labels, err := client.ListLabelsWithCounts(context.Background(), vaultID)
		if err != nil {
			return tagsLoadedMsg{err: err}
		}
		return tagsLoadedMsg{labels: labels}
	}
}

func (t tagsModel) loadFilesForTag(tag string) tea.Cmd {
	client := t.client
	vaultID := t.vaultID
	return func() tea.Msg {
		files, err := client.ListFilesByLabels(context.Background(), vaultID, []string{tag})
		if err != nil {
			return tagFilesLoadedMsg{err: err}
		}
		return tagFilesLoadedMsg{files: files}
	}
}

// labelsToEntries converts label counts to FileEntry items so we can reuse pick.Model.
func labelsToEntries(labels []models.LabelCount) []models.FileEntry {
	entries := make([]models.FileEntry, len(labels))
	for i, lc := range labels {
		entries[i] = models.FileEntry{
			Path:  lc.Label,
			Title: fmt.Sprintf("(%d)", lc.Count),
		}
	}
	return entries
}

// activePicker returns a pointer to whichever picker is currently active.
func (t *tagsModel) activePicker() *pick.Model {
	if t.state == tagStateFiles {
		return &t.filePicker
	}
	return &t.tagPicker
}

// delegateToPicker forwards a message to the active picker and clears statusErr on input change.
func (t *tagsModel) delegateToPicker(msg tea.Msg) tea.Cmd {
	p := t.activePicker()
	prev := p.Input.Value()
	updated, cmd := p.Update(msg)
	*p = updated.(pick.Model)
	if p.Input.Value() != prev {
		t.statusErr = ""
	}
	return cmd
}

func (t tagsModel) Update(msg tea.Msg) (tagsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tagsLoadedMsg:
		t.loaded = true
		if msg.err != nil {
			t.statusErr = fmt.Sprintf("Failed to load tags: %v", msg.err)
			return t, nil
		}
		t.tags = msg.labels
		w, h := t.tagPicker.Width, t.tagPicker.Height
		t.tagPicker = pick.NewModel(labelsToEntries(msg.labels))
		t.tagPicker.SetSize(w, h)
		t.statusErr = ""
		return t, t.tagPicker.Input.Focus()

	case tagFilesLoadedMsg:
		if msg.err != nil {
			t.statusErr = fmt.Sprintf("Failed to load files: %v", msg.err)
			t.state = tagStateList
			return t, t.tagPicker.Input.Focus()
		}
		w, h := t.tagPicker.Width, t.tagPicker.Height
		t.filePicker = pick.NewModel(msg.files)
		t.filePicker.SetSize(w, h)
		t.state = tagStateFiles
		t.statusErr = ""
		return t, t.filePicker.Input.Focus()

	case tea.KeyPressMsg:
		switch t.state {
		case tagStateList:
			return t.updateTagList(msg)
		case tagStateFiles:
			return t.updateFileList(msg)
		}
	}

	// Delegate non-key messages to the active picker.
	cmd := t.delegateToPicker(msg)
	return t, cmd
}

func (t tagsModel) updateTagList(msg tea.KeyPressMsg) (tagsModel, tea.Cmd) {
	switch msg.String() {
	case "enter":
		if len(t.tagPicker.Matches) > 0 && t.tagPicker.Cursor < len(t.tagPicker.Matches) {
			idx := t.tagPicker.Matches[t.tagPicker.Cursor].Index
			t.selectedTag = t.tagPicker.AllFiles[idx].Path
			t.tagPicker.Input.Blur()
			return t, t.loadFilesForTag(t.selectedTag)
		}
		return t, nil
	case "esc":
		return t, tea.Quit
	}

	cmd := t.delegateToPicker(msg)
	return t, cmd
}

func (t tagsModel) updateFileList(msg tea.KeyPressMsg) (tagsModel, tea.Cmd) {
	switch msg.String() {
	case "enter":
		if len(t.filePicker.Matches) > 0 && t.filePicker.Cursor < len(t.filePicker.Matches) {
			idx := t.filePicker.Matches[t.filePicker.Cursor].Index
			return t, func() tea.Msg {
				return fileSelectedMsg{path: t.filePicker.AllFiles[idx].Path}
			}
		}
		return t, nil
	case "esc":
		t.state = tagStateList
		t.filePicker.Input.Blur()
		t.statusErr = ""
		return t, t.tagPicker.Input.Focus()
	}

	cmd := t.delegateToPicker(msg)
	return t, cmd
}

func (t tagsModel) View() string {
	if !t.loaded {
		return "  Loading tags..."
	}

	switch t.state {
	case tagStateList:
		return t.viewTagList()
	case tagStateFiles:
		return t.viewFileList()
	}
	return ""
}

// renderPickerItems renders the shared item list, padding, and status/footer for a picker.
func renderPickerItems(b *strings.Builder, p *pick.Model, statusErr, footer string) {
	visible := p.VisibleRows()
	end := min(p.Offset+visible, len(p.Matches))

	for i := p.Offset; i < end; i++ {
		m := p.Matches[i]
		entry := p.AllFiles[m.Index]

		prefix := "  "
		style := pick.NormalStyle
		if i == p.Cursor {
			prefix = "> "
			style = pick.SelectedStyle
		}

		line := pick.RenderHighlighted(entry.Path, m.MatchedIndexes, style)
		if entry.Title != "" {
			line += " " + pick.TitleStyle.Render(entry.Title)
		}

		b.WriteString(prefix + line + "\n")
	}

	rendered := end - p.Offset
	for i := rendered; i < visible; i++ {
		b.WriteString("\n")
	}

	if statusErr != "" {
		b.WriteString(errStyle.Render("  " + statusErr))
		b.WriteString("\n")
	}

	b.WriteString(pick.CountStyle.Render(footer))
}

func (t tagsModel) viewTagList() string {
	var b strings.Builder

	b.WriteString(t.tagPicker.Input.View())
	b.WriteString("\n")
	b.WriteString(pick.CountStyle.Render(fmt.Sprintf("  %d/%d tags", len(t.tagPicker.Matches), len(t.tagPicker.AllFiles))))
	b.WriteString("\n")

	if len(t.tagPicker.AllFiles) == 0 {
		if t.statusErr != "" {
			b.WriteString(errStyle.Render("  " + t.statusErr))
		} else {
			b.WriteString("\n  No tags yet.")
		}
		return b.String()
	}

	renderPickerItems(&b, &t.tagPicker, t.statusErr, "  enter: show files  esc: quit")
	return b.String()
}

func (t tagsModel) viewFileList() string {
	var b strings.Builder

	header := fmt.Sprintf("  %s — %d files", t.selectedTag, len(t.filePicker.AllFiles))
	b.WriteString(lipgloss.NewStyle().Foreground(pick.PrimaryColor).Bold(true).Render(header))
	b.WriteString("\n")
	b.WriteString(t.filePicker.Input.View())
	b.WriteString("\n")
	b.WriteString(pick.CountStyle.Render(fmt.Sprintf("  %d/%d files", len(t.filePicker.Matches), len(t.filePicker.AllFiles))))
	b.WriteString("\n")

	renderPickerItems(&b, &t.filePicker, t.statusErr, "  enter: open  esc: back to tags")
	return b.String()
}
