package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/raphi011/know/internal/apiclient"
	"github.com/raphi011/know/internal/models"
	"github.com/raphi011/know/internal/tui/browse"
	"github.com/raphi011/know/internal/tui/pick"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var (
	browseAPI     *apiFlags
	browseVaultID *string
	browseEdit    bool
	browseViewer  string
)

var browseCmd = &cobra.Command{
	Use:   "browse [path]",
	Short: "Browse, view, or edit vault documents",
	Long: `Browse vault documents with fuzzy search, view them, or edit in $EDITOR.

Without a path, launches a fuzzy finder to pick a file.
With a path, opens the file directly.

Output adapts to context: interactive terminal shows a TUI viewer,
piped output prints raw content to stdout.

Flags:
  -e, --edit      Open the selected file in $EDITOR and save changes
  --viewer CMD    Pipe content through a viewer command (e.g. bat, glow)

Environment variables:
  KNOW_SERVER_URL   Server base URL (default: http://localhost:4001)
  KNOW_TOKEN        API bearer token
  KNOW_VAULT        Default vault ID
  KNOW_VIEWER       Viewer command (alternative to --viewer flag)

Examples:
  know browse                              # fuzzy search → TUI viewer
  know browse /docs/readme.md              # TUI viewer for specific file
  know browse /docs/readme.md | head       # print raw content (piped)
  know browse --viewer bat /docs/readme.md # view with bat
  know browse -e /docs/readme.md           # edit in $EDITOR
  know browse -e                           # fuzzy pick → edit`,
	Args: cobra.MaximumNArgs(1),
	RunE: runBrowse,
}

func init() {
	browseAPI = addAPIFlags(browseCmd)
	browseVaultID = addVaultFlag(browseCmd, browseAPI)
	browseCmd.Flags().BoolVarP(&browseEdit, "edit", "e", false, "open in $EDITOR and save changes")
	browseCmd.Flags().StringVar(&browseViewer, "viewer", os.Getenv("KNOW_VIEWER"), "viewer command (env: KNOW_VIEWER)")
	browseCmd.ValidArgsFunction = completeVaultPaths(browseAPI, browseVaultID, pathFilterFiles)
}

func runBrowse(_ *cobra.Command, args []string) error {
	ctx := context.Background()
	client := browseAPI.newClient()

	if len(args) > 0 {
		return browseWithPath(ctx, client, args[0])
	}
	return browseWithPicker(ctx, client)
}

// browseWithPath handles the case where a path is given as a positional argument.
func browseWithPath(ctx context.Context, client *apiclient.Client, path string) error {
	if browseEdit {
		if !models.IsTextFile(path) {
			return fmt.Errorf("browse: %s is not a text file — only .md and .txt files can be edited", path)
		}
		doc, err := client.GetDocument(ctx, *browseVaultID, path)
		if err != nil {
			return fmt.Errorf("browse: %w", err)
		}
		return editAndSave(ctx, client, *browseVaultID, path, doc.Content)
	}

	doc, err := client.GetDocument(ctx, *browseVaultID, path)
	if err != nil {
		return fmt.Errorf("browse: %w", err)
	}

	if browseViewer != "" {
		return viewWithCommand(browseViewer, filepath.Ext(path), doc.Content)
	}

	isTTY := term.IsTerminal(int(os.Stdout.Fd()))
	if isTTY {
		isDark := lipgloss.HasDarkBackground(os.Stdin, os.Stdout)
		model := browse.NewModelWithDocument(doc, client, *browseVaultID, isDark)
		p := tea.NewProgram(model)
		if _, err := p.Run(); err != nil {
			return fmt.Errorf("browse: %w", err)
		}
		return nil
	}

	fmt.Print(doc.Content)
	return nil
}

// browseWithPicker handles the case where no path is given — launch fuzzy finder.
func browseWithPicker(ctx context.Context, client *apiclient.Client) error {
	files, err := client.ListFiles(ctx, *browseVaultID, "/", true)
	if err != nil {
		return fmt.Errorf("browse: %w", err)
	}

	// Filter out directories.
	docs := make([]models.FileEntry, 0, len(files))
	for _, f := range files {
		if f.IsDir {
			continue
		}
		if browseEdit && !models.IsTextFile(f.Name) {
			continue
		}
		docs = append(docs, f)
	}

	if len(docs) == 0 {
		if browseEdit {
			return fmt.Errorf("browse: no editable text files found in vault %q", *browseVaultID)
		}
		return fmt.Errorf("browse: no documents found in vault %q", *browseVaultID)
	}

	// --edit: pick → edit in $EDITOR
	if browseEdit {
		return pickAndDo(ctx, client, docs, func(path string, doc *apiclient.Document) error {
			return editAndSave(ctx, client, *browseVaultID, path, doc.Content)
		})
	}

	// --viewer: pick → pipe through viewer
	if browseViewer != "" {
		return pickAndDo(ctx, client, docs, func(path string, doc *apiclient.Document) error {
			return viewWithCommand(browseViewer, filepath.Ext(path), doc.Content)
		})
	}

	// Standard browse TUI.
	isDark := lipgloss.HasDarkBackground(os.Stdin, os.Stdout)
	model := browse.NewModel(client, *browseVaultID, docs, isDark)
	p := tea.NewProgram(model)
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("browse: %w", err)
	}
	return nil
}

// pickAndDo launches the fuzzy picker then runs action on the selected document.
func pickAndDo(ctx context.Context, client *apiclient.Client, docs []models.FileEntry, action func(string, *apiclient.Document) error) error {
	selected, err := pick.Run(docs)
	if err != nil {
		return fmt.Errorf("browse: %w", err)
	}
	if selected == "" {
		return nil // user cancelled
	}
	doc, err := client.GetDocument(ctx, *browseVaultID, selected)
	if err != nil {
		return fmt.Errorf("browse: %w", err)
	}
	return action(selected, doc)
}

// editAndSave opens content in $EDITOR and saves changes back to the server.
func editAndSave(ctx context.Context, client *apiclient.Client, vaultID, path, content string) error {
	updated, err := openInEditor(content, filepath.Ext(path))
	if err != nil {
		return err
	}

	if updated == content {
		fmt.Println("no changes")
		return nil
	}

	if _, err := client.EditDocument(ctx, apiclient.EditDocumentRequest{
		VaultID: vaultID,
		Path:    path,
		Content: updated,
	}); err != nil {
		return fmt.Errorf("save: %w", err)
	}

	fmt.Println("saved")
	return nil
}

// viewWithCommand pipes content through an external viewer command.
func viewWithCommand(viewer, ext, content string) error {
	tmpFile, err := os.CreateTemp("", "know-*"+ext)
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	defer func() {
		if err := os.Remove(tmpFile.Name()); err != nil {
			slog.Warn("failed to remove temp file", "path", tmpFile.Name(), "error", err)
		}
	}()

	if _, err := tmpFile.WriteString(content); err != nil {
		tmpFile.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	cmd := exec.Command("sh", "-c", viewer+" "+shellQuote(tmpFile.Name()))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run viewer: %w", err)
	}

	return nil
}

// shellQuote wraps a string in single quotes, escaping any embedded single quotes.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
