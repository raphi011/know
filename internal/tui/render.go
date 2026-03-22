package tui

import (
	"fmt"
	"log/slog"
	"strings"

	imgcolor "image/color"

	"charm.land/bubbles/v2/viewport"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/glamour"
	"github.com/raphi011/know/internal/diff"
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
	Input    map[string]any        // for PartToolCall — tool arguments (set on tool_start)
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

// renderToolStatus renders a single tool call part in function-call style.
// Running:  tool_name(key_arg)
// Complete: tool_name(key_arg) · detail
// Failed:   tool_name(key_arg) · failed
func renderToolStatus(p ContentPart) string {
	keyArg := toolKeyArg(p.ToolName, p.Input)
	base := fmt.Sprintf("  %s(%s)", p.ToolName, keyArg)

	switch p.Status {
	case ToolRunning:
		return toolRoleStyle.Render(base)
	case ToolComplete:
		if detail := toolDetail(p.ToolName, p.Meta); detail != "" {
			return toolRoleStyle.Render(fmt.Sprintf("%s · %s", base, detail))
		}
		return toolRoleStyle.Render(base)
	case ToolFailed:
		return toolRoleStyle.Render(fmt.Sprintf("%s · failed", base))
	default:
		return toolRoleStyle.Render(base)
	}
}

// toolKeyArg extracts the most relevant argument from tool input for display.
func toolKeyArg(toolName string, input map[string]any) string {
	if input == nil {
		return ""
	}
	// Map tool names to their key argument field.
	var field string
	switch toolName {
	case tools.ToolReadDocument, tools.ToolCreateDocument, tools.ToolEditDocument,
		tools.ToolEditDocumentSection, tools.ToolGetDocumentVersions:
		field = "path"
	case tools.ToolSearchDocuments, tools.ToolWebSearch:
		field = "query"
	case tools.ToolListFolderContents:
		field = "folder"
	case tools.ToolListFolders:
		field = "parent"
	case tools.ToolCreateMemory:
		field = "title"
	}
	if field != "" {
		if v, ok := input[field].(string); ok {
			return v
		}
	}
	return ""
}

// toolDetail returns a short summary string from tool result metadata.
// The key argument (path, query) is already shown by toolKeyArg, so
// this returns supplementary info like counts or content length.
func toolDetail(toolName string, meta *tools.ToolResultMeta) string {
	if meta == nil {
		return ""
	}
	switch toolName {
	case tools.ToolReadDocument:
		if meta.ContentLength != nil {
			return fmt.Sprintf("%d chars", *meta.ContentLength)
		}
	case tools.ToolSearchDocuments:
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
	case tools.ToolListFolderContents, tools.ToolListFolders, tools.ToolListLabels:
		if meta.ResultCount != nil {
			return fmt.Sprintf("%d results", *meta.ResultCount)
		}
	case tools.ToolGetDocumentVersions:
		if meta.ResultCount != nil {
			return fmt.Sprintf("%d versions", *meta.ResultCount)
		}
	case tools.ToolWebSearch:
		if meta.WebResultCount != nil {
			return fmt.Sprintf("%d results", *meta.WebResultCount)
		}
	}
	return ""
}

// renderParts renders a sequence of content parts (text, tool calls, errors).
// Inserts a blank line when transitioning from tool calls to text.
func renderParts(sb *strings.Builder, renderer *glamour.TermRenderer, parts []ContentPart) {
	prevType := PartType(-1)
	for _, p := range parts {
		switch p.Type {
		case PartText:
			if prevType == PartToolCall {
				sb.WriteString("\n")
			}
			rendered := renderMarkdown(renderer, p.Content)
			sb.WriteString(assistantMsgStyle.MaxWidth(maxTextWidth).Render(rendered))
		case PartToolCall:
			sb.WriteString(renderToolStatus(p))
			sb.WriteString("\n")
		case PartError:
			sb.WriteString(errorMsgStyle.Render("Error: " + p.Content))
			sb.WriteString("\n")
		}
		prevType = p.Type
	}
}

// renderStreamParts renders the current streaming content (text + tool calls + errors).
// Used for the live managed region during streaming.
func renderStreamParts(renderer *glamour.TermRenderer, parts []ContentPart, width int) string {
	if len(parts) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString(assistantRoleStyle.Render("assistant"))
	sb.WriteString("\n")
	renderParts(&sb, renderer, parts)
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

// renderDiffDialog renders the scrollable diff approval dialog.
func renderDiffDialog(dialog *approvalDialog, width, height int) string {
	if len(dialog.approvals) == 0 {
		return ""
	}
	if dialog.current < 0 || dialog.current >= len(dialog.approvals) {
		return ""
	}

	cur := &dialog.approvals[dialog.current]
	approval := cur.event.Approval

	var sb strings.Builder

	// Header: tool name + path + stats + navigation
	header := renderDiffHeader(cur.event.Tool, approval, dialog.current, len(dialog.approvals))
	sb.WriteString(diffHeaderStyle.Render(header))
	sb.WriteString("\n")

	// Viewport (scrollable diff body)
	sb.WriteString(dialog.viewport.View())
	sb.WriteString("\n")

	// Footer: key hints
	var hints []string
	switch cur.decision {
	case decisionPending:
		hints = append(hints,
			approveKeyStyle.Render("[a] approve"),
			rejectKeyStyle.Render("[r] reject"),
		)
	case decisionApproved:
		hints = append(hints, approveKeyStyle.Render("approved"))
	case decisionRejected:
		hints = append(hints, rejectKeyStyle.Render("rejected"))
	}
	hints = append(hints, diffNavStyle.Render("[↑↓] scroll"))
	if len(dialog.approvals) > 1 {
		hints = append(hints, diffNavStyle.Render("[◄►] prev/next"))
	}
	sb.WriteString(strings.Join(hints, "  "))

	boxWidth := max(width-4, 20)
	return approvalBoxStyle.Width(boxWidth).Render(sb.String())
}

// renderDiffHeader builds the header line for the diff dialog.
func renderDiffHeader(toolName string, approval *Approval, current, total int) string {
	var parts []string
	parts = append(parts, toolName)

	if approval != nil && approval.Path != "" {
		parts[0] = fmt.Sprintf("%s(%s)", toolName, approval.Path)
	}

	if approval != nil && approval.Diff != nil {
		stats := approval.Diff.Stats
		parts = append(parts, fmt.Sprintf("+%d -%d (%d hunks)", stats.Additions, stats.Deletions, stats.HunksCount))
	} else if approval != nil && approval.IsNew {
		parts = append(parts, "new document")
	}

	if total > 1 {
		parts = append(parts, fmt.Sprintf("[%d/%d] ◄ ►", current+1, total))
	}

	return "─── " + strings.Join(parts, " ─── ") + " ───"
}

// renderHunks renders diff hunks with colored lines and line number gutter.
func renderHunks(hunks []diff.Hunk) string {
	var sb strings.Builder

	// Find max line number width for gutter alignment
	maxLineNo := 0
	for _, h := range hunks {
		for _, line := range h.Lines {
			if line.OldLineNo != nil && *line.OldLineNo > maxLineNo {
				maxLineNo = *line.OldLineNo
			}
			if line.NewLineNo != nil && *line.NewLineNo > maxLineNo {
				maxLineNo = *line.NewLineNo
			}
		}
	}
	gutterWidth := max(len(fmt.Sprintf("%d", maxLineNo)), 2)

	for i, h := range hunks {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(diffHunkHeaderStyle.Render(h.Header()))
		sb.WriteString("\n")

		for _, line := range h.Lines {
			oldNo := strings.Repeat(" ", gutterWidth)
			newNo := strings.Repeat(" ", gutterWidth)
			if line.OldLineNo != nil {
				oldNo = fmt.Sprintf("%*d", gutterWidth, *line.OldLineNo)
			}
			if line.NewLineNo != nil {
				newNo = fmt.Sprintf("%*d", gutterWidth, *line.NewLineNo)
			}

			gutter := diffGutterStyle.Render(oldNo+" "+newNo) +
				diffSeparatorStyle.Render(" │")

			content := strings.TrimRight(line.Content, "\n")
			switch line.Type {
			case diff.DiffAdd:
				sb.WriteString(gutter + diffAddStyle.Render("+"+content))
			case diff.DiffDelete:
				sb.WriteString(gutter + diffDeleteStyle.Render("-"+content))
			case diff.DiffContext:
				sb.WriteString(gutter + " " + content)
			}
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// renderNewDocPreview renders the full content of a new document, all green with + prefix.
func renderNewDocPreview(content string) string {
	var sb strings.Builder
	lines := strings.Split(content, "\n")

	gutterWidth := max(len(fmt.Sprintf("%d", len(lines))), 2)

	for i, line := range lines {
		lineNo := fmt.Sprintf("%*d", gutterWidth, i+1)
		gutter := diffGutterStyle.Render(lineNo) + diffSeparatorStyle.Render(" │")
		sb.WriteString(gutter + diffAddStyle.Render("+"+line) + "\n")
	}

	return sb.String()
}

// buildDiffContent builds the viewport content string for a given approval.
// Falls back to a plain text summary when no diff data is available.
func buildDiffContent(toolName string, approval *Approval) string {
	if approval == nil {
		return fmt.Sprintf("Tool: %s\nApproval required (no diff available)", toolName)
	}
	if approval.IsNew && approval.Content != "" {
		return renderNewDocPreview(approval.Content)
	}
	if approval.Diff != nil && len(approval.Diff.Hunks) > 0 {
		return renderHunks(approval.Diff.Hunks)
	}
	return fmt.Sprintf("Tool: %s\nApproval required (no diff available)", toolName)
}

// newDiffViewport creates a viewport sized for the diff dialog.
func newDiffViewport(content string, width, height int) viewport.Model {
	// Reserve lines for header (1), footer (1), box border (2), blank line (1)
	vpHeight := max(height-5, 3)
	vpWidth := max(width-6, 20) // box border + padding

	vp := viewport.New(viewport.WithWidth(vpWidth), viewport.WithHeight(vpHeight))
	vp.SetContent(content)
	return vp
}
