// Package browse provides a bubbletea v2 TUI for fuzzy-finding and viewing
// vault documents.
package browse

import (
	"context"
	"fmt"
	"log/slog"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/glamour"
	"github.com/raphi011/know/internal/apiclient"
	"github.com/raphi011/know/internal/models"
)

type viewState int

const (
	stateFinding viewState = iota
	stateViewing
)

// documentFetchedMsg is the result of an async document fetch.
type documentFetchedMsg struct {
	doc *apiclient.Document
	err error
}

// Model is the root browser model composing a finder and viewer.
type Model struct {
	state    viewState
	finder   finderModel
	viewer   viewerModel
	client   *apiclient.Client
	vaultID  string
	noFinder bool // true when opened with a direct path (skip finder on Escape)
	width    int
	height   int

	glamourStyle string
	renderer     *glamour.TermRenderer
}

// initGlamour creates a glamour renderer for the given terminal background.
func initGlamour(isDark bool) (string, *glamour.TermRenderer) {
	style := "light"
	if isDark {
		style = "dark"
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(style),
		glamour.WithWordWrap(100),
	)
	if err != nil {
		slog.Warn("glamour renderer init failed", "error", err)
	}
	return style, r
}

// NewModel creates a browser with a pre-fetched file list.
// isDark should be detected before bubbletea starts.
func NewModel(client *apiclient.Client, vaultID string, files []models.FileEntry, isDark bool) Model {
	glamourStyle, r := initGlamour(isDark)
	return Model{
		state:        stateFinding,
		finder:       newFinder(files),
		client:       client,
		vaultID:      vaultID,
		glamourStyle: glamourStyle,
		renderer:     r,
	}
}

// NewModelWithDocument creates a browser that starts directly in the viewer
// with the given document, skipping the fuzzy finder.
func NewModelWithDocument(doc *apiclient.Document, client *apiclient.Client, vaultID string, isDark bool) Model {
	glamourStyle, r := initGlamour(isDark)
	return Model{
		state:        stateViewing,
		client:       client,
		vaultID:      vaultID,
		noFinder:     true,
		glamourStyle: glamourStyle,
		renderer:     r,
		viewer:       newViewer(doc.Path, renderContent(r, doc), 80, 24),
	}
}

// renderContent renders document content with glamour, falling back to raw content.
func renderContent(r *glamour.TermRenderer, doc *apiclient.Document) string {
	if r == nil {
		return doc.Content
	}
	rendered, err := r.Render(doc.Content)
	if err != nil {
		slog.Warn("glamour render failed, showing raw content", "path", doc.Path, "error", err)
		return doc.Content
	}
	return rendered
}

func (m Model) Init() tea.Cmd {
	return m.finder.Init()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.finder.picker.SetSize(msg.Width, msg.Height)
		m.updateRenderer()
		if m.state == stateViewing {
			m.viewer.width = msg.Width
			m.viewer.viewport.SetWidth(msg.Width)
			m.viewer.viewport.SetHeight(max(msg.Height-2, 1))
		}
		return m, nil

	case tea.KeyPressMsg:
		// Global quit (ctrl+c works in both states)
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}

	case fileSelectedMsg:
		return m, m.fetchDocument(msg.path)

	case documentFetchedMsg:
		if msg.err != nil {
			slog.Warn("document fetch failed", "error", msg.err)
			m.finder.statusErr = fmt.Sprintf("Failed to open: %v", msg.err)
			return m, nil
		}
		m.finder.statusErr = ""
		m.viewer = newViewer(msg.doc.Path, renderContent(m.renderer, msg.doc), m.width, m.height)
		m.state = stateViewing
		return m, nil

	case backToFinderMsg:
		if m.noFinder {
			return m, tea.Quit
		}
		m.state = stateFinding
		return m, m.finder.picker.Input.Focus()
	}

	// Route to active sub-model
	switch m.state {
	case stateFinding:
		var cmd tea.Cmd
		m.finder, cmd = m.finder.Update(msg)
		return m, cmd
	case stateViewing:
		var cmd tea.Cmd
		m.viewer, cmd = m.viewer.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m Model) View() tea.View {
	var content string

	switch m.state {
	case stateFinding:
		content = m.finder.View()
	case stateViewing:
		content = m.viewer.View()
	}

	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

func (m *Model) fetchDocument(path string) tea.Cmd {
	client := m.client
	vaultID := m.vaultID
	return func() tea.Msg {
		doc, err := client.GetDocument(context.Background(), vaultID, path)
		if err != nil {
			return documentFetchedMsg{err: fmt.Errorf("fetch %s: %w", path, err)}
		}
		return documentFetchedMsg{doc: doc}
	}
}

// updateRenderer recreates the glamour renderer when terminal width changes.
func (m *Model) updateRenderer() {
	wrapWidth := min(max(m.width-4, 20), 100)
	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(m.glamourStyle),
		glamour.WithWordWrap(wrapWidth),
	)
	if err != nil {
		slog.Warn("glamour renderer re-creation failed on resize", "width", wrapWidth, "error", err)
	} else {
		m.renderer = r
	}
}
