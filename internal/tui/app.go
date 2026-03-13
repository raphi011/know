// Package tui provides a bubbletea v2 inline terminal UI for Knowhow.
package tui

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/glamour"
	"github.com/raphi011/knowhow/internal/api"
	"github.com/raphi011/knowhow/internal/models"
)

// Model is the root bubbletea model for inline chat.
type Model struct {
	client         *Client
	vaultID        string
	conversationID string

	// Input
	input    textinput.Model
	fileList FileList
	spinner  spinner.Model

	// Streaming state
	streaming       bool
	streamParts     []ContentPart
	pendingApproval *StreamEvent
	errMsg          string

	// Token usage (cumulative across messages)
	tokenInput  int64
	tokenOutput int64

	// Context window tracking (from latest msg_end)
	contextWindowMax  int
	contextWindowUsed int64

	// Rendering
	renderer      *glamour.TermRenderer
	rendererWidth int
	glamourStyle  string // "dark" or "light" — detected before p.Run()
	width         int

	// Lifecycle
	ctx       context.Context
	cancel    context.CancelFunc
	termReady bool // true after first WindowSizeMsg (terminal setup done)
	ready     bool // true after conversation created
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

// NewModel creates a new TUI model. isDark should be detected before bubbletea
// starts (e.g. via lipgloss.HasDarkBackground) to avoid racing with bubbletea's
// terminal reader. The caller should defer model.Close().
func NewModel(client *Client, vaultID string, isDark bool) Model {
	ctx, cancel := context.WithCancel(context.Background())

	ti := textinput.New()
	ti.Placeholder = "Type a message..."
	ti.CharLimit = 4096
	ti.Prompt = inputPromptStyle.Render("> ")

	styles := ti.Styles()
	styles.Cursor.Blink = false
	ti.SetStyles(styles)

	glamourStyle := "light"
	if isDark {
		glamourStyle = "dark"
	}

	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(glamourStyle),
		glamour.WithWordWrap(maxTextWidth),
	)
	if err != nil {
		slog.Warn("glamour renderer init failed, markdown will render as plain text", "error", err)
	}

	sp := spinner.New(
		spinner.WithSpinner(spinner.MiniDot),
	)

	return Model{
		client:       client,
		vaultID:      vaultID,
		input:        ti,
		spinner:      sp,
		renderer:     r,
		glamourStyle: glamourStyle,
		ctx:          ctx,
		cancel:       cancel,
	}
}

// Close cancels the TUI context, stopping in-flight requests.
func (m *Model) Close() {
	m.cancel()
}

func (m Model) Init() tea.Cmd {
	return m.createConversation()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.updateRenderer()
		m.input.SetWidth(msg.Width - 4)
		if !m.termReady {
			m.termReady = true
			return m, m.tryFocus()
		}
		return m, nil

	case tea.KeyPressMsg:
		return m.handleKey(msg)

	case tea.PasteMsg:
		return m.handlePaste(msg)

	case conversationCreatedMsg:
		if msg.err != nil {
			m.errMsg = fmt.Sprintf("Failed to create conversation: %v", msg.err)
			slog.Warn("failed to create conversation", "error", msg.err)
			return m, nil
		}
		m.conversationID = msg.conv.ID
		m.ready = true
		return m, m.tryFocus()

	case streamEventMsg:
		return m.handleStreamEvent(msg)

	case approvalSentMsg:
		if msg.err != nil {
			m.errMsg = fmt.Sprintf("Approval failed: %v", msg.err)
			slog.Warn("approval failed", "conversationID", m.conversationID, "error", msg.err)
			return m, nil
		}
		// Approval triggers a resumed agent run — resubscribe to events
		m.streaming = true
		return m, tea.Batch(m.resubscribeAfterApproval(), m.spinner.Tick)

	case spinner.TickMsg:
		if m.streaming {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
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
		if m.errMsg != "" {
			content.WriteString(errorMsgStyle.Render(m.errMsg))
		} else {
			content.WriteString(statusStyle.Render("Creating conversation..."))
		}
		// Pad to same height as the ready view so the managed region
		// doesn't grow on transition and eat previous terminal output.
		content.WriteString("\n\n")
		return tea.NewView(content.String())
	}

	// Streaming content
	if m.streaming && len(m.streamParts) > 0 {
		content.WriteString(renderStreamParts(m.renderer, m.streamParts, m.pendingApproval, m.width))
	}

	// Error line
	if m.errMsg != "" {
		content.WriteString(errorMsgStyle.Render(m.errMsg))
		content.WriteString("\n")
	}

	// Blank line before prompt
	content.WriteString("\n")

	// Spinner (only during streaming)
	if m.streaming {
		content.WriteString(statusStyle.Render(m.spinner.View() + " Thinking..."))
		content.WriteString("\n\n")
	}

	// File list (above input, only when files are attached)
	if fl := m.fileList.View(m.width); fl != "" {
		content.WriteString(fl)
	}

	// Input always visible
	content.WriteString(m.input.View())
	content.WriteString("\n\n")
	content.WriteString(renderStatusBar(m.tokenInput, m.tokenOutput, m.vaultID, m.contextWindowMax, m.contextWindowUsed))

	return tea.NewView(content.String())
}

