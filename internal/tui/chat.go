package tui

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/glamour"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/raphi011/knowhow/internal/api"
)

// chatPane is the right pane model.
type chatPane struct {
	viewport       viewport.Model
	input          textinput.Model
	client         *Client
	vaultID        string
	conversationID string
	messages       []*api.ChatMessage
	streaming      bool
	streamContent  string
	width          int
	height         int
	renderer       *glamour.TermRenderer
	rendererWidth  int // cached width to avoid unnecessary glamour re-creation
	ctx            context.Context

	// Tool approval state
	pendingApproval *StreamEvent

	// Error display
	errMsg string
}

func newChatPane(ctx context.Context, client *Client, vaultID string) chatPane {
	vp := viewport.New()
	vp.SetContent("Select or create a conversation to start chatting.")

	ti := textinput.New()
	ti.Placeholder = "Type a message..."
	ti.CharLimit = 4096

	r, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(0), // we'll set this dynamically
	)
	if err != nil {
		slog.Warn("glamour renderer init failed, markdown will render as plain text", "error", err)
	}

	return chatPane{
		viewport: vp,
		input:    ti,
		client:   client,
		vaultID:  vaultID,
		renderer: r,
		ctx:      ctx,
	}
}

// streamEventMsg wraps SSE events from the chat stream.
// ch carries the channel for chaining listenForEvents after each event.
type streamEventMsg struct {
	event StreamEvent
	ch    <-chan StreamEvent
	done  bool
}

// conversationLoadedMsg carries a loaded conversation with messages.
type conversationLoadedMsg struct {
	conv *api.Conversation
	err  error
}

// approvalSentMsg is sent after an approval/rejection is processed.
type approvalSentMsg struct {
	err error
}

func (cp *chatPane) loadConversation(id string) tea.Cmd {
	cp.conversationID = id
	client := cp.client
	ctx := cp.ctx
	return func() tea.Msg {
		conv, err := client.GetConversation(ctx, id)
		return conversationLoadedMsg{conv: conv, err: err}
	}
}

func (cp *chatPane) sendMessage() tea.Cmd {
	content := cp.input.Value()
	if content == "" || cp.streaming {
		return nil
	}

	cp.input.SetValue("")
	cp.streaming = true
	cp.streamContent = ""

	client := cp.client
	ctx := cp.ctx
	convID := cp.conversationID
	vaultID := cp.vaultID

	return func() tea.Msg {
		ch, err := client.Chat(ctx, convID, vaultID, content, false)
		if err != nil {
			return streamEventMsg{event: StreamEvent{Type: "error", Content: err.Error()}, done: true}
		}

		event, ok := <-ch
		if !ok {
			return streamEventMsg{done: true}
		}

		return streamEventMsg{event: event, ch: ch}
	}
}

func (cp *chatPane) listenForEvents(ch <-chan StreamEvent) tea.Cmd {
	return func() tea.Msg {
		event, ok := <-ch
		if !ok {
			return streamEventMsg{done: true}
		}
		return streamEventMsg{event: event, ch: ch}
	}
}

func (cp *chatPane) sendApproval(action string) tea.Cmd {
	if cp.pendingApproval == nil {
		return nil
	}

	client := cp.client
	ctx := cp.ctx
	convID := cp.conversationID
	callID := cp.pendingApproval.CallID

	cp.pendingApproval = nil

	return func() tea.Msg {
		err := client.Approve(ctx, convID, callID, action)
		return approvalSentMsg{err: err}
	}
}

func (cp *chatPane) setSize(w, h int) {
	cp.width = w
	cp.height = h
	inputH := 3 // input area height
	cp.viewport.SetWidth(w - 4) // account for border + padding
	cp.viewport.SetHeight(h - inputH - 4)

	// Only recreate glamour renderer when width actually changes
	wrapWidth := w - 8
	if wrapWidth > 0 && wrapWidth != cp.rendererWidth {
		r, err := glamour.NewTermRenderer(
			glamour.WithAutoStyle(),
			glamour.WithWordWrap(wrapWidth),
		)
		if err != nil {
			slog.Debug("glamour renderer re-creation failed on resize", "error", err)
		} else {
			cp.renderer = r
			cp.rendererWidth = wrapWidth
		}
	}
}

func (cp *chatPane) focus() tea.Cmd {
	return cp.input.Focus()
}

func (cp *chatPane) blur() {
	cp.input.Blur()
}

