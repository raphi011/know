// Package browse provides a bubbletea v2 TUI for fuzzy-finding and viewing
// vault documents.
package browse

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/glamour/v2"
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
	doc  *apiclient.Document
	line int // 1-based source line to scroll to (0 = top)
	err  error
}

// editReadyMsg is sent when raw document content has been fetched for editing.
type editReadyMsg struct {
	path    string
	content string
	line    int // 1-based source line for +LINE (0 = none)
	err     error
}

// audioReadyMsg is the result of an async audio asset fetch.
type audioReadyMsg struct {
	path       string
	player     *record.Player
	transcript string
	err        error
}

// Model is the root browser model composing a search, finder, links list, bookmarks, tags, and viewer.
type Model struct {
	state     viewState
	activeTab Tab
	search    searchModel
	finder    finderModel
	links     linksModel
	bookmarks bookmarksModel
	tags      tagsModel
	tasks     tasksModel
	viewer    viewerModel
	client    *apiclient.Client
	vaultID   string
	noFinder  bool // true when opened with a direct path (skip finder on Escape)
	termReady bool // true after first WindowSizeMsg
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
	search := newSearchModel(client, vaultID)
	return Model{
		state:        stateFinding,
		activeTab:    initial,
		search:       search,
		finder:       newFinder(files),
		links:        newLinksModel(client, vaultID),
		bookmarks:    newBookmarksModel(client, vaultID),
		tags:         newTagsModel(client, vaultID),
		tasks:        newTasksModel(client, vaultID),
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
	if m.noFinder {
		return nil
	}
	// Note: Focus is deferred to the first WindowSizeMsg in Update() because
	// Init() in bubbletea v2 returns only tea.Cmd — model state changes
	// (like textarea Focus()) are discarded since Init has a value receiver.
	return tea.Batch(m.links.loadLinks(), m.bookmarks.loadBookmarks(), m.tags.loadTags(), m.tasks.loadTasks())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		contentHeight := msg.Height - 1 // reserve 1 line for tab bar
		m.search.width = msg.Width
		m.search.height = contentHeight
		m.search.filterBar.SetWidth(msg.Width)
		m.finder.picker.SetSize(msg.Width, contentHeight)
		m.links.width = msg.Width
		m.links.height = contentHeight
		m.links.filterBar.SetWidth(msg.Width)
		m.bookmarks.width = msg.Width
		m.bookmarks.height = contentHeight
		m.bookmarks.filterBar.SetWidth(msg.Width)
		m.tags.tagPicker.SetSize(msg.Width, contentHeight)
		m.tags.filePicker.SetSize(msg.Width, contentHeight)
		m.tasks.width = msg.Width
		m.tasks.height = contentHeight
		m.tasks.filterBar.SetWidth(msg.Width)
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
		// Focus the active tab's input on the first WindowSizeMsg.
		// Init() returns only tea.Cmd in bubbletea v2, so Focus() state
		// changes made there are discarded. We defer focus to here.
		if !m.termReady {
			m.termReady = true
			return m, m.focusActiveTab()
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
		slog.Debug("file selected", "path", msg.path, "line", msg.line, "tab", m.activeTab)
		return m, m.fetchDocument(msg.path, msg.line)

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
			case TabTags:
				m.tags.statusErr = errMsg
			case TabTasks:
				m.tasks.statusErr = errMsg
			}
			return m, nil
		}
		m.search.statusErr = ""
		m.finder.statusErr = ""
		m.links.statusErr = ""
		m.bookmarks.statusErr = ""
		m.tags.statusErr = ""
		m.tasks.statusErr = ""

		// Audio files: fetch binary asset for playback.
		if strings.HasPrefix(msg.doc.MimeType, "audio/") && strings.HasSuffix(msg.doc.Path, ".wav") {
			return m, m.fetchAudio(msg.doc)
		}

		slog.Debug("document fetched", "path", msg.doc.Path, "content_len", len(msg.doc.Content), "mime", msg.doc.MimeType)
		rendered := renderContent(m.renderer, msg.doc)
		m.viewer = newViewer(msg.doc.Path, rendered, m.width, m.height)
		if msg.line > 0 {
			srcLines := strings.Count(msg.doc.Content, "\n") + 1
			renderedLines := strings.Count(rendered, "\n") + 1
			targetLine := int(float64(msg.line) / float64(srcLines) * float64(renderedLines))
			targetLine = min(targetLine, max(renderedLines-1, 0))
			m.viewer.viewport.SetYOffset(targetLine)
			m.viewer.sourceLine = msg.line
		}
		// Initialize bookmark state from loaded bookmarks.
		for _, bm := range m.bookmarks.allItems {
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
				m.viewer = newViewer(msg.path, renderContent(m.renderer, doc), m.width, m.height)
				m.state = stateViewing
				return m, nil
			}
			errMsg := fmt.Sprintf("Audio playback failed: %v", msg.err)
			switch m.activeTab {
			case TabSearch:
				m.search.statusErr = errMsg
			case TabAllFiles:
				m.finder.statusErr = errMsg
			case TabLinks:
				m.links.statusErr = errMsg
			case TabBookmarks:
				m.bookmarks.statusErr = errMsg
			case TabTags:
				m.tags.statusErr = errMsg
			case TabTasks:
				m.tasks.statusErr = errMsg
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

	case editReadyMsg:
		if msg.err != nil {
			slog.Warn("edit failed", "path", msg.path, "error", msg.err)
			m.viewer.statusMsg = fmt.Sprintf("Edit failed: %v", msg.err)
			return m, nil
		}
		return m, openInEditor(msg.path, msg.content, msg.line, m.client, m.vaultID)

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
		m.viewer = newViewer(msg.path, renderContent(m.renderer, &apiclient.Document{Content: msg.content}), m.width, m.height)
		m.viewer.statusMsg = "Saved"
		// Restore bookmark state.
		for _, bm := range m.bookmarks.allItems {
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
	case tagsLoadedMsg, tagFilesLoadedMsg:
		var cmd tea.Cmd
		m.tags, cmd = m.tags.Update(msg)
		return m, cmd
	case tasksLoadedMsg, taskToggledMsg:
		var cmd tea.Cmd
		m.tasks, cmd = m.tasks.Update(msg)
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
		case TabTags:
			var cmd tea.Cmd
			m.tags, cmd = m.tags.Update(msg)
			return m, cmd
		case TabTasks:
			var cmd tea.Cmd
			m.tasks, cmd = m.tasks.Update(msg)
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
		tabBar := renderTabs(m.activeTab, len(m.links.allLinks), len(m.bookmarks.allItems), len(m.tags.tags), len(m.tasks.allTasks))
		switch m.activeTab {
		case TabSearch:
			content = tabBar + "\n" + m.search.View()
		case TabAllFiles:
			content = tabBar + "\n" + m.finder.View()
		case TabLinks:
			content = tabBar + "\n" + m.links.View()
		case TabBookmarks:
			content = tabBar + "\n" + m.bookmarks.View()
		case TabTags:
			content = tabBar + "\n" + m.tags.View()
		case TabTasks:
			content = tabBar + "\n" + m.tasks.View()
		}
	case stateViewing:
		content = m.viewer.View()
	}

	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

// editDocument fetches the raw document content (without wiki-link resolution
// or query block execution) and opens it in $EDITOR.
func (m *Model) editDocument() tea.Cmd {
	path := m.viewer.path
	line := m.viewer.sourceLine
	client := m.client
	vaultID := m.vaultID
	return func() tea.Msg {
		doc, err := client.GetRawDocument(context.Background(), vaultID, path)
		if err != nil {
			return editReadyMsg{path: path, err: fmt.Errorf("edit: %w", err)}
		}
		return editReadyMsg{path: path, content: doc.Content, line: line}
	}
}

// focusActiveTab blurs all inputs then focuses the active tab's input.
func (m *Model) focusActiveTab() tea.Cmd {
	m.search.filterBar = m.search.filterBar.Blur()
	m.finder.picker.Input.Blur()
	m.links.filterBar = m.links.filterBar.Blur()
	m.bookmarks.filterBar = m.bookmarks.filterBar.Blur()
	m.tags.tagPicker.Input.Blur()
	m.tags.filePicker.Input.Blur()
	m.tasks.filterBar = m.tasks.filterBar.Blur()
	switch m.activeTab {
	case TabSearch:
		var cmd tea.Cmd
		m.search.filterBar, cmd = m.search.filterBar.Focus()
		return cmd
	case TabAllFiles:
		return m.finder.picker.Input.Focus()
	case TabLinks:
		var cmd tea.Cmd
		m.links.filterBar, cmd = m.links.filterBar.Focus()
		return cmd
	case TabBookmarks:
		var cmd tea.Cmd
		m.bookmarks.filterBar, cmd = m.bookmarks.filterBar.Focus()
		return cmd
	case TabTags:
		if m.tags.state == tagStateFiles {
			return m.tags.filePicker.Input.Focus()
		}
		return m.tags.tagPicker.Input.Focus()
	case TabTasks:
		var cmd tea.Cmd
		m.tasks.filterBar, cmd = m.tasks.filterBar.Focus()
		return cmd
	default:
		return nil
	}
}

func (m *Model) fetchDocument(path string, line int) tea.Cmd {
	client := m.client
	vaultID := m.vaultID
	return func() tea.Msg {
		doc, err := client.GetDocument(context.Background(), vaultID, path)
		if err != nil {
			return documentFetchedMsg{err: fmt.Errorf("fetch %s: %w", path, err)}
		}
		return documentFetchedMsg{doc: doc, line: line}
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
