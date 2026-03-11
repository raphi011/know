// Package tui provides a bubbletea v2 inline terminal UI for Knowhow.
package tui

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/glamour"
	"github.com/raphi011/knowhow/internal/api"
)

// Model is the root bubbletea model for inline chat.
type Model struct {
	client         *Client
	vaultID        string
	conversationID string

	// Input
	input textinput.Model

	// Streaming state
	streaming       bool
	streamParts     []ContentPart
	pendingApproval *StreamEvent
	errMsg          string

	// Rendering
	renderer      *glamour.TermRenderer
	rendererWidth int
	width         int

	// Lifecycle
	ctx      context.Context
	cancel   context.CancelFunc
	focusCmd tea.Cmd // cursor blink cmd from textinput.Focus()
	ready    bool    // true after conversation created
}

// conversationCreatedMsg is sent when a new conversation is created on startup.
type conversationCreatedMsg struct {
	conv *api.Conversation
	err  error
}

// streamEventMsg wraps SSE events from the chat stream.
// ch carries the channel for chaining listenForEvents after each event.
type streamEventMsg struct {
	event StreamEvent
	ch    <-chan StreamEvent
	done  bool
}

// approvalSentMsg is sent after an approval/rejection is processed.
type approvalSentMsg struct {
	err error
}

// NewModel creates a new TUI model. The caller should defer model.Close()
// to cancel in-flight requests on quit.
func NewModel(client *Client, vaultID string) Model {
	ctx, cancel := context.WithCancel(context.Background())

	ti := textinput.New()
	ti.Placeholder = "Type a message..."
	ti.CharLimit = 4096
	ti.Prompt = inputPromptStyle.Render("> ")
	focusCmd := ti.Focus()

	r, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(0),
	)
	if err != nil {
		slog.Warn("glamour renderer init failed, markdown will render as plain text", "error", err)
	}

	return Model{
		client:   client,
		vaultID:  vaultID,
		input:    ti,
		renderer: r,
		ctx:      ctx,
		cancel:   cancel,
		focusCmd: focusCmd,
	}
}

// Close cancels the TUI context, stopping in-flight requests.
func (m *Model) Close() {
	m.cancel()
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.createConversation(),
		m.focusCmd,
	)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.updateRenderer()
		m.input.SetWidth(msg.Width - 4)
		return m, nil

	case tea.KeyPressMsg:
		return m.handleKey(msg)

	case conversationCreatedMsg:
		if msg.err != nil {
			m.errMsg = fmt.Sprintf("Failed to create conversation: %v", msg.err)
			slog.Warn("failed to create conversation", "error", msg.err)
			return m, nil
		}
		m.conversationID = msg.conv.ID
		m.ready = true
		banner := headerStyle.Render("knowhow") + " " +
			statusStyle.Render("vault:"+m.vaultID)
		return m, tea.Println(banner)

	case streamEventMsg:
		return m.handleStreamEvent(msg)

	case approvalSentMsg:
		if msg.err != nil {
			m.errMsg = fmt.Sprintf("Approval failed: %v", msg.err)
		}
		return m, nil
	}

	// Pass other messages to text input (cursor blink, etc.)
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m Model) View() tea.View {
	var content strings.Builder

	if !m.ready {
		content.WriteString(statusStyle.Render("Creating conversation..."))
		return tea.NewView(content.String())
	}

	// Show streaming content in the managed region
	if m.streaming && len(m.streamParts) > 0 {
		content.WriteString(renderStreamParts(m.renderer, m.streamParts, m.pendingApproval, m.width))
		content.WriteString("\n")
	}

	// Error line
	if m.errMsg != "" {
		content.WriteString(errorMsgStyle.Render(m.errMsg))
		content.WriteString("\n")
	}

	// Input area
	if m.streaming {
		content.WriteString(statusStyle.Render("Streaming response..."))
	} else {
		content.WriteString(m.input.View())
	}

	return tea.NewView(content.String())
}

func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// Clear error on any keypress
	if m.errMsg != "" {
		m.errMsg = ""
	}

	// Global quit
	if key.Matches(msg, keys.Quit) {
		m.cancel()
		return m, tea.Quit
	}

	// Approval keys when pending
	if m.pendingApproval != nil {
		switch {
		case key.Matches(msg, keys.Approve):
			return m, m.sendApproval("approve_all")
		case key.Matches(msg, keys.Reject):
			return m, m.sendApproval("reject")
		}
	}

	// Two-phase ESC: clear input first
	if key.Matches(msg, keys.Escape) {
		if m.input.Value() != "" {
			m.input.SetValue("")
			return m, nil
		}
		return m, nil
	}

	// Send message
	if key.Matches(msg, keys.Send) {
		if cmd := m.sendMessage(); cmd != nil {
			return m, cmd
		}
		return m, nil
	}

	// Pass to text input
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m *Model) createConversation() tea.Cmd {
	client := m.client
	ctx := m.ctx
	vaultID := m.vaultID
	return func() tea.Msg {
		conv, err := client.CreateConversation(ctx, vaultID)
		return conversationCreatedMsg{conv: conv, err: err}
	}
}

