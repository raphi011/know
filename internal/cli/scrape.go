package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/raphaelgruber/memcp-go/internal/client"
	"github.com/spf13/cobra"
)

var (
	scrapeExtractGraph bool
	scrapeLabels       []string
	scrapeDryRun       bool
	scrapeRecursive    bool
)

var scrapeCmd = &cobra.Command{
	Use:   "scrape <path>",
	Short: "Ingest Markdown files from a directory",
	Long: `Scrape and ingest Markdown files from a directory into the knowledge base.

Files are parsed for frontmatter metadata, content is chunked if long,
and embeddings are generated for semantic search.

Use --extract-graph to also extract entity relationships using LLM.

Examples:
  knowhow scrape ./docs
  knowhow scrape ./notes --labels "personal"
  knowhow scrape ./specs --extract-graph
  knowhow scrape ./wiki --recursive --dry-run`,
	Args: cobra.ExactArgs(1),
	RunE: runScrape,
}

func init() {
	scrapeCmd.Flags().BoolVar(&scrapeExtractGraph, "extract-graph", false, "extract entity relations using LLM")
	scrapeCmd.Flags().StringSliceVarP(&scrapeLabels, "labels", "l", nil, "labels to apply to all ingested entities")
	scrapeCmd.Flags().BoolVar(&scrapeDryRun, "dry-run", false, "show what would be ingested without making changes")
	scrapeCmd.Flags().BoolVarP(&scrapeRecursive, "recursive", "r", true, "recursively process subdirectories")
}

func runScrape(cmd *cobra.Command, args []string) error {
	path := args[0]
	ctx := context.Background()

	// Verify path exists
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("path must be a directory: %s", path)
	}

	opts := &client.IngestOptions{
		Labels:       scrapeLabels,
		ExtractGraph: &scrapeExtractGraph,
		DryRun:       &scrapeDryRun,
		Recursive:    &scrapeRecursive,
	}

	result, err := gqlClient.IngestDirectory(ctx, path, opts)
	if err != nil {
		return fmt.Errorf("ingest: %w", err)
	}

	// Report results
	if scrapeDryRun {
		fmt.Printf("Dry run - would ingest %d files\n", result.FilesProcessed)
	} else {
		fmt.Printf("Ingested %d files\n", result.FilesProcessed)
		fmt.Printf("  Entities created: %d\n", result.EntitiesCreated)
		fmt.Printf("  Chunks created: %d\n", result.ChunksCreated)
		if result.RelationsCreated > 0 {
			fmt.Printf("  Relations created: %d\n", result.RelationsCreated)
		}
	}

	if len(result.Errors) > 0 {
		fmt.Printf("\nErrors (%d):\n", len(result.Errors))
		for _, e := range result.Errors {
			fmt.Printf("  - %s\n", e)
		}
	}

	return nil
}
