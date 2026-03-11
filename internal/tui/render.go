package tui

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/charmbracelet/glamour"
)

const maxTextWidth = 120

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
	ToolFailed                     // Tool execution failed (reserved — not yet emitted by server)
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

// renderMarkdown renders markdown content using glamour, falling back to
// raw content if the renderer is nil or rendering fails.
func renderMarkdown(renderer *glamour.TermRenderer, content string) string {
	if renderer == nil {
		return content
	}
	rendered, err := renderer.Render(content)
	if err != nil {
		slog.Debug("markdown render failed, using raw content", "error", err, "contentLen", len(content))
		return content
	}
	return strings.TrimSpace(rendered)
}

// renderToolStatus renders a single tool call part with status indicator.
func renderToolStatus(p ContentPart) string {
	switch p.Status {
	case ToolRunning:
		return toolRoleStyle.Render(fmt.Sprintf("  Running: %s", p.ToolName))
	case ToolComplete:
		return toolRoleStyle.Render(fmt.Sprintf("  %s complete", p.ToolName))
	case ToolFailed:
		return toolRoleStyle.Render(fmt.Sprintf("  %s failed", p.ToolName))
	default:
		return toolRoleStyle.Render(fmt.Sprintf("  %s", p.ToolName))
	}
}

// renderParts renders a sequence of content parts (text, tool calls, errors).
func renderParts(sb *strings.Builder, renderer *glamour.TermRenderer, parts []ContentPart) {
	for _, p := range parts {
		switch p.Type {
		case PartText:
			rendered := renderMarkdown(renderer, p.Content)
			sb.WriteString(assistantMsgStyle.MaxWidth(maxTextWidth).Render(rendered))
		case PartToolCall:
			sb.WriteString(renderToolStatus(p))
			sb.WriteString("\n")
		case PartError:
			sb.WriteString(errorMsgStyle.Render("Error: " + p.Content))
			sb.WriteString("\n")
		}
	}
}

// renderStreamParts renders the current streaming content (text + tool calls + errors)
// plus the approval prompt if pending. Used for the live managed region during streaming.
func renderStreamParts(renderer *glamour.TermRenderer, parts []ContentPart, pending *StreamEvent, width int) string {
	if len(parts) == 0 && pending == nil {
		return ""
	}

	var sb strings.Builder

	if len(parts) > 0 {
		sb.WriteString(assistantRoleStyle.Render("assistant"))
		sb.WriteString("\n")
		renderParts(&sb, renderer, parts)
	}

	if pending != nil {
		sb.WriteString("\n")
		sb.WriteString(renderApproval(pending, width))
	}

	return sb.String()
}

// renderUserMessage renders a user message with role label for scrollback output.
func renderUserMessage(content string) string {
	return "\n" + userRoleStyle.Render("you") + "\n" + userMsgStyle.MaxWidth(maxTextWidth).Render(content) + "\n"
}

// renderAssistantMessage renders a finalized assistant response for scrollback output.
// It walks the stream parts in order, rendering text and tool statuses interleaved.
func renderAssistantMessage(renderer *glamour.TermRenderer, parts []ContentPart) string {
	if len(parts) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString(assistantRoleStyle.Render("assistant"))
	sb.WriteString("\n")
	renderParts(&sb, renderer, parts)
	return sb.String()
}

// formatTokens formats a token count for display (e.g. 1234 → "1.2k", 1500000 → "1.5M").
func formatTokens(n int64) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fk", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

// renderStatusBar renders the inline status bar below the prompt.
func renderStatusBar(tokenInput, tokenOutput int64, vaultID string, _ int) string {
	if tokenInput == 0 && tokenOutput == 0 {
		return statusBarDetailStyle.Render(" vault: " + vaultID)
	}
	return statusBarDetailStyle.Render(
		fmt.Sprintf(" tokens: %s in / %s out │ vault: %s", formatTokens(tokenInput), formatTokens(tokenOutput), vaultID),
	)
}

// renderApproval renders the tool approval prompt box.
func renderApproval(event *StreamEvent, width int) string {
	var sb strings.Builder
	sb.WriteString("Tool approval required\n")
	if event.Tool != "" {
		fmt.Fprintf(&sb, "Tool: %s\n", event.Tool)
	}
	if event.Diff != "" {
		sb.WriteString("\n" + event.Diff + "\n")
	}
	sb.WriteString("\n")
	sb.WriteString(approveKeyStyle.Render("[a] approve") + "  " + rejectKeyStyle.Render("[r] reject"))

	boxWidth := max(width-4, 20)
	return approvalBoxStyle.Width(boxWidth).Render(sb.String())
}
