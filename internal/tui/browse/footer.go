package browse

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/raphi011/know/internal/tui/pick"
)

// hotkey represents a key binding shown in a tab footer.
type hotkey struct {
	key  string
	desc string
}

// renderHotkeys formats a list of hotkeys with styled key/description pairs.
func renderHotkeys(keys []hotkey) string {
	var parts []string
	for _, k := range keys {
		parts = append(parts, hotkeyKeyStyle.Render(k.key)+" "+hotkeyDescStyle.Render(k.desc))
	}
	return "  " + strings.Join(parts, "    ")
}

// renderFooter renders the bottom section: blank line + optional status + hotkeys.
func renderFooter(statusErr string, keys []hotkey) string {
	return renderFooterStatus(statusErr, "", keys)
}

// renderFooterStatus renders the bottom section with either an error or success status.
func renderFooterStatus(statusErr, statusOK string, keys []hotkey) string {
	var sb strings.Builder
	sb.WriteString("\n")
	if statusErr != "" {
		sb.WriteString(errStyle.Render("  " + statusErr))
		sb.WriteString("\n")
	} else if statusOK != "" {
		sb.WriteString(lipgloss.NewStyle().Foreground(pick.MutedColor).Render("  "+statusOK) + "\n")
	}
	sb.WriteString(renderHotkeys(keys))
	return sb.String()
}
