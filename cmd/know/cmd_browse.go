package main

import (
	"context"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/raphi011/know/internal/models"
	"github.com/raphi011/know/internal/tui/browse"
	"github.com/spf13/cobra"
)

var (
	browseAPI     *apiFlags
	browseVaultID *string
)

var browseCmd = &cobra.Command{
	Use:   "browse",
	Short: "Browse vault files with fuzzy search",
	Long: `Launch a terminal UI for browsing files in a vault.

Fuzzy-find documents by path or title, then view rendered markdown.
Press Enter to open a file, Esc to return to the finder, q to quit.

  KNOW_SERVER_URL   Server base URL (default: http://localhost:4001)
  KNOW_TOKEN        API bearer token
  KNOW_VAULT        Default vault ID

Examples:
  know browse
  know browse --vault default`,
	RunE: runBrowse,
}

func init() {
	browseAPI = addAPIFlags(browseCmd)
	browseVaultID = addVaultFlag(browseCmd, browseAPI)
}

func runBrowse(_ *cobra.Command, _ []string) error {
	isDark := lipgloss.HasDarkBackground(os.Stdin, os.Stdout)

	client := browseAPI.newClient()

	files, err := client.ListFiles(context.Background(), *browseVaultID, "/", true)
	if err != nil {
		return fmt.Errorf("browse: %w", err)
	}

	// Filter out directories — only show documents
	docs := make([]models.FileEntry, 0, len(files))
	for _, f := range files {
		if !f.IsDir {
			docs = append(docs, f)
		}
	}

	model := browse.NewModel(client, *browseVaultID, docs, isDark)
	p := tea.NewProgram(model)
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("browse: %w", err)
	}

	return nil
}
