package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/raphi011/knowhow/internal/apiclient"
	"github.com/raphi011/knowhow/internal/models"
	"github.com/spf13/cobra"
)

var (
	cpVaultID   string
	cpLabels    []string
	cpDryRun    bool
	cpForce     bool
	cpSource    string
	cpRecursive bool
)

var cpCmd = &cobra.Command{
	Use:   "cp <local-dir> <vault-path>",
	Short: "Copy local files into a vault",
	Long: `Copy Markdown files and images from a local directory into a vault.

Unchanged files are skipped by comparing content hashes, unless --force is set.
Markdown files go through the full document pipeline (parse, embed, link, chunk).
Image files (PNG, JPEG, GIF, SVG, WebP) are uploaded as binary assets.

By default only top-level files are copied. Use -r to recurse into subdirectories.

Examples:
  knowhow cp ./docs / --vault default
  knowhow cp ./docs /imported --vault default -r
  knowhow cp ./notes /notes --vault default --labels personal,notes
  knowhow cp ./wiki /wiki --vault default --dry-run
  knowhow cp ./docs /docs --vault default --force`,
	Args: cobra.ExactArgs(2),
	RunE: runCp,
}

func init() {
	cpCmd.Flags().StringVar(&cpVaultID, "vault", "", "vault ID (required)")
	cpCmd.Flags().StringSliceVarP(&cpLabels, "labels", "l", nil, "labels to include in document path metadata")
	cpCmd.Flags().BoolVar(&cpDryRun, "dry-run", false, "show what would be copied without changes")
	cpCmd.Flags().BoolVar(&cpForce, "force", false, "overwrite existing files if content hash differs")
	cpCmd.Flags().StringVar(&cpSource, "source", "cp", "document source tag")
	cpCmd.Flags().BoolVarP(&cpRecursive, "recursive", "r", false, "recurse into subdirectories")
	if err := cpCmd.MarkFlagRequired("vault"); err != nil {
		panic(fmt.Sprintf("mark vault flag required: %v", err))
	}
}

func runCp(cmd *cobra.Command, args []string) error {
	if err := requireToken(); err != nil {
		return fmt.Errorf("cp: %w", err)
	}

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
	filePaths, skippedDirs, err := collectCpFiles(dirPath, cpRecursive)
	if err != nil {
		return fmt.Errorf("cp: %w", err)
	}
	if skippedDirs > 0 {
		fmt.Fprintf(os.Stderr, "Skipping %d subdirectories, use -r to include them\n", skippedDirs)
	}
	if len(filePaths) == 0 {
		fmt.Println("No files found")
		return nil
	}

	// Build bulk files with vault path mapping
	var bulkFiles []apiclient.BulkFile
	for _, absPath := range filePaths {
		rel, err := filepath.Rel(dirPath, absPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "✗ %s: %v\n", absPath, err)
			continue
		}

		targetPath := vaultPath
		if !strings.HasSuffix(targetPath, "/") {
			targetPath += "/"
		}
		targetPath += filepath.ToSlash(rel)

		f, err := os.Open(absPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "✗ %s: %v\n", absPath, err)
			continue
		}
		defer f.Close()

		bulkFiles = append(bulkFiles, apiclient.BulkFile{
			Path: targetPath,
			Data: f,
		})
	}

	if len(bulkFiles) == 0 {
		fmt.Println("No files to upload")
		return nil
	}

	client := apiclient.New(apiURL, apiToken)
	meta := apiclient.BulkMeta{
		VaultID: cpVaultID,
		Source:  cpSource,
		Force:   cpForce,
		DryRun:  cpDryRun,
	}

	results, err := client.BulkUpload(context.Background(), meta, bulkFiles)
	if err != nil {
		return fmt.Errorf("cp: %w", err)
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
			fmt.Fprintf(os.Stderr, "✗ %s: %s\n", r.Path, r.Error)
			errCount++
		}
	}

	if cpDryRun {
		fmt.Printf("\nDry run: %d would be created, %d would be updated, %d unchanged\n", created, updated, skipped)
	} else {
		fmt.Printf("\nDone: %d created, %d updated, %d skipped, %d errors\n", created, updated, skipped, errCount)
	}

	return nil
}

// collectCpFiles collects files from a directory. When recursive is false,
// only top-level files are returned and the number of skipped subdirectories is reported.
func collectCpFiles(dirPath string, recursive bool) (files []string, skippedDirs int, err error) {
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
