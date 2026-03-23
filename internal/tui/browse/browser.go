// Package browse provides a bubbletea v2 TUI for fuzzy-finding and viewing
// vault documents.
package browse

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/glamour"
	"github.com/raphi011/know/internal/apiclient"
	"github.com/raphi011/know/internal/models"
	"github.com/raphi011/know/internal/record"
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

// audioReadyMsg is the result of an async audio asset fetch.
type audioReadyMsg struct {
	path       string
	player     *record.Player
	transcript string
	err        error
}

// Model is the root browser model composing a search, finder, links list, bookmarks, and viewer.
type Model struct {
	state     viewState
	activeTab Tab
	search    searchModel
	finder    finderModel
	links     linksModel
	bookmarks bookmarksModel
	viewer    viewerModel
	client    *apiclient.Client
	vaultID   string
	noFinder  bool // true when opened with a direct path (skip finder on Escape)
	width     int
	height    int

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
// startTab selects which tab to show initially.
func NewModel(client *apiclient.Client, vaultID string, files []models.FileEntry, isDark bool, startTab ...Tab) Model {
	glamourStyle, r := initGlamour(isDark)
	initial := TabSearch
	if len(startTab) > 0 {
		initial = startTab[0]
	}
	return Model{
		state:        stateFinding,
		activeTab:    initial,
		search:       newSearchModel(client, vaultID),
		finder:       newFinder(files),
		links:        newLinksModel(client, vaultID),
		bookmarks:    newBookmarksModel(client, vaultID),
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
		viewer:       newViewer(doc.Path, doc.Content, renderContent(r, doc), 80, 24),
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
	if m.noFinder {
		return nil
	}
	cmds := []tea.Cmd{m.links.loadLinks(), m.bookmarks.loadBookmarks()}
	// Focus the input for the initial tab.
	switch m.activeTab {
	case TabSearch:
		cmds = append(cmds, m.search.input.Focus())
	case TabAllFiles:
		cmds = append(cmds, m.finder.Init())
	case TabLinks:
		cmds = append(cmds, m.links.input.Focus())
	}
	return tea.Batch(cmds...)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		contentHeight := msg.Height - 1 // reserve 1 line for tab bar
		m.search.width = msg.Width
		m.search.height = contentHeight
		m.search.input.SetWidth(msg.Width - len(m.search.input.Prompt))
		m.finder.picker.SetSize(msg.Width, contentHeight)
		m.links.width = msg.Width
		m.links.height = contentHeight
		m.links.input.SetWidth(msg.Width - len(m.links.input.Prompt))
		m.bookmarks.width = msg.Width
		m.bookmarks.height = contentHeight
		m.updateRenderer()
		if m.state == stateViewing {
			m.viewer.width = msg.Width
			m.viewer.height = msg.Height
			if m.viewer.audioPlayer != nil {
				// Audio player handles its own resize via WindowSizeMsg
			} else {
				m.viewer.viewport.SetWidth(msg.Width)
				m.viewer.viewport.SetHeight(max(msg.Height-2, 1))
			}
		}
		return m, nil

	case tea.KeyPressMsg:
		// Global quit (ctrl+c works in both states)
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}

		// Tab switching with Tab/Shift+Tab (only in finding state, and only when
		// the links model isn't in a confirmation mode)
		if m.state == stateFinding && !m.links.confirmDelete {
			var newTab Tab = -1
			switch msg.String() {
			case "tab":
				newTab = (m.activeTab + 1) % tabCount
			case "shift+tab":
				newTab = (m.activeTab + tabCount - 1) % tabCount
			}
			if newTab >= 0 && newTab != m.activeTab {
				m.activeTab = newTab
				return m, m.focusActiveTab()
			} else if newTab >= 0 {
				return m, nil
			}
		}

	case fileSelectedMsg:
		return m, m.fetchDocument(msg.path)

	case documentFetchedMsg:
		if msg.err != nil {
			slog.Warn("document fetch failed", "error", msg.err)
			errMsg := fmt.Sprintf("Failed to open: %v", msg.err)
			switch m.activeTab {
			case TabSearch:
				m.search.statusErr = errMsg
			case TabAllFiles:
				m.finder.statusErr = errMsg
			case TabLinks:
				m.links.statusErr = errMsg
			case TabBookmarks:
				m.bookmarks.statusErr = errMsg
			}
			return m, nil
		}
		m.search.statusErr = ""
		m.finder.statusErr = ""
		m.links.statusErr = ""
		m.bookmarks.statusErr = ""

		// Audio files: fetch binary asset for playback.
		if strings.HasPrefix(msg.doc.MimeType, "audio/") && strings.HasSuffix(msg.doc.Path, ".wav") {
			return m, m.fetchAudio(msg.doc)
		}

		m.viewer = newViewer(msg.doc.Path, msg.doc.Content, renderContent(m.renderer, msg.doc), m.width, m.height)
		// Initialize bookmark state from loaded bookmarks.
		for _, bm := range m.bookmarks.items {
			if bm.Path == msg.doc.Path {
				m.viewer.bookmarked = true
				break
			}
		}
		m.state = stateViewing
		return m, nil

	case audioReadyMsg:
		if msg.err != nil {
			slog.Warn("audio player init failed", "error", msg.err)
			// Fall back to text viewer if we have a transcript.
			if msg.transcript != "" {
				content := "**Audio playback unavailable:** " + msg.err.Error() + "\n\n---\n\n" + msg.transcript
				doc := &apiclient.Document{Content: content}
				m.viewer = newViewer(msg.path, content, renderContent(m.renderer, doc), m.width, m.height)
				m.state = stateViewing
				return m, nil
			}
			if m.activeTab == TabAllFiles {
				m.finder.statusErr = fmt.Sprintf("Audio playback failed: %v", msg.err)
			} else {
				m.links.statusErr = fmt.Sprintf("Audio playback failed: %v", msg.err)
			}
			return m, nil
		}
		transcript := ""
		if msg.transcript != "" {
			transcript = renderContent(m.renderer, &apiclient.Document{Content: msg.transcript})
		}
		m.viewer = newAudioViewer(msg.path, msg.player, transcript, m.width, m.height)
		m.state = stateViewing
		return m, m.viewer.audioPlayer.tick()

	case backToFinderMsg:
		if m.noFinder {
			return m, tea.Quit
		}
		m.state = stateFinding
		return m, m.focusActiveTab()

	case bookmarkToggledMsg:
		var cmd tea.Cmd
		m.bookmarks, cmd = m.bookmarks.Update(msg)
		if msg.err == nil && msg.added {
			m.viewer.bookmarked = true
		} else if msg.err == nil && !msg.added {
			m.viewer.bookmarked = false
		}
		return m, cmd

	case editorFinishedMsg:
		if msg.err != nil {
			slog.Warn("editor failed", "path", msg.path, "error", msg.err)
			m.viewer.statusMsg = fmt.Sprintf("Edit failed: %v", msg.err)
			return m, nil
		}
		if !msg.changed {
			m.viewer.statusMsg = "No changes"
			return m, nil
		}
		// Re-render with updated content.
		m.viewer = newViewer(msg.path, msg.content, renderContent(m.renderer, &apiclient.Document{Content: msg.content}), m.width, m.height)
		m.viewer.statusMsg = "Saved"
		// Restore bookmark state.
		for _, bm := range m.bookmarks.items {
			if bm.Path == msg.path {
				m.viewer.bookmarked = true
				break
			}
		}
		return m, nil
	}

	// Async messages must always reach their model regardless of active tab.
	switch msg.(type) {
	case searchResultsMsg, searchDebounceMsg:
		var cmd tea.Cmd
		m.search, cmd = m.search.Update(msg)
		return m, cmd
	case linksLoadedMsg, linkArchivedMsg, linkDeletedMsg:
		var cmd tea.Cmd
		m.links, cmd = m.links.Update(msg)
		return m, cmd
	case bookmarksLoadedMsg:
		var cmd tea.Cmd
		m.bookmarks, cmd = m.bookmarks.Update(msg)
		return m, cmd
	}

	// Route to active sub-model
	switch m.state {
	case stateFinding:
		switch m.activeTab {
		case TabSearch:
			var cmd tea.Cmd
			m.search, cmd = m.search.Update(msg)
			return m, cmd
		case TabAllFiles:
			var cmd tea.Cmd
			m.finder, cmd = m.finder.Update(msg)
			return m, cmd
		case TabLinks:
			var cmd tea.Cmd
			m.links, cmd = m.links.Update(msg)
			return m, cmd
		case TabBookmarks:
			var cmd tea.Cmd
			m.bookmarks, cmd = m.bookmarks.Update(msg)
			return m, cmd
		}
	case stateViewing:
		if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
			switch keyMsg.String() {
			case "b":
				return m, m.bookmarks.toggleBookmark(m.viewer.path, !m.viewer.bookmarked)
			case "e":
				if m.viewer.audioPlayer != nil || !models.IsTextFile(m.viewer.path) {
					break
				}
				return m, m.editDocument()
			}
		}
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
		tabBar := renderTabs(m.activeTab, len(m.links.allLinks), len(m.bookmarks.items))
		switch m.activeTab {
		case TabSearch:
			content = tabBar + "\n" + m.search.View()
		case TabAllFiles:
			content = tabBar + "\n" + m.finder.View()
		case TabLinks:
			content = tabBar + "\n" + m.links.View()
		case TabBookmarks:
			content = tabBar + "\n" + m.bookmarks.View()
		}
	case stateViewing:
		content = m.viewer.View()
	}

	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

