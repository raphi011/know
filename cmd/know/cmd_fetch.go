package main

import (
	"fmt"

	"github.com/raphi011/know/internal/apiclient"
	"github.com/spf13/cobra"
)

var (
	fetchAPI     *apiFlags
	fetchVaultID *string
	fetchPath    string
)

var fetchCmd = &cobra.Command{
	Use:   "fetch <url>",
	Short: "Fetch a web page as markdown and save to vault",
	Long: `Fetch a web page via Jina Reader, convert it to markdown, and save it
to the vault's web clip folder (default: /web/). The page is saved with
frontmatter containing the title, source URL, and fetch timestamp.

Fetched pages go through the full ingestion pipeline (chunking, embedding,
wiki-link resolution) just like any other document.

Examples:
  know fetch https://example.com
  know fetch https://example.com --path /articles/custom.md
  know fetch https://example.com --vault my-vault`,
	Args: cobra.ExactArgs(1),
	RunE: runFetch,
}

func init() {
	fetchAPI = addAPIFlags(fetchCmd)
	fetchVaultID = addVaultFlag(fetchCmd, fetchAPI)
	fetchCmd.Flags().StringVar(&fetchPath, "path", "", "custom vault path (default: auto-derived from page title)")
}

func runFetch(cmd *cobra.Command, args []string) error {
	url := args[0]

	client := fetchAPI.newClient()

	req := apiclient.FetchWebpageRequest{
		URL:     url,
		VaultID: *fetchVaultID,
	}
	if fetchPath != "" {
		req.Path = &fetchPath
	}

	resp, err := client.FetchWebpage(cmd.Context(), req)
	if err != nil {
		return fmt.Errorf("fetch: %w", err)
	}

	fmt.Printf("Saved: %s → %s\n", resp.Title, resp.Path)
	return nil
}
