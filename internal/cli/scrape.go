package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/raphaelgruber/memcp-go/internal/client"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var (
	scrapeExtractGraph bool
	scrapeLabels       []string
	scrapeDryRun       bool
	scrapeRecursive    bool
	scrapeSync         bool
	scrapeForce        bool
)

var scrapeCmd = &cobra.Command{
	Use:   "scrape <path>",
	Short: "Ingest Markdown files from a directory",
	Long: `Scrape and ingest Markdown files from a directory into the knowledge base.

Files are parsed for frontmatter metadata, content is chunked if long,
and embeddings are generated for semantic search.

By default, unchanged files are skipped based on content hash comparison.
Use --force to re-ingest all files regardless of changes.

Use --extract-graph to also extract entity relationships using LLM.

Examples:
  knowhow scrape ./docs
  knowhow scrape ./notes --labels "personal"
  knowhow scrape ./specs --extract-graph
  knowhow scrape ./wiki --recursive --dry-run
  knowhow scrape ./docs --force  # re-ingest all files`,
	Args: cobra.ExactArgs(1),
	RunE: runScrape,
}

func init() {
	scrapeCmd.Flags().BoolVar(&scrapeExtractGraph, "extract-graph", false, "extract entity relations using LLM")
	scrapeCmd.Flags().StringSliceVarP(&scrapeLabels, "labels", "l", nil, "labels to apply to all ingested entities")
	scrapeCmd.Flags().BoolVar(&scrapeDryRun, "dry-run", false, "show what would be ingested without making changes")
	scrapeCmd.Flags().BoolVarP(&scrapeRecursive, "recursive", "r", true, "recursively process subdirectories")
	scrapeCmd.Flags().BoolVar(&scrapeSync, "sync", false, "wait for completion (default: run async with hash checking)")
	scrapeCmd.Flags().BoolVar(&scrapeForce, "force", false, "force re-ingest all files (skip hash checking)")
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

	// Sync mode with server-side file reading (legacy)
	if scrapeSync {
		result, err := gqlClient.IngestDirectory(ctx, path, opts)
		if err != nil {
			return fmt.Errorf("ingest: %w", err)
		}
		printIngestResult(result)
		return nil
	}

	// Force mode - skip hash checking, use async server-side ingestion
	if scrapeForce {
		job, err := gqlClient.IngestDirectoryAsync(ctx, path, opts)
		if err != nil {
			return fmt.Errorf("start ingest job: %w", err)
		}

		if !term.IsTerminal(int(os.Stdout.Fd())) {
			fmt.Printf("Started job %s\n", job.ID)
			fmt.Printf("  Use 'knowhow jobs %s' to check progress\n", job.ID)
			return nil
		}

		return RunJobProgress(gqlClient, job)
	}

	// Default mode: hash-based deduplication with client-side file reading
	return runScrapeWithHashCheck(ctx, path, opts)
}

// runScrapeWithHashCheck implements the two-phase hash-based ingestion protocol.
func runScrapeWithHashCheck(ctx context.Context, dirPath string, opts *client.IngestOptions) error {
	// 1. Collect files locally
	files, err := collectMarkdownFiles(dirPath, scrapeRecursive)
	if err != nil {
		return fmt.Errorf("collect files: %w", err)
	}

	if len(files) == 0 {
		fmt.Println("No Markdown files found")
		return nil
	}

	fmt.Printf("Found %d Markdown files\n", len(files))

	// 2. Compute hashes and read content locally
	type fileData struct {
		path    string
		content []byte
		hash    string
	}
	fileMap := make(map[string]fileData, len(files))
	var fileHashes []client.FileHashInput

	for _, f := range files {
		content, err := os.ReadFile(f)
		if err != nil {
			fmt.Printf("Warning: could not read %s: %v\n", f, err)
			continue
		}
		hash := sha256.Sum256(content)
		hashStr := hex.EncodeToString(hash[:])

		fileMap[f] = fileData{path: f, content: content, hash: hashStr}
		fileHashes = append(fileHashes, client.FileHashInput{
			Path: f,
			Hash: hashStr,
		})
	}

	// 3. Ask server which files are needed
	fmt.Printf("Checking for changes...\n")
	checkResult, err := gqlClient.CheckHashes(ctx, fileHashes)
	if err != nil {
		return fmt.Errorf("check hashes: %w", err)
	}

	skipped := len(fileHashes) - len(checkResult.Needed)
	if len(checkResult.Needed) == 0 {
		fmt.Printf("All %d files are up to date, nothing to ingest\n", len(fileHashes))
		return nil
	}

	fmt.Printf("  %d files unchanged (skipped)\n", skipped)
	fmt.Printf("  %d files to ingest\n", len(checkResult.Needed))

	// Dry run - just report what would be ingested
	if opts.DryRun != nil && *opts.DryRun {
		fmt.Printf("\nDry run - would ingest these files:\n")
		for _, p := range checkResult.Needed {
			fmt.Printf("  - %s\n", p)
		}
		return nil
	}

	// 4. Build list of files to upload
	var filesToUpload []client.FileContentInput
	for _, p := range checkResult.Needed {
		data, ok := fileMap[p]
		if !ok {
			continue
		}
		filesToUpload = append(filesToUpload, client.FileContentInput{
			Path:    data.path,
			Content: string(data.content),
			Hash:    data.hash,
		})
	}

	// 5. Send to server for processing
	fmt.Printf("Uploading %d files...\n", len(filesToUpload))
	result, err := gqlClient.IngestFiles(ctx, filesToUpload, opts)
	if err != nil {
		return fmt.Errorf("ingest files: %w", err)
	}

	// Override skipped count with our local calculation
	result.FilesSkipped = skipped
	printIngestResult(result)
	return nil
}

// collectMarkdownFiles walks a directory and returns all markdown file paths.
func collectMarkdownFiles(dirPath string, recursive bool) ([]string, error) {
	var files []string
	walkFn := func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && !recursive && path != dirPath {
			return filepath.SkipDir
		}
		ext := strings.ToLower(filepath.Ext(path))
		if !d.IsDir() && (ext == ".md" || ext == ".markdown") {
			files = append(files, path)
		}
		return nil
	}

	if err := filepath.WalkDir(dirPath, walkFn); err != nil {
		return nil, fmt.Errorf("scan directory: %w", err)
	}
	return files, nil
}

func printIngestResult(result *client.IngestResult) {
	if scrapeDryRun {
		fmt.Printf("Dry run - would ingest %d files\n", result.FilesProcessed)
	} else {
		fmt.Printf("Ingested %d files", result.FilesProcessed)
		if result.FilesSkipped > 0 {
			fmt.Printf(" (%d skipped)", result.FilesSkipped)
		}
		fmt.Println()
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
}