// editDocument opens the current document in $EDITOR.
func (m *Model) editDocument() tea.Cmd {
	return openInEditor(m.viewer.path, m.viewer.rawContent, m.client, m.vaultID)
}

// focusActiveTab blurs all inputs then focuses the active tab's input.
func (m *Model) focusActiveTab() tea.Cmd {
	m.search.input.Blur()
	m.finder.picker.Input.Blur()
	m.links.input.Blur()
	switch m.activeTab {
	case TabSearch:
		return m.search.input.Focus()
	case TabAllFiles:
		return m.finder.picker.Input.Focus()
	case TabLinks:
		return m.links.input.Focus()
	default:
		return nil
	}
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

func (m Model) fetchAudio(doc *apiclient.Document) tea.Cmd {
	client := m.client
	vaultID := m.vaultID
	return func() tea.Msg {
		rc, err := client.GetAssetReader(context.Background(), vaultID, doc.Path)
		if err != nil {
			// Asset not available (e.g. after transcription replaced the binary).
			// Fall back to showing the text content.
			return audioReadyMsg{path: doc.Path, transcript: doc.Content, err: fmt.Errorf("download audio: %w", err)}
		}
		player, err := record.NewPlayer(rc)
		if err != nil {
			return audioReadyMsg{path: doc.Path, transcript: doc.Content, err: fmt.Errorf("init player: %w", err)}
		}
		return audioReadyMsg{
			path:       doc.Path,
			player:     player,
			transcript: doc.Content,
		}
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
		slog.Warn("glamour renderer re-creation failed on resize", "error", err)
	} else {
		m.renderer = r
	}
}
