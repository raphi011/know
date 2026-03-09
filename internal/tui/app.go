// Package tui provides a bubbletea v2 terminal UI for Knowhow.
package tui

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
)

// pane identifies which pane is focused.
type pane int

const (
	paneConversations pane = iota
	paneChat
)

// Model is the root bubbletea model.
type Model struct {
	conversations conversationList
	chat          chatPane
	activePane    pane
	width         int
	height        int
	vaultID       string
}

// NewModel creates a new TUI model.
func NewModel(client *Client, vaultID string) Model {
	return Model{
		conversations: newConversationList(client, vaultID),
		chat:          newChatPane(client, vaultID),
		activePane:    paneConversations,
		vaultID:       vaultID,
	}
}

func (m Model) Init() tea.Cmd {
	return m.conversations.fetchConversations()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.layoutPanes()
		return m, nil

	case tea.KeyPressMsg:
		// Global keys
		if key.Matches(msg, keys.Quit) {
			return m, tea.Quit
		}

		if key.Matches(msg, keys.Tab) {
			m.switchPane()
			return m, nil
		}

		// Delegate to active pane
		switch m.activePane {
		case paneConversations:
			if cmd, handled := m.conversations.handleKey(msg); handled {
				cmds = append(cmds, cmd)
				return m, tea.Batch(cmds...)
			}

			// Enter selects conversation
			if key.Matches(msg, keys.Send) {
				item := m.conversations.selectedItem()
				if item != nil {
					cmd := m.chat.loadConversation(item.conv.ID)
					m.activePane = paneChat
					cmds = append(cmds, cmd, m.chat.focus())
					return m, tea.Batch(cmds...)
				}
			}

		case paneChat:
			if cmd, handled := m.chat.handleKey(msg); handled {
				cmds = append(cmds, cmd)
				return m, tea.Batch(cmds...)
			}
		}
	}

	// Route messages to both panes
	cmd := m.conversations.update(msg)
	if cmd != nil {
		cmds = append(cmds, cmd)
	}

	cmd = m.chat.update(msg)
	if cmd != nil {
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m Model) View() tea.View {
	var v tea.View
	v.AltScreen = true

	if m.width == 0 {
		v = tea.NewView("Loading...")
		v.AltScreen = true
		return v
	}

	left := m.conversations.view(m.activePane == paneConversations)
	right := m.chat.view(m.activePane == paneChat)

	layout := lipgloss.JoinHorizontal(lipgloss.Top, left, right)

	v = tea.NewView(layout)
	v.AltScreen = true
	return v
}

func (m *Model) layoutPanes() {
	// Left pane: ~30% width, right pane: ~70%
	leftW := m.width * 3 / 10
	if leftW < 20 {
		leftW = 20
	}
	if leftW > 40 {
		leftW = 40
	}
	rightW := m.width - leftW

	m.conversations.setSize(leftW, m.height)
	m.chat.setSize(rightW, m.height)
}

func (m *Model) switchPane() {
	if m.activePane == paneConversations {
		m.activePane = paneChat
		m.chat.focus()
	} else {
		m.activePane = paneConversations
		m.chat.blur()
	}
}
