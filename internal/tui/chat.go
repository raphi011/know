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

// PartType distinguishes the kind of content in a streaming response.
type PartType int

const (
	PartText     PartType = iota // Markdown text from the assistant
	PartToolCall                 // Tool invocation with status tracking
	PartError                    // Error message
)

// ToolStatus tracks the lifecycle of a tool call.
type ToolStatus int

const (
	ToolRunning  ToolStatus = iota // Tool is currently executing
	ToolComplete                   // Tool finished successfully
	ToolFailed                     // Tool execution failed
)

// ContentPart represents one segment of a streaming response.
// Parts are ordered sequentially and rendered in order, allowing
// text and tool calls to interleave naturally.
type ContentPart struct {
	Type     PartType
	Content  string     // text content or error message
	ToolName string     // for PartToolCall
	CallID   string     // for PartToolCall — key for in-place updates
	Status   ToolStatus // for PartToolCall
}

// chatPane is the right pane model.
type chatPane struct {
	viewport       viewport.Model
	input          textinput.Model
	client         *Client
	vaultID        string
	conversationID string
	messages       []*api.ChatMessage
	streaming      bool
	streamParts    []ContentPart // structured content parts during streaming
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
	ti.Prompt = inputPromptStyle.Render("> ")

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
	cp.streamParts = nil
	cp.messages = append(cp.messages, &api.ChatMessage{Role: "user", Content: content})
	cp.updateViewport()

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
	cp.input.SetWidth(w - 4 - lipgloss.Width(cp.input.Prompt)) // text area excludes prompt

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
		// Handle error events before checking done — when Chat() fails,
		// the message has both an error and done=true.
		if msg.event.Type == "error" {
			cp.errMsg = msg.event.Content
		}

		if msg.done {
			cp.finalizeStream()
			return nil
		}

		// Chain the next event read for all non-terminal events
		var nextCmd tea.Cmd
		if msg.ch != nil {
			nextCmd = cp.listenForEvents(msg.ch)
		}

		switch msg.event.Type {
		case "text":
			cp.appendText(msg.event.Content)
			cp.updateViewportWithStream()
			return nextCmd
		case "tool_start":
			cp.streamParts = append(cp.streamParts, ContentPart{
				Type:     PartToolCall,
				ToolName: msg.event.Tool,
				CallID:   msg.event.CallID,
				Status:   ToolRunning,
			})
			cp.updateViewportWithStream()
			return nextCmd
		case "tool_end":
			cp.updateToolStatus(msg.event.CallID, msg.event.Tool, ToolComplete)
			cp.updateViewportWithStream()
			return nextCmd
		case "tool_approval_required":
			cp.pendingApproval = &msg.event
			cp.updateViewportWithStream()
			return nextCmd
		case "conv_id":
			cp.conversationID = msg.event.ConvID
			return nextCmd
		case "error":
			cp.streamParts = append(cp.streamParts, ContentPart{
				Type:    PartError,
				Content: msg.event.Content,
			})
			cp.finalizeStream()
			return nextCmd // keep reading so the goroutine can exit cleanly
		case "msg_end":
			cp.finalizeStream()
			return nextCmd // keep reading; channel close sends done
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

// appendText adds text to the last PartText or creates a new one.
// This naturally coalesces consecutive text events while keeping
// tool calls as separate interleaved parts.
func (cp *chatPane) appendText(s string) {
	if n := len(cp.streamParts); n > 0 && cp.streamParts[n-1].Type == PartText {
		cp.streamParts[n-1].Content += s
		return
	}
	cp.streamParts = append(cp.streamParts, ContentPart{Type: PartText, Content: s})
}

// updateToolStatus finds a tool call part by CallID and updates its status.
// Falls back to matching by tool name if CallID is empty (older servers).
func (cp *chatPane) updateToolStatus(callID, toolName string, status ToolStatus) {
	// Search backwards — most recent match is the one we want
	for i := len(cp.streamParts) - 1; i >= 0; i-- {
		p := &cp.streamParts[i]
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
	slog.Debug("tool_end event had no matching tool_start", "callID", callID, "tool", toolName)
}

// finalizeStream commits streamed content into messages and resets streaming state.
func (cp *chatPane) finalizeStream() {
	if !cp.streaming {
		return
	}
	cp.streaming = false

	// Build messages from stream parts, preserving interleaving order
	var text strings.Builder
	for _, p := range cp.streamParts {
		switch p.Type {
		case PartText:
			text.WriteString(p.Content)
		case PartToolCall:
			name := p.ToolName
			cp.messages = append(cp.messages, &api.ChatMessage{
				Role: "tool_result", ToolName: &name,
			})
		case PartError:
			cp.messages = append(cp.messages, &api.ChatMessage{
				Role: "assistant", Content: "Error: " + p.Content,
			})
		}
	}
	if text.Len() > 0 {
		cp.messages = append(cp.messages, &api.ChatMessage{Role: "assistant", Content: text.String()})
	}

	cp.streamParts = nil
	cp.pendingApproval = nil
	cp.updateViewport()
}

func (cp *chatPane) updateViewportWithStream() {
	content := cp.renderMessages()

	if len(cp.streamParts) > 0 {
		content += "\n" + assistantRoleStyle.Render("assistant") + "\n"
	}
	for _, p := range cp.streamParts {
		switch p.Type {
		case PartText:
			rendered := cp.renderMarkdown(p.Content)
			content += assistantMsgStyle.Render(rendered)
		case PartToolCall:
			content += cp.renderToolStatus(p) + "\n"
		case PartError:
			content += errorMsgStyle.Render("Error: "+p.Content) + "\n"
		}
	}

	if cp.pendingApproval != nil {
		content += "\n" + cp.renderApproval()
	}
	cp.viewport.SetContent(content)
	cp.viewport.GotoBottom()
}

// renderToolStatus renders a single tool call part with status indicator.
func (cp *chatPane) renderToolStatus(p ContentPart) string {
	switch p.Status {
	case ToolRunning:
		return toolRoleStyle.Render(fmt.Sprintf("🔧 Running: %s", p.ToolName))
	case ToolComplete:
		return toolRoleStyle.Render(fmt.Sprintf("✓ %s complete", p.ToolName))
	case ToolFailed:
		return toolRoleStyle.Render(fmt.Sprintf("✗ %s failed", p.ToolName))
	default:
		return toolRoleStyle.Render(fmt.Sprintf("🔧 %s", p.ToolName))
	}
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
			sb.WriteString(toolRoleStyle.Render("✓ " + name + " complete"))
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
	if cp.pendingApproval.Tool != "" {
		sb.WriteString(fmt.Sprintf("Tool: %s\n", cp.pendingApproval.Tool))
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
	inputBox := cp.input.View()
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
