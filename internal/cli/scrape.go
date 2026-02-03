package cli

import (
	"fmt"
	"os"
	"path/filepath"

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

	// Verify path exists
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("path must be a directory: %s", path)
	}

	// Find markdown files
	var files []string
	walkFn := func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && !scrapeRecursive && p != path {
			return filepath.SkipDir
		}
		if !d.IsDir() && (filepath.Ext(p) == ".md" || filepath.Ext(p) == ".markdown") {
			files = append(files, p)
		}
		return nil
	}

	if err := filepath.WalkDir(path, walkFn); err != nil {
		return fmt.Errorf("scan directory: %w", err)
	}

	if len(files) == 0 {
		fmt.Println("No Markdown files found.")
		return nil
	}

	fmt.Printf("Found %d Markdown files\n", len(files))

	if scrapeDryRun {
		fmt.Println("\nDry run - would ingest:")
		for _, f := range files {
			fmt.Printf("  %s\n", f)
		}
		return nil
	}

	// TODO: Implement actual ingestion with:
	// - Markdown parsing (frontmatter, headings)
	// - Chunking for long content
	// - Embedding generation
	// - Entity creation
	// - Optional graph extraction

	fmt.Println("\nIngestion not yet implemented. Files found:")
	for _, f := range files {
		fmt.Printf("  %s\n", f)
	}

	return nil
}
