package main

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/raphi011/know/internal/tui"
	"github.com/spf13/cobra"
)

var agentVaultID *string

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Launch the terminal UI for agent chat",
	Long: `Start an interactive terminal UI for chatting with the Know agent.

The TUI connects to the server via REST API. Configure connection with
environment variables or flags:

  KNOW_SERVER_URL   Server base URL (default: http://localhost:4001)
  KNOW_TOKEN        API bearer token
  KNOW_VAULT        Default vault ID

Examples:
  know agent
  know agent --vault default
  know agent --api-url http://localhost:4001 --token kh_...`,
	RunE: runAgent,
}

func init() {
	agentVaultID = addVaultFlag(agentCmd)
}

func runAgent(_ *cobra.Command, _ []string) error {
	// Detect dark/light background BEFORE bubbletea starts, so we don't race
	// with its TerminalReader on the TTY fd. glamour.WithAutoStyle() calls
	// termenv.HasDarkBackground() which does a direct TTY read — unsafe once
	// bubbletea is running.
	isDark := lipgloss.HasDarkBackground(os.Stdin, os.Stdout)

	// Print banner before bubbletea starts — tea.Println inside the program
	// would clear all previous terminal output in inline mode.
	fmt.Println(tui.Banner())

	client := tui.NewClient(globalAPI.URL, globalAPI.Token)
	model := tui.NewModel(client, *agentVaultID, isDark)
	defer model.Close()

	p := tea.NewProgram(model)
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("agent: %w", err)
	}

	return nil
}