func (cp *chatPane) update(msg tea.Msg) tea.Cmd {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case conversationLoadedMsg:
		if msg.err != nil {
			cp.viewport.SetContent("Error loading conversation: " + msg.err.Error())
			return nil
		}
		cp.messages = msg.conv.Messages
		cp.updateViewport()

	case approvalSentMsg:
		if msg.err != nil {
			cp.errMsg = fmt.Sprintf("Approval failed: %v", msg.err)
		}
		return nil

	case streamEventMsg:
		if msg.done {
			cp.streaming = false
			// Reload conversation to get final messages
			if cp.conversationID != "" {
				return cp.loadConversation(cp.conversationID)
			}
			return nil
		}

		// Chain the next event read for all non-terminal events
		var nextCmd tea.Cmd
		if msg.ch != nil {
			nextCmd = cp.listenForEvents(msg.ch)
		}

		switch msg.event.Type {
		case "token":
			cp.streamContent += msg.event.Content
			cp.updateViewportWithStream()
			return nextCmd
		case "tool_start":
			cp.streamContent += fmt.Sprintf("\n🔧 Running: %s\n", msg.event.ToolName)
			cp.updateViewportWithStream()
			return nextCmd
		case "tool_result":
			cp.streamContent += fmt.Sprintf("✓ %s complete\n", msg.event.ToolName)
			cp.updateViewportWithStream()
			return nextCmd
		case "approval_request":
			cp.pendingApproval = &msg.event
			cp.updateViewportWithStream()
			return nextCmd
		case "error":
			cp.streamContent += fmt.Sprintf("\n❌ Error: %s\n", msg.event.Content)
			cp.streaming = false
			cp.updateViewportWithStream()
		case "done":
			cp.streaming = false
			if cp.conversationID != "" {
				return cp.loadConversation(cp.conversationID)
			}
		}
	}

	var cmd tea.Cmd
	cp.input, cmd = cp.input.Update(msg)
	if cmd != nil {
		cmds = append(cmds, cmd)
	}

	cp.viewport, cmd = cp.viewport.Update(msg)
	if cmd != nil {
		cmds = append(cmds, cmd)
	}

	return tea.Batch(cmds...)
}

func (cp *chatPane) handleKey(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	// Clear error on any keypress
	if cp.errMsg != "" {
		cp.errMsg = ""
	}

	// Handle approval keys when pending
	if cp.pendingApproval != nil {
		switch {
		case key.Matches(msg, keys.Approve):
			return cp.sendApproval("approve_all"), true
		case key.Matches(msg, keys.Reject):
			return cp.sendApproval("reject"), true
		}
	}

	switch {
	case key.Matches(msg, keys.Escape):
		// Two-phase ESC: clear input first, second ESC handled by parent
		if cp.input.Value() != "" {
			cp.input.SetValue("")
			return nil, true
		}
		return nil, false
	case key.Matches(msg, keys.Send):
		if !cp.streaming && cp.conversationID != "" {
			return cp.sendMessage(), true
		}
	}
	return nil, false
}

func (cp *chatPane) updateViewport() {
	cp.viewport.SetContent(cp.renderMessages())
	cp.viewport.GotoBottom()
}

func (cp *chatPane) updateViewportWithStream() {
	content := cp.renderMessages()
	if cp.streamContent != "" {
		content += "\n" + assistantRoleStyle.Render("assistant") + "\n"
		rendered := cp.renderMarkdown(cp.streamContent)
		content += assistantMsgStyle.Render(rendered)
	}
	if cp.pendingApproval != nil {
		content += "\n" + cp.renderApproval()
	}
	cp.viewport.SetContent(content)
	cp.viewport.GotoBottom()
}

func (cp *chatPane) renderMessages() string {
	if len(cp.messages) == 0 {
		return helpStyle.Render("No messages yet. Type a message and press Enter.")
	}

	var sb strings.Builder
	for _, m := range cp.messages {
		switch m.Role {
		case "user":
			sb.WriteString(userRoleStyle.Render("you"))
			sb.WriteString("\n")
			sb.WriteString(userMsgStyle.Render(m.Content))
		case "assistant":
			sb.WriteString(assistantRoleStyle.Render("assistant"))
			sb.WriteString("\n")
			rendered := cp.renderMarkdown(m.Content)
			sb.WriteString(assistantMsgStyle.Render(rendered))
		case "tool_result":
			name := "tool"
			if m.ToolName != nil {
				name = *m.ToolName
			}
			sb.WriteString(toolRoleStyle.Render("🔧 " + name))
			sb.WriteString("\n")
		default:
			sb.WriteString(lipgloss.NewStyle().Foreground(mutedColor).Render(m.Role))
			sb.WriteString("\n")
			sb.WriteString(m.Content)
		}
		sb.WriteString("\n\n")
	}
	return sb.String()
}

func (cp *chatPane) renderMarkdown(content string) string {
	if cp.renderer == nil {
		return content
	}
	rendered, err := cp.renderer.Render(content)
	if err != nil {
		slog.Debug("markdown render failed, using raw content", "error", err)
		return content
	}
	return strings.TrimSpace(rendered)
}

func (cp *chatPane) renderApproval() string {
	var sb strings.Builder
	sb.WriteString("⚠️  Tool approval required\n")
	if cp.pendingApproval.ToolName != "" {
		sb.WriteString(fmt.Sprintf("Tool: %s\n", cp.pendingApproval.ToolName))
	}
	if cp.pendingApproval.Diff != "" {
		sb.WriteString("\n" + cp.pendingApproval.Diff + "\n")
	}
	sb.WriteString("\n")
	sb.WriteString(approveKeyStyle.Render("[a] approve") + "  " + rejectKeyStyle.Render("[r] reject"))

	return approvalBoxStyle.Width(cp.width - 6).Render(sb.String())
}

func (cp *chatPane) view(active bool) string {
	style := inactivePaneBorder
	if active {
		style = activePaneBorder
	}

	// Input area
	inputBox := inputPromptStyle.Render("> ") + cp.input.View()
	if cp.streaming {
		inputBox = statusStyle.Render("⏳ Streaming response...")
	}
	if cp.conversationID == "" {
		inputBox = statusStyle.Render("← Select a conversation")
	}
	if cp.errMsg != "" {
		inputBox = errorMsgStyle.Render(cp.errMsg) + "\n" + inputBox
	}

	vpContent := cp.viewport.View()

	content := vpContent + "\n" + inputBox

	return style.
		Width(cp.width - 2).
		Height(cp.height - 2).
		Render(content)
}
