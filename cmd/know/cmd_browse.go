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
	browseVaultID   *string
	browseViewer    string
	browseLinks     bool
	browseBookmarks bool
)

var browseCmd = &cobra.Command{
	Use:   "browse [path]",
	Short: "Browse, view, or edit vault documents",
	Long: `Browse vault documents with fuzzy search and view them in a TUI.

Without a path, launches a fuzzy finder to pick a file.
With a path, opens the file directly. Press 'e' in the viewer to edit in $EDITOR.

Output adapts to context: interactive terminal shows a TUI viewer,
piped output prints raw content to stdout.

Flags:
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
  know browse | head                       # fuzzy pick → pipe content
  know browse | wc -l                      # fuzzy pick → count lines
  know browse --viewer bat /docs/readme.md # view with bat
  know browse --links                      # browse saved web clips`,
	Args: cobra.MaximumNArgs(1),
	RunE: runBrowse,
}

func init() {
	browseVaultID = addVaultFlag(browseCmd)
	browseCmd.Flags().StringVar(&browseViewer, "viewer", os.Getenv("KNOW_VIEWER"), "viewer command (env: KNOW_VIEWER)")
	browseCmd.Flags().BoolVarP(&browseLinks, "links", "l", false, "start on the Links tab (saved web clips)")
	browseCmd.Flags().BoolVarP(&browseBookmarks, "bookmarks", "b", false, "start on the Bookmarks tab")
	browseCmd.ValidArgsFunction = completeVaultPaths(browseVaultID, pathFilterFiles)
}

func runBrowse(_ *cobra.Command, args []string) error {
	ctx := context.Background()
	client := globalAPI.newClient()

	if len(args) > 0 {
		return browseWithPath(ctx, client, args[0])
	}
	return browseWithPicker(ctx, client)
}

// browseWithPath handles the case where a path is given as a positional argument.
func browseWithPath(ctx context.Context, client *apiclient.Client, path string) error {
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
		docs = append(docs, f)
	}

	if len(docs) == 0 {
		return fmt.Errorf("browse: no documents found in vault %q", *browseVaultID)
	}

	// --viewer: pick → pipe through viewer
	if browseViewer != "" {
		return pickAndDo(ctx, client, docs, func(path string, doc *apiclient.Document) error {
			return viewWithCommand(browseViewer, filepath.Ext(path), doc.Content)
		})
	}

	// When stdout is piped, pick on stderr and pipe content to stdout.
	isTTY := term.IsTerminal(int(os.Stdout.Fd()))
	if !isTTY {
		return pickAndDo(ctx, client, docs, func(_ string, doc *apiclient.Document) error {
			fmt.Print(doc.Content)
			return nil
		}, tea.WithOutput(os.Stderr))
	}

	// Standard browse TUI.
	isDark := lipgloss.HasDarkBackground(os.Stdin, os.Stdout)
	var startTab []browse.Tab
	if browseLinks {
		startTab = append(startTab, browse.TabLinks)
	} else if browseBookmarks {
		startTab = append(startTab, browse.TabBookmarks)
	}
	model := browse.NewModel(client, *browseVaultID, docs, isDark, startTab...)
	p := tea.NewProgram(model)
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("browse: %w", err)
	}
	return nil
}

// pickAndDo launches the fuzzy picker then runs action on the selected document.
func pickAndDo(ctx context.Context, client *apiclient.Client, docs []models.FileEntry, action func(string, *apiclient.Document) error, opts ...tea.ProgramOption) error {
	selected, err := pick.Run(docs, opts...)
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
