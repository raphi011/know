package main

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/raphi011/knowhow/internal/tui"
	"github.com/spf13/cobra"
)

var (
	agentAPI     *apiFlags
	agentVaultID *string
)

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Launch the terminal UI for agent chat",
	Long: `Start an interactive terminal UI for chatting with the Knowhow agent.

The TUI connects to the server via REST API. Configure connection with
environment variables or flags:

  KNOWHOW_SERVER_URL   Server base URL (default: http://localhost:4001)
  KNOWHOW_TOKEN        API bearer token
  KNOWHOW_VAULT        Default vault ID

Examples:
  knowhow agent
  knowhow agent --vault default
  knowhow agent --api-url http://localhost:4001 --token kh_...`,
	RunE: runAgent,
}

func init() {
	agentAPI = addAPIFlags(agentCmd)
	agentVaultID = addVaultFlag(agentCmd)
}

func runAgent(_ *cobra.Command, _ []string) error {
	// Detect dark/light background BEFORE bubbletea starts, so we don't race
	// with its TerminalReader on the TTY fd. glamour.WithAutoStyle() calls
	// termenv.HasDarkBackground() which does a direct TTY read — unsafe once
	// bubbletea is running.
	isDark := lipgloss.HasDarkBackground(os.Stdin, os.Stdout)

	client := tui.NewClient(agentAPI.URL, agentAPI.Token)
	model := tui.NewModel(client, *agentVaultID, isDark)
	defer model.Close()

	p := tea.NewProgram(model)
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("agent: %w", err)
	}

	return nil
}
