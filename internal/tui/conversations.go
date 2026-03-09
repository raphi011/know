package tui

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/raphi011/knowhow/internal/api"
)

// conversationItem implements list.Item for the conversation list.
type conversationItem struct {
	conv api.Conversation
}

func (i conversationItem) FilterValue() string { return i.conv.Title }

// conversationDelegate renders conversation list items.
type conversationDelegate struct{}

func (d conversationDelegate) Height() int                             { return 1 }
func (d conversationDelegate) Spacing() int                            { return 0 }
func (d conversationDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (d conversationDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	ci, ok := item.(conversationItem)
	if !ok {
		return
	}
	title := ci.conv.Title
	if title == "" {
		title = "(untitled)"
	}

	// Truncate to fit width
	maxW := m.Width() - 4
	if maxW > 0 && len(title) > maxW {
		title = title[:maxW-1] + "…"
	}

	if index == m.Index() {
		fmt.Fprint(w, lipgloss.NewStyle().
			Foreground(primaryColor).
			Bold(true).
			Render("▸ "+title))
	} else {
		fmt.Fprint(w, lipgloss.NewStyle().
			Foreground(mutedColor).
			Render("  "+title))
	}
}

// conversationList is the left pane model.
type conversationList struct {
	list    list.Model
	client  *Client
	vaultID string
	ctx     context.Context
	width   int
	height  int
	errMsg  string
}

func newConversationList(ctx context.Context, client *Client, vaultID string) conversationList {
	l := list.New(nil, conversationDelegate{}, 0, 0)
	l.Title = "Conversations"
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.SetFilteringEnabled(false)
	l.Styles.Title = titleStyle

	return conversationList{
		list:    l,
		client:  client,
		vaultID: vaultID,
		ctx:     ctx,
	}
}

// fetchConversationsMsg carries fetched conversations.
type fetchConversationsMsg struct {
	convs []api.Conversation
	err   error
}

// conversationCreatedMsg is sent when a new conversation is created.
type conversationCreatedMsg struct {
	conv *api.Conversation
	err  error
}

// conversationDeletedMsg is sent when a conversation is deleted.
type conversationDeletedMsg struct {
	err error
}

func (cl *conversationList) fetchConversations() tea.Cmd {
	client := cl.client
	ctx := cl.ctx
	vaultID := cl.vaultID
	return func() tea.Msg {
		convs, err := client.ListConversations(ctx, vaultID)
		return fetchConversationsMsg{convs: convs, err: err}
	}
}

func (cl *conversationList) createConversation() tea.Cmd {
	client := cl.client
	ctx := cl.ctx
	vaultID := cl.vaultID
	return func() tea.Msg {
		conv, err := client.CreateConversation(ctx, vaultID)
		return conversationCreatedMsg{conv: conv, err: err}
	}
}

func (cl *conversationList) deleteSelected() tea.Cmd {
	item := cl.selectedItem()
	if item == nil {
		return nil
	}
	client := cl.client
	ctx := cl.ctx
	id := item.conv.ID
	return func() tea.Msg {
		return conversationDeletedMsg{err: client.DeleteConversation(ctx, id)}
	}
}

func (cl *conversationList) selectedItem() *conversationItem {
	item := cl.list.SelectedItem()
	if item == nil {
		return nil
	}
	ci := item.(conversationItem)
	return &ci
}

func (cl *conversationList) setSize(w, h int) {
	cl.width = w
	cl.height = h
	cl.list.SetSize(w-2, h-2) // account for border
}

func (cl *conversationList) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case fetchConversationsMsg:
		if msg.err != nil {
			cl.errMsg = fmt.Sprintf("Fetch failed: %v", msg.err)
			slog.Warn("failed to fetch conversations", "error", msg.err)
			return nil
		}
		cl.errMsg = ""
		items := make([]list.Item, len(msg.convs))
		for i := range msg.convs {
			items[i] = conversationItem{conv: msg.convs[i]}
		}
		return cl.list.SetItems(items)

	case conversationCreatedMsg:
		if msg.err != nil {
			cl.errMsg = fmt.Sprintf("Create failed: %v", msg.err)
			slog.Warn("failed to create conversation", "error", msg.err)
			return nil
		}
		cl.errMsg = ""
		return cl.fetchConversations()

	case conversationDeletedMsg:
		if msg.err != nil {
			cl.errMsg = fmt.Sprintf("Delete failed: %v", msg.err)
			slog.Warn("failed to delete conversation", "error", msg.err)
			return nil
		}
		cl.errMsg = ""
		return cl.fetchConversations()
	}

	var cmd tea.Cmd
	cl.list, cmd = cl.list.Update(msg)
	return cmd
}

func (cl *conversationList) handleKey(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	// Clear error on ESC
	if key.Matches(msg, keys.Escape) && cl.errMsg != "" {
		cl.errMsg = ""
		return nil, true
	}

	switch {
	case key.Matches(msg, keys.NewConv):
		return cl.createConversation(), true
	case key.Matches(msg, keys.DeleteConv):
		return cl.deleteSelected(), true
	}
	return nil, false
}

func (cl *conversationList) view(active bool) string {
	style := inactivePaneBorder
	if active {
		style = activePaneBorder
	}

	content := cl.list.View()

	// Add error + help at bottom
	var footer string
	if cl.errMsg != "" {
		footer = errorMsgStyle.Render(cl.errMsg) + "\n"
	}
	footer += helpStyle.Render("n:new  d:del  tab:switch")

	// Calculate available content height
	availH := cl.height - 4 // border + help line
	lines := strings.Split(content, "\n")
	if len(lines) > availH {
		lines = lines[:availH]
	}

	return style.
		Width(cl.width - 2).
		Height(cl.height - 2).
		Render(strings.Join(lines, "\n") + "\n" + footer)
}
