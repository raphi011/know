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

// Model is the root browser model composing a finder, links list, bookmarks, and viewer.
type Model struct {
	state     viewState
	activeTab Tab
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
	initial := TabAllFiles
	if len(startTab) > 0 {
		initial = startTab[0]
	}
	return Model{
		state:        stateFinding,
		activeTab:    initial,
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
	cmds := []tea.Cmd{m.finder.Init()}
	// Start loading links and bookmarks immediately so they're ready when user switches tabs
	cmds = append(cmds, m.links.loadLinks(), m.bookmarks.loadBookmarks())
	return tea.Batch(cmds...)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		contentHeight := msg.Height - 1 // reserve 1 line for tab bar
		m.finder.picker.SetSize(msg.Width, contentHeight)
		m.links.width = msg.Width
		m.links.height = contentHeight
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

		// Tab switching with 1/2 keys (only in finding state, and only when
		// the links model isn't in a confirmation mode)
		if m.state == stateFinding && !m.links.confirmDelete {
			switch msg.String() {
			case "1":
				if m.activeTab != TabAllFiles {
					m.activeTab = TabAllFiles
					return m, m.finder.picker.Input.Focus()
				}
				return m, nil
			case "2":
				if m.activeTab != TabLinks {
					m.activeTab = TabLinks
				}
				return m, nil
			case "3":
				if m.activeTab != TabBookmarks {
					m.activeTab = TabBookmarks
				}
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
			case TabAllFiles:
				m.finder.statusErr = errMsg
			case TabLinks:
				m.links.statusErr = errMsg
			case TabBookmarks:
				m.bookmarks.statusErr = errMsg
			}
			return m, nil
		}
		m.finder.statusErr = ""
		m.links.statusErr = ""
		m.bookmarks.statusErr = ""

		// Audio files: fetch binary asset for playback.
		if strings.HasPrefix(msg.doc.MimeType, "audio/") && strings.HasSuffix(msg.doc.Path, ".wav") {
			return m, m.fetchAudio(msg.doc)
		}

		m.viewer = newViewer(msg.doc.Path, renderContent(m.renderer, msg.doc), m.width, m.height)
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
				m.viewer = newViewer(msg.path, renderContent(m.renderer, doc), m.width, m.height)
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
		if m.activeTab == TabAllFiles {
			return m, m.finder.picker.Input.Focus()
		}
		return m, nil

	case bookmarkToggledMsg:
		var cmd tea.Cmd
		m.bookmarks, cmd = m.bookmarks.Update(msg)
		if msg.err == nil && msg.added {
			m.viewer.bookmarked = true
		} else if msg.err == nil && !msg.added {
			m.viewer.bookmarked = false
		}
		return m, cmd
	}

	// Async messages must always reach their model regardless of active tab.
	switch msg.(type) {
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
		// Handle bookmark toggle in viewer
		if keyMsg, ok := msg.(tea.KeyPressMsg); ok && keyMsg.String() == "b" {
			return m, m.bookmarks.toggleBookmark(m.viewer.path, !m.viewer.bookmarked)
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
