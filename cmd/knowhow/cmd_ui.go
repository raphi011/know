package main

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/raphi011/knowhow/internal/tui"
	"github.com/spf13/cobra"
)

var uiVaultID string

var uiCmd = &cobra.Command{
	Use:   "ui",
	Short: "Launch the terminal UI for agent chat",
	Long: `Start an interactive terminal UI for chatting with the Knowhow agent.

The TUI connects to the server via REST API. Configure connection with
environment variables or flags:

  KNOWHOW_SERVER_URL   Server base URL (default: http://localhost:4001)
  KNOWHOW_TOKEN        API bearer token
  KNOWHOW_VAULT        Default vault ID

Examples:
  knowhow ui
  knowhow ui --vault default
  knowhow ui --server-url http://localhost:4001 --token kh_...`,
	RunE: runUI,
}

func init() {
	uiCmd.Flags().StringVar(&uiVaultID, "vault", envOrDefault("KNOWHOW_VAULT", "default"), "vault name to use")
}

func runUI(_ *cobra.Command, _ []string) error {
	if err := requireToken(); err != nil {
		return fmt.Errorf("ui: %w", err)
	}

	client := tui.NewClient(apiURL, apiToken)
	model := tui.NewModel(client, uiVaultID)

	p := tea.NewProgram(model)
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("ui: %w", err)
	}

	return nil
}
