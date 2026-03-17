package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
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
	importNoIgnore  bool
)

var importCmd = &cobra.Command{
	Use:   "import <source> [vault-path]",
	Short: "Import local files or an export archive into a vault",
	Long: `Import files from a local path or export archive into a vault.

The source can be a single file, a directory, or a .tar.gz archive created by 'know export'.
If vault-path is omitted, files are imported to / (root).

Unchanged files are skipped by comparing content hashes, unless --force is set.
Markdown files go through the full document pipeline (parse, embed, link, chunk).
Image files (PNG, JPEG, GIF, SVG, WebP) are uploaded as binary assets.
Audio files (MP3, WAV, M4A, OGG, FLAC, etc.) are uploaded as binary assets
and transcribed if STT is configured.

By default only top-level files are imported from directories. Use -r to recurse
into subdirectories. Archives are always imported recursively.

When importing from a git repository, .gitignore rules are respected automatically.
Files and directories ignored by git are skipped. When importing from a non-git
directory, dotfiles (e.g. .DS_Store) and dot-directories (e.g. .obsidian/) are skipped.
Use --no-ignore to disable all filtering and import everything.

Examples:
  know import ./speech.mp3 / --vault default
  know import ./docs --vault default -r
  know import ./docs /imported --vault default -r
  know import ./notes /notes --vault default --labels personal,notes
  know import ./export.tar.gz --vault default
  know import ./export.tar.gz /restored --vault default --dry-run
  know import ./docs /docs --vault default --force --yes
  know import ./vault /vault --vault default -r --no-ignore`,
	Args: cobra.RangeArgs(1, 2),
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
	importCmd.Flags().BoolVar(&importNoIgnore, "no-ignore", false, "import all files, ignoring .gitignore rules and dotfile filtering")
	if err := importCmd.MarkFlagRequired("vault"); err != nil {
		panic(fmt.Sprintf("mark vault flag required: %v", err))
	}
	if err := importCmd.RegisterFlagCompletionFunc("vault", completeVaultNames(importAPI)); err != nil {
		panic(fmt.Sprintf("register vault completion: %v", err))
	}
	importCmd.ValidArgsFunction = func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		switch len(args) {
		case 0:
			// First arg is local filesystem path
			return nil, cobra.ShellCompDirectiveDefault
		case 1:
			// Second arg is vault path
			return completeVaultPaths(importAPI, &importVaultID, pathFilterFolders)(cmd, args, toComplete)
		default:
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
	}
}

// fileMapping describes a file to import with its source label and target vault path.
type fileMapping struct {
	sourceLabel string // display path (abs path for dirs, archive entry for archives)
	targetPath  string
}

func runImport(cmd *cobra.Command, args []string) error {
	sourcePath := args[0]
	vaultPath := "/"
	if len(args) >= 2 {
		vaultPath = args[1]
	}

	if !strings.HasPrefix(vaultPath, "/") {
		return fmt.Errorf("vault path must start with /: %s", vaultPath)
	}

	info, err := os.Stat(sourcePath)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}

	ctx := cmd.Context()

	if info.IsDir() {
		return importFromDirectory(ctx, sourcePath, vaultPath)
	}
	if isArchiveFile(sourcePath) {
		return importFromArchive(ctx, sourcePath, vaultPath)
	}
	if info.Mode().IsRegular() {
		return importSingleFile(ctx, sourcePath, vaultPath)
	}
	return fmt.Errorf("source must be a file, directory, or .tar.gz archive: %s", sourcePath)
}

func importSingleFile(ctx context.Context, sourcePath, vaultPath string) error {
	if !isSupportedFile(sourcePath) {
		return fmt.Errorf("unsupported file type %s (supported: .md, .markdown, images, audio)", filepath.Ext(sourcePath))
	}

	// If vault path ends with /, append the filename
	targetPath := vaultPath
	if strings.HasSuffix(targetPath, "/") {
		targetPath += filepath.Base(sourcePath)
	}

	if importDryRun {
		fmt.Printf("  %s -> %s\n", sourcePath, targetPath)
		fmt.Printf("\nDry run: 1 file would be imported to vault %s\n", importVaultID)
		return nil
	}

	f, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	bulkFiles := []apiclient.BulkFile{{Path: targetPath, Data: f}}
	return uploadAndPrintResults(ctx, bulkFiles, 0)
}