func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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

	// File list focused — handle navigation and deletion
	if m.fileList.Focused() {
		switch msg.String() {
		case "up":
			m.fileList.Up()
			return m, nil
		case "down":
			if m.fileList.AtBottom() {
				m.fileList.Blur()
				return m, m.input.Focus()
			}
			m.fileList.Down()
			return m, nil
		case "backspace":
			m.fileList.RemoveSelected()
			if !m.fileList.Focused() {
				// Last item was removed, return to input
				return m, m.input.Focus()
			}
			return m, nil
		case "escape":
			m.fileList.Blur()
			return m, m.input.Focus()
		default:
			return m, nil
		}
	}

	// Up arrow with files attached → focus file list
	if msg.String() == "up" && m.fileList.Len() > 0 {
		m.input.Blur()
		m.fileList.Focus()
		return m, nil
	}

	// ESC clears input text; no-op if already empty
	if key.Matches(msg, keys.Escape) {
		if m.input.Value() != "" {
			m.input.SetValue("")
		}
		return m, nil
	}

	// Send message
	if key.Matches(msg, keys.Send) {
		m.errMsg = ""
		return m, m.sendMessage()
	}

	// Pass to text input
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// handlePaste adds file paths to the file list or delegates to textinput for normal text.
func (m Model) handlePaste(msg tea.PasteMsg) (tea.Model, tea.Cmd) {
	content := strings.TrimSpace(msg.Content)

	if looksLikeFilePath(content) {
		if reason := m.fileList.Add(content); reason != "" {
			m.errMsg = reason
		}
		return m, nil
	}

	// Normal paste — delegate to textinput
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

	if content == "" && m.fileList.Len() == 0 {
		return nil
	}
	if content == "" {
		m.errMsg = "message is empty"
		return nil
	}
	if m.streaming || !m.ready {
		return nil
	}

	// Check for resolution errors before clearing
	var errs []string
	for _, att := range m.fileList.files {
		if att.Error != "" {
			errs = append(errs, att.Error)
		}
	}
	if len(errs) > 0 {
		m.errMsg = strings.Join(errs, "; ")
		return nil
	}

	// Past the point of no return — clear file list and input
	files := m.fileList.Clear()

	// Build wire-format attachments
	var chatAttachments []models.ChatAttachment
	for _, att := range files {
		chatAttachments = append(chatAttachments, models.ChatAttachment{
			Path:     att.Path,
			Content:  att.Content,
			MimeType: att.MimeType,
			Language: att.Language,
			Type:     toAttachmentType(att.Type),
		})
	}

	m.input.SetValue("")
	m.streaming = true
	m.streamParts = nil

	client := m.client
	ctx := m.ctx
	convID := m.conversationID
	vaultID := m.vaultID
	attachments := chatAttachments
	resolved := files

	startStreamCmd := func() tea.Msg {
		ch, err := client.Chat(ctx, convID, vaultID, content, attachments, false)
		if err != nil {
			return streamEventMsg{event: StreamEvent{Type: "error", Content: err.Error()}, done: true}
		}

		event, ok := <-ch
		if !ok {
			return streamEventMsg{done: true}
		}

		return streamEventMsg{event: event, ch: ch}
	}

	// Print user message to scrollback, then start streaming + spinner
	return tea.Sequence(
		tea.Println(renderUserMessage(content, resolved)),
		tea.Batch(startStreamCmd, m.spinner.Tick),
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
	interruptID := m.pendingApproval.InterruptID

	m.pendingApproval = nil

	return func() tea.Msg {
		err := client.Approve(ctx, convID, interruptID, action)
		return approvalSentMsg{err: err}
	}
}

// resubscribeAfterApproval subscribes to the resumed agent's SSE event stream.
func (m *Model) resubscribeAfterApproval() tea.Cmd {
	client := m.client
	ctx := m.ctx
	convID := m.conversationID

	return func() tea.Msg {
		ch, err := client.SubscribeEvents(ctx, convID)
		if err != nil {
			return streamEventMsg{event: StreamEvent{Type: "error", Content: fmt.Sprintf("resubscribe failed: %v", err)}, done: true}
		}

		event, ok := <-ch
		if !ok {
			return streamEventMsg{done: true}
		}
		return streamEventMsg{event: event, ch: ch}
	}
}

func (m Model) handleStreamEvent(msg streamEventMsg) (tea.Model, tea.Cmd) {
	// Handle error events before checking done — when Chat() fails,
	// the message has both an error and done=true.
	if msg.event.Type == "error" {
		slog.Warn("stream error", "conversationID", m.conversationID, "error", msg.event.Content)
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
	case "interrupted":
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
		m.tokenInput += msg.event.InputTokens
		m.tokenOutput += msg.event.OutputTokens
		m.contextWindowMax = msg.event.ContextWindowMax
		m.contextWindowUsed = msg.event.ContextWindowUsed
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

	// Keep pendingApproval — it will be cleared when the user approves/rejects
	// or when the next chat message starts.

	if rendered == "" {
		return nil
	}
	return tea.Println(rendered)
}

// tryFocus focuses the text input once both terminal setup and conversation
// creation are done. This avoids capturing terminal query responses as input.
func (m *Model) tryFocus() tea.Cmd {
	if m.termReady && m.ready {
		return m.input.Focus()
	}
	return nil
}

// updateRenderer recreates the glamour renderer when terminal width changes.
// Uses the pre-detected glamourStyle to avoid direct TTY reads that would
// race with bubbletea's TerminalReader.
func (m *Model) updateRenderer() {
	wrapWidth := min(m.width-4, maxTextWidth)
	if wrapWidth > 0 && wrapWidth != m.rendererWidth {
		r, err := glamour.NewTermRenderer(
			glamour.WithStandardStyle(m.glamourStyle),
			glamour.WithWordWrap(wrapWidth),
		)
		if err != nil {
			slog.Warn("glamour renderer re-creation failed on resize", "width", wrapWidth, "error", err)
		} else {
			m.renderer = r
			m.rendererWidth = wrapWidth
		}
	}
}
