package tui

import (
	"fmt"
	"log/slog"
	"strings"

	imgcolor "image/color"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/glamour"
	"github.com/raphi011/know/internal/tools"
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
	Content  string                // text content or error message
	ToolName string                // for PartToolCall
	CallID   string                // for PartToolCall — key for in-place updates
	Status   ToolStatus            // for PartToolCall
	Meta     *tools.ToolResultMeta // for PartToolCall — result metadata (set on tool_end)
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
		detail := toolDetail(p.ToolName, p.Meta)
		if detail != "" {
			return toolRoleStyle.Render(fmt.Sprintf("  %s: %s", p.ToolName, detail))
		}
		return toolRoleStyle.Render(fmt.Sprintf("  %s complete", p.ToolName))
	case ToolFailed:
		return toolRoleStyle.Render(fmt.Sprintf("  %s failed", p.ToolName))
	default:
		return toolRoleStyle.Render(fmt.Sprintf("  %s", p.ToolName))
	}
}

// toolDetail returns a short summary string from tool result metadata.
func toolDetail(toolName string, meta *tools.ToolResultMeta) string {
	if meta == nil {
		return ""
	}
	switch toolName {
	case "create_document", "edit_document", "edit_document_section",
		"read_document", "create_memory", "get_document_versions":
		if meta.DocumentPath != nil {
			return *meta.DocumentPath
		}
	case "search_documents":
		var parts []string
		if meta.ResultCount != nil {
			parts = append(parts, fmt.Sprintf("%d docs", *meta.ResultCount))
		}
		if meta.ChunkCount != nil {
			parts = append(parts, fmt.Sprintf("%d chunks", *meta.ChunkCount))
		}
		if len(parts) > 0 {
			return strings.Join(parts, ", ")
		}
	case "list_folder_contents", "list_folders", "list_labels":
		if meta.ResultCount != nil {
			return fmt.Sprintf("%d results", *meta.ResultCount)
		}
	case "web_search":
		if meta.WebResultCount != nil {
			return fmt.Sprintf("%d results", *meta.WebResultCount)
		}
	}
	return ""
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

// renderUserMessage renders a user message with role label and optional attachment indicators.
func renderUserMessage(content string, attachments []Attachment) string {
	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString(userRoleStyle.Render("you"))
	sb.WriteString("\n")
	sb.WriteString(userMsgStyle.MaxWidth(maxTextWidth).Render(content))
	sb.WriteString("\n")

	for _, att := range attachments {
		var line string
		if att.Type == FileTypeImage {
			line = fmt.Sprintf("  %s (%s, %s)", att.Name(), att.Path, formatSize(att.Size))
		} else {
			line = fmt.Sprintf("  %s (%s, %d lines)", att.Name(), att.Path, att.LineCount())
		}
		sb.WriteString(attachmentStyle.Render(line))
		sb.WriteString("\n")
	}

	return sb.String()
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

// renderContextBar renders a progress bar (12 block characters + percentage) showing context window usage.
// Color thresholds: green < 60%, yellow 60-85%, red > 85%.
// Returns empty string if context data is unavailable.
func renderContextBar(contextMax int, contextUsed int64) string {
	if contextMax <= 0 || contextUsed <= 0 {
		return ""
	}

	const barWidth = 12
	ratio := float64(contextUsed) / float64(contextMax)
	if ratio > 1 {
		ratio = 1
	}
	filled := min(int(ratio*barWidth+0.5), barWidth)
	pct := int(ratio * 100)

	var clr imgcolor.Color
	switch {
	case ratio > 0.85:
		clr = lipgloss.Color("#EF4444") // red
	case ratio >= 0.60:
		clr = lipgloss.Color("#F59E0B") // yellow
	default:
		clr = lipgloss.Color("#10B981") // green
	}

	filledStyle := lipgloss.NewStyle().Foreground(clr)
	emptyStyle := lipgloss.NewStyle().Foreground(mutedColor)

	bar := filledStyle.Render(strings.Repeat("█", filled)) +
		emptyStyle.Render(strings.Repeat("░", barWidth-filled))

	return fmt.Sprintf("%s %d%%", bar, pct)
}

// renderStatusBar renders the inline status bar below the prompt.
func renderStatusBar(tokenInput, tokenOutput int64, vaultID string, contextMax int, contextUsed int64) string {
	var parts []string

	if ctxBar := renderContextBar(contextMax, contextUsed); ctxBar != "" {
		parts = append(parts, ctxBar)
	}
	if tokenInput > 0 || tokenOutput > 0 {
		parts = append(parts, fmt.Sprintf("tokens: %s in / %s out", formatTokens(tokenInput), formatTokens(tokenOutput)))
	}
	parts = append(parts, "vault: "+vaultID)

	return statusBarDetailStyle.Render(" " + strings.Join(parts, " │ "))
}

// renderApproval renders the tool approval prompt box.
func renderApproval(event *StreamEvent, width int) string {
	var sb strings.Builder
	sb.WriteString("Tool approval required\n")
	if event.Tool != "" {
		fmt.Fprintf(&sb, "Tool: %s\n", event.Tool)
	}
	sb.WriteString("\n")
	sb.WriteString(approveKeyStyle.Render("[a] approve") + "  " + rejectKeyStyle.Render("[r] reject"))

	boxWidth := max(width-4, 20)
	return approvalBoxStyle.Width(boxWidth).Render(sb.String())
}
