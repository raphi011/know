package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/raphi011/know/internal/apiclient"
	"github.com/raphi011/know/internal/models"
	"github.com/spf13/cobra"
)

var (
	importAPI       *apiFlags
	importVaultID   string
	importLabels    []string
	importDryRun    bool
	importForce     bool
	importRecursive bool
	importYes       bool
)

var importCmd = &cobra.Command{
	Use:   "import <local-dir> <vault-path>",
	Short: "Import local files into a vault",
	Long: `Import Markdown files and images from a local directory into a vault.

Unchanged files are skipped by comparing content hashes, unless --force is set.
Markdown files go through the full document pipeline (parse, embed, link, chunk).
Image files (PNG, JPEG, GIF, SVG, WebP) are uploaded as binary assets.

By default only top-level files are imported. Use -r to recurse into subdirectories.

Examples:
  know import ./docs / --vault default
  know import ./docs /imported --vault default -r
  know import ./notes /notes --vault default --labels personal,notes
  know import ./wiki /wiki --vault default --dry-run
  know import ./docs /docs --vault default --force
  know import ./docs /docs --vault default --yes`,
	Args: cobra.ExactArgs(2),
	RunE: runImport,
}

func init() {
	importAPI = addAPIFlags(importCmd)
	// import requires an explicit vault (no default) unlike other commands that use addVaultFlag.
	importCmd.Flags().StringVar(&importVaultID, "vault", "", "vault ID (required)")
	importCmd.Flags().StringSliceVarP(&importLabels, "labels", "l", nil, "labels to include in document path metadata")
	importCmd.Flags().BoolVar(&importDryRun, "dry-run", false, "show what would be imported without changes")
	importCmd.Flags().BoolVar(&importForce, "force", false, "overwrite existing files if content hash differs")
	importCmd.Flags().BoolVarP(&importRecursive, "recursive", "r", false, "recurse into subdirectories")
	importCmd.Flags().BoolVarP(&importYes, "yes", "y", false, "skip confirmation prompt")
	if err := importCmd.MarkFlagRequired("vault"); err != nil {
		panic(fmt.Sprintf("mark vault flag required: %v", err))
	}
}

func runImport(_ *cobra.Command, args []string) error {
	dirPath := args[0]
	vaultPath := args[1]

	info, err := os.Stat(dirPath)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("path must be a directory: %s", dirPath)
	}
	if !strings.HasPrefix(vaultPath, "/") {
		return fmt.Errorf("vault path must start with /: %s", vaultPath)
	}

	// Collect local files
	filePaths, skippedDirs, err := collectImportFiles(dirPath, importRecursive)
	if err != nil {
		return fmt.Errorf("import: %w", err)
	}
	if skippedDirs > 0 {
		fmt.Fprintf(os.Stderr, "Skipping %d subdirectories, use -r to include them\n", skippedDirs)
	}
	if len(filePaths) == 0 {
		fmt.Println("No files found")
		return nil
	}

	// Build target path list for confirmation/dry-run display
	type fileMapping struct {
		absPath    string
		targetPath string
	}
	var mappings []fileMapping
	var localErrors int
	for _, absPath := range filePaths {
		rel, err := filepath.Rel(dirPath, absPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "- %s: %v\n", absPath, err)
			localErrors++
			continue
		}

		targetPath := vaultPath
		if !strings.HasSuffix(targetPath, "/") {
			targetPath += "/"
		}
		targetPath += filepath.ToSlash(rel)

		mappings = append(mappings, fileMapping{absPath: absPath, targetPath: targetPath})
	}

	if len(mappings) == 0 {
		fmt.Println("No files to import")
		return nil
	}

	// Dry-run: show full file list and exit
	if importDryRun {
		for _, m := range mappings {
			fmt.Printf("  %s -> %s\n", m.absPath, m.targetPath)
		}
		fmt.Printf("\nDry run: %d files would be imported to %s in vault %s\n", len(mappings), vaultPath, importVaultID)
		return nil
	}

	// Confirmation prompt (unless --yes)
	if !importYes {
		if !confirmPrompt(fmt.Sprintf("%d files will be imported to %s in vault %s. Proceed? [y/N] ", len(mappings), vaultPath, importVaultID)) {
			fmt.Println("Aborted.")
			return nil
		}
	}

	// Open files and build bulk upload list (streaming, not buffered)
	var bulkFiles []apiclient.BulkFile
	var openFiles []*os.File
	defer func() {
		for _, f := range openFiles {
			f.Close()
		}
	}()

	for _, m := range mappings {
		f, err := os.Open(m.absPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "- %s: %v\n", m.absPath, err)
			localErrors++
			continue
		}
		openFiles = append(openFiles, f)
		bulkFiles = append(bulkFiles, apiclient.BulkFile{
			Path: m.targetPath,
			Data: f,
		})
	}

	if len(bulkFiles) == 0 {
		fmt.Println("No files to upload")
		return nil
	}

	client := importAPI.newClient()
	meta := apiclient.BulkMeta{
		VaultID: importVaultID,
		Force:   importForce,
		DryRun:  false,
	}

	results, err := client.BulkUpload(context.Background(), meta, bulkFiles)
	if err != nil {
		return fmt.Errorf("import: %w", err)
	}

	// Print per-file results
	var created, updated, skipped, errCount int
	for _, r := range results {
		switch r.Status {
		case "created":
			fmt.Printf("+ %s\n", r.Path)
			created++
		case "updated":
			fmt.Printf("~ %s\n", r.Path)
			updated++
		case "skipped":
			fmt.Printf("= %s (%s)\n", r.Path, r.Reason)
			skipped++
		case "error":
			fmt.Fprintf(os.Stderr, "- %s: %s\n", r.Path, r.Error)
			errCount++
		}
	}

	totalErrors := errCount + localErrors
	fmt.Printf("\nDone: %d created, %d updated, %d skipped, %d errors\n", created, updated, skipped, totalErrors)

	return nil
}

// collectImportFiles collects files from a directory. When recursive is false,
// only top-level files are returned and the number of skipped subdirectories is reported.
func collectImportFiles(dirPath string, recursive bool) (files []string, skippedDirs int, err error) {
	if !recursive {
		entries, err := os.ReadDir(dirPath)
		if err != nil {
			return nil, 0, fmt.Errorf("read directory: %w", err)
		}
		for _, e := range entries {
			if e.IsDir() {
				skippedDirs++
				continue
			}
			if isSupportedFile(e.Name()) {
				files = append(files, filepath.Join(dirPath, e.Name()))
			}
		}
		return files, skippedDirs, nil
	}

	err = filepath.WalkDir(dirPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if isSupportedFile(path) {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, 0, fmt.Errorf("scan directory: %w", err)
	}
	return files, 0, nil
}

func isSupportedFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".md" || ext == ".markdown" || models.IsImageFile(path)
}

func confirmPrompt(msg string) bool {
	fmt.Print(msg)
	var answer string
	if _, err := fmt.Scanln(&answer); err != nil {
		fmt.Fprintf(os.Stderr, "failed to read input (use --yes to skip): %v\n", err)
		return false
	}
	return strings.EqualFold(strings.TrimSpace(answer), "y")
}