func importFromDirectory(ctx context.Context, dirPath, vaultPath string) error {
	filePaths, skippedDirs, skippedArchives, err := collectImportFiles(dirPath, importRecursive)
	if err != nil {
		return fmt.Errorf("import: %w", err)
	}
	if skippedDirs > 0 {
		fmt.Fprintf(os.Stderr, "Skipping %d subdirectories, use -r to include them\n", skippedDirs)
	}
	if skippedArchives > 0 {
		fmt.Fprintf(os.Stderr, "Note: skipped %d archive file(s) (use 'know import <file>' to import archives)\n", skippedArchives)
	}
	if len(filePaths) == 0 {
		fmt.Println("No files found")
		return nil
	}

	// Build target path list
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

		mappings = append(mappings, fileMapping{sourceLabel: absPath, targetPath: targetPath})
	}

	if len(mappings) == 0 {
		fmt.Println("No files to import")
		return nil
	}

	if importDryRun {
		for _, m := range mappings {
			fmt.Printf("  %s -> %s\n", m.sourceLabel, m.targetPath)
		}
		fmt.Printf("\nDry run: %d files would be imported to %s in vault %s\n", len(mappings), vaultPath, importVaultID)
		return nil
	}

	if !importYes {
		if !confirmPrompt(fmt.Sprintf("%d files will be imported to %s in vault %s. Proceed? [y/N] ", len(mappings), vaultPath, importVaultID)) {
			fmt.Println("Aborted.")
			return nil
		}
	}

	// Open files and build bulk upload list
	var bulkFiles []apiclient.BulkFile
	var openFiles []*os.File
	defer func() {
		for _, f := range openFiles {
			if err := f.Close(); err != nil {
				fmt.Fprintf(os.Stderr, "warning: close %s: %v\n", f.Name(), err)
			}
		}
	}()

	for _, m := range mappings {
		f, err := os.Open(m.sourceLabel)
		if err != nil {
			fmt.Fprintf(os.Stderr, "- %s: %v\n", m.sourceLabel, err)
			localErrors++
			continue
		}
		openFiles = append(openFiles, f)
		bulkFiles = append(bulkFiles, apiclient.BulkFile{
			Path: m.targetPath,
			Data: f,
		})
	}

	return uploadAndPrintResults(ctx, bulkFiles, localErrors)
}

func importFromArchive(ctx context.Context, archivePath, vaultPath string) error {
	if importRecursive {
		fmt.Fprintf(os.Stderr, "Note: -r is ignored for archive imports\n")
	}

	entries, skippedFiles, err := collectArchiveEntries(archivePath)
	if err != nil {
		return fmt.Errorf("import: %w", err)
	}
	if skippedFiles > 0 {
		fmt.Fprintf(os.Stderr, "Note: skipped %d unsupported file(s) in archive\n", skippedFiles)
	}
	if len(entries) == 0 {
		fmt.Println("No supported files found in archive")
		return nil
	}

	// Build mappings
	var mappings []fileMapping
	for _, e := range entries {
		targetPath := vaultPath
		if !strings.HasSuffix(targetPath, "/") {
			targetPath += "/"
		}
		targetPath += e.path

		mappings = append(mappings, fileMapping{sourceLabel: e.path, targetPath: targetPath})
	}

	if importDryRun {
		for _, m := range mappings {
			fmt.Printf("  %s -> %s\n", m.sourceLabel, m.targetPath)
		}
		fmt.Printf("\nDry run: %d files would be imported from archive to %s in vault %s\n", len(mappings), vaultPath, importVaultID)
		return nil
	}

	if !importYes {
		if !confirmPrompt(fmt.Sprintf("%d files will be imported from archive to %s in vault %s. Proceed? [y/N] ", len(mappings), vaultPath, importVaultID)) {
			fmt.Println("Aborted.")
			return nil
		}
	}

	// Build bulk file list from archive entries
	var bulkFiles []apiclient.BulkFile
	for i, e := range entries {
		bulkFiles = append(bulkFiles, apiclient.BulkFile{
			Path: mappings[i].targetPath,
			Data: bytes.NewReader(e.data),
		})
	}

	return uploadAndPrintResults(ctx, bulkFiles, 0)
}