func (m *Model) sendMessage() tea.Cmd {
	content := m.input.Value()
	if content == "" || m.streaming || !m.ready {
		return nil
	}

	m.input.SetValue("")
	m.streaming = true
	m.streamParts = nil

	client := m.client
	ctx := m.ctx
	convID := m.conversationID
	vaultID := m.vaultID

	startStreamCmd := func() tea.Msg {
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

	// Print user message to scrollback, then start streaming
	return tea.Sequence(
		tea.Println(renderUserMessage(content)),
		startStreamCmd,
	)
}

func (m *Model) listenForEvents(ch <-chan StreamEvent) tea.Cmd {
	return func() tea.Msg {
		event, ok := <-ch
		if !ok {
			return streamEventMsg{done: true}
		}
		return streamEventMsg{event: event, ch: ch}
	}
}

func (m *Model) sendApproval(action string) tea.Cmd {
	if m.pendingApproval == nil {
		return nil
	}

	client := m.client
	ctx := m.ctx
	convID := m.conversationID
	callID := m.pendingApproval.CallID

	m.pendingApproval = nil

	return func() tea.Msg {
		err := client.Approve(ctx, convID, callID, action)
		return approvalSentMsg{err: err}
	}
}

func (m Model) handleStreamEvent(msg streamEventMsg) (tea.Model, tea.Cmd) {
	// Handle error events before checking done — when Chat() fails,
	// the message has both an error and done=true.
	if msg.event.Type == "error" {
		m.errMsg = msg.event.Content
	}

	if msg.done {
		cmd := m.finalizeStream()
		return m, cmd
	}

	// Chain the next event read for all non-terminal events
	var nextCmd tea.Cmd
	if msg.ch != nil {
		nextCmd = m.listenForEvents(msg.ch)
	}

	switch msg.event.Type {
	case "text":
		m.appendText(msg.event.Content)
	case "tool_start":
		m.streamParts = append(m.streamParts, ContentPart{
			Type:     PartToolCall,
			ToolName: msg.event.Tool,
			CallID:   msg.event.CallID,
			Status:   ToolRunning,
		})
	case "tool_end":
		m.updateToolStatus(msg.event.CallID, msg.event.Tool, ToolComplete)
	case "tool_approval_required":
		m.pendingApproval = &msg.event
	case "conv_id":
		m.conversationID = msg.event.ConvID
	case "error":
		m.streamParts = append(m.streamParts, ContentPart{
			Type:    PartError,
			Content: msg.event.Content,
		})
		cmd := m.finalizeStream()
		return m, tea.Batch(cmd, nextCmd)
	case "msg_end":
		cmd := m.finalizeStream()
		return m, tea.Batch(cmd, nextCmd)
	}

	return m, nextCmd
}

// appendText adds text to the last PartText or creates a new one.
// This naturally coalesces consecutive text events while keeping
// tool calls as separate interleaved parts.
func (m *Model) appendText(s string) {
	if n := len(m.streamParts); n > 0 && m.streamParts[n-1].Type == PartText {
		m.streamParts[n-1].Content += s
		return
	}
	m.streamParts = append(m.streamParts, ContentPart{Type: PartText, Content: s})
}

// updateToolStatus finds a tool call part by CallID and updates its status.
// Falls back to matching by tool name if CallID is empty (older servers).
func (m *Model) updateToolStatus(callID, toolName string, status ToolStatus) {
	for i := len(m.streamParts) - 1; i >= 0; i-- {
		p := &m.streamParts[i]
		if p.Type != PartToolCall {
			continue
		}
		if callID != "" && p.CallID == callID {
			p.Status = status
			return
		}
		if callID == "" && p.ToolName == toolName && p.Status == ToolRunning {
			p.Status = status
			return
		}
	}
	slog.Warn("tool_end event had no matching tool_start", "callID", callID, "tool", toolName)
}

// finalizeStream commits the streaming response to terminal scrollback
// via tea.Println and resets streaming state.
func (m *Model) finalizeStream() tea.Cmd {
	if !m.streaming {
		return nil
	}
	m.streaming = false

	rendered := renderAssistantMessage(m.renderer, m.streamParts)
	m.streamParts = nil
	m.pendingApproval = nil

	if rendered == "" {
		return nil
	}
	return tea.Println(rendered)
}

// updateRenderer recreates the glamour renderer when terminal width changes.
func (m *Model) updateRenderer() {
	wrapWidth := m.width - 4
	if wrapWidth > 0 && wrapWidth != m.rendererWidth {
		r, err := glamour.NewTermRenderer(
			glamour.WithAutoStyle(),
			glamour.WithWordWrap(wrapWidth),
		)
		if err != nil {
			slog.Debug("glamour renderer re-creation failed on resize", "error", err)
		} else {
			m.renderer = r
			m.rendererWidth = wrapWidth
		}
	}
}