// uploadAndPrintResults uploads files via the bulk API and prints per-file results.
func uploadAndPrintResults(ctx context.Context, bulkFiles []apiclient.BulkFile, localErrors int) error {
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

	results, err := client.BulkUpload(ctx, meta, bulkFiles)
	if err != nil {
		return fmt.Errorf("import: %w", err)
	}

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

// maxArchiveEntrySize is the maximum size of a single file in an archive (100 MB).
const maxArchiveEntrySize = 100 << 20

// archiveEntry holds a file extracted from a tar.gz archive.
type archiveEntry struct {
	path string // relative path within the archive
	data []byte
}

// collectArchiveEntries reads a tar.gz archive and returns supported file entries.
// Returns the entries and the number of skipped (unsupported) files.
func collectArchiveEntries(archivePath string) ([]archiveEntry, int, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return nil, 0, fmt.Errorf("open archive: %w", err)
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		return nil, 0, fmt.Errorf("read gzip: %w", err)
	}

	tr := tar.NewReader(gr)
	var entries []archiveEntry
	var skipped int

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, 0, fmt.Errorf("read tar entry: %w", err)
		}

		// Only process regular files.
		if hdr.Typeflag != tar.TypeReg && hdr.Typeflag != tar.TypeRegA {
			continue
		}

		// Sanitize path to prevent traversal attacks.
		clean := filepath.ToSlash(filepath.Clean(hdr.Name))
		if strings.HasPrefix(clean, "/") || strings.HasPrefix(clean, "..") {
			return nil, 0, fmt.Errorf("archive contains unsafe path: %s", hdr.Name)
		}

		if !isSupportedFile(clean) {
			skipped++
			continue
		}

		data, err := io.ReadAll(io.LimitReader(tr, maxArchiveEntrySize+1))
		if err != nil {
			return nil, 0, fmt.Errorf("read tar entry %s: %w", clean, err)
		}
		if len(data) > maxArchiveEntrySize {
			return nil, 0, fmt.Errorf("tar entry %s exceeds maximum size of %s", clean, formatBytes(int64(maxArchiveEntrySize)))
		}

		entries = append(entries, archiveEntry{
			path: clean,
			data: data,
		})
	}

	// Close verifies the gzip checksum — a failure means the archive may be corrupted.
	if err := gr.Close(); err != nil {
		return nil, 0, fmt.Errorf("verify archive integrity: %w", err)
	}

	return entries, skipped, nil
}

// collectImportFiles collects files from a directory, respecting ignore rules.
//
// When importNoIgnore is false (default), it tries git-based discovery first:
// if the directory is inside a git repo, it uses `git ls-files` to list only
// tracked and untracked-but-not-ignored files, respecting .gitignore rules.
// If git is unavailable or the directory is not a git repo, it falls back to
// filesystem walking with dot-prefix filtering (skipping .dotfiles and .dotdirs).
//
// When importNoIgnore is true, all files are included (only extension and archive filtering apply).
func collectImportFiles(dirPath string, recursive bool) (files []string, skippedDirs, skippedArchives int, err error) {
	if !importNoIgnore {
		gitFiles, gitErr := collectGitFiles(dirPath, recursive)
		if gitErr == nil {
			fmt.Fprintf(os.Stderr, "Using .gitignore rules (use --no-ignore to include all files)\n")
			// git ls-files doesn't distinguish dirs/archives — filter out archives from results.
			var filtered []string
			for _, f := range gitFiles {
				if isArchiveFile(f) {
					skippedArchives++
					continue
				}
				filtered = append(filtered, f)
			}
			return filtered, 0, skippedArchives, nil
		}
		// Not a git repo or git unavailable — fall back to dotfile filtering.
		fmt.Fprintf(os.Stderr, "Skipping dotfiles and dot-directories (use --no-ignore to include all files)\n")
	}

	return collectWalkFiles(dirPath, recursive, !importNoIgnore)
}

// collectGitFiles uses `git ls-files` to collect files from a git repository,
// respecting .gitignore rules. Returns an error if the directory is not in a git
// repo or git is not available.
func collectGitFiles(dirPath string, recursive bool) ([]string, error) {
	absDir, err := filepath.Abs(dirPath)
	if err != nil {
		return nil, fmt.Errorf("resolve path: %w", err)
	}

	// Resolve symlinks so absDir matches the repo root from git (which resolves
	// symlinks). On macOS, /tmp → /private/tmp causes mismatches otherwise.
	absDir, err = filepath.EvalSymlinks(absDir)
	if err != nil {
		return nil, fmt.Errorf("resolve symlinks: %w", err)
	}

	// Verify this is inside a git repo.
	check := exec.Command("git", "-C", absDir, "rev-parse", "--show-toplevel")
	if err := check.Run(); err != nil {
		return nil, fmt.Errorf("not a git repo: %w", err)
	}

	// List tracked + untracked-but-not-ignored files.
	// With -C absDir and absDir as pathspec, output is relative to absDir.
	cmd := exec.Command("git", "-C", absDir, "ls-files", "--cached", "--others", "--exclude-standard", absDir)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git ls-files: %w", err)
	}

	var files []string
	for line := range strings.SplitSeq(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// git ls-files with -C absDir returns paths relative to absDir.
		absPath := line
		if !filepath.IsAbs(line) {
			absPath = filepath.Join(absDir, line)
		}

		// Skip files that no longer exist on disk (e.g. tracked but deleted).
		if _, err := os.Stat(absPath); err != nil {
			continue
		}

		if !isSupportedFile(absPath) {
			continue
		}

		if !recursive {
			// Only include direct children of dirPath.
			rel, err := filepath.Rel(absDir, absPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: skipping %s: %v\n", absPath, err)
				continue
			}
			if strings.Contains(rel, string(filepath.Separator)) {
				continue
			}
		}

		files = append(files, absPath)
	}

	return files, nil
}

// collectWalkFiles collects files by walking the filesystem. When skipDot is true,
// directories and files starting with "." are skipped.
func collectWalkFiles(dirPath string, recursive, skipDot bool) (files []string, skippedDirs, skippedArchives int, err error) {
	if !recursive {
		entries, err := os.ReadDir(dirPath)
		if err != nil {
			return nil, 0, 0, fmt.Errorf("read directory: %w", err)
		}
		for _, e := range entries {
			if skipDot && strings.HasPrefix(e.Name(), ".") {
				if e.IsDir() {
					skippedDirs++
				}
				continue
			}
			if e.IsDir() {
				skippedDirs++
				continue
			}
			if isArchiveFile(e.Name()) {
				skippedArchives++
				continue
			}
			if isSupportedFile(e.Name()) {
				files = append(files, filepath.Join(dirPath, e.Name()))
			}
		}
		return files, skippedDirs, skippedArchives, nil
	}

	err = filepath.WalkDir(dirPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDot && d.Name() != "." && strings.HasPrefix(d.Name(), ".") {
				skippedDirs++
				return filepath.SkipDir
			}
			return nil
		}
		if skipDot && strings.HasPrefix(d.Name(), ".") {
			return nil
		}
		if isArchiveFile(path) {
			skippedArchives++
			return nil
		}
		if isSupportedFile(path) {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, 0, 0, fmt.Errorf("scan directory: %w", err)
	}
	return files, skippedDirs, skippedArchives, nil
}

func isSupportedFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".md" || ext == ".markdown" || models.IsImageFile(path) || models.IsAudioFile(path)
}

func isArchiveFile(path string) bool {
	lower := strings.ToLower(path)
	return strings.HasSuffix(lower, ".tar.gz") || strings.HasSuffix(lower, ".tgz")
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
