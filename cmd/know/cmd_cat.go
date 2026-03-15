package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var (
	catAPI     *apiFlags
	catVaultID *string
	catViewer  string
)

var catCmd = &cobra.Command{
	Use:   "cat <path>",
	Short: "Print a document's content",
	Long: `Fetch a document from a vault and print its content to stdout.

If --viewer is set (or KNOW_VIEWER env var), the content is displayed
through the specified viewer command (e.g. bat, glow).

Environment variables:
  KNOW_VAULT    vault name (alternative to --vault flag)
  KNOW_VIEWER   viewer command (alternative to --viewer flag)

Examples:
  know cat /docs/readme.md
  know cat /docs/readme.md --vault my-vault
  know cat /docs/readme.md --viewer "bat -l md"
  KNOW_VIEWER=glow know cat /notes/todo.md`,
	Args: cobra.ExactArgs(1),
	RunE: runCat,
}

func init() {
	catAPI = addAPIFlags(catCmd)
	catVaultID = addVaultFlag(catCmd, catAPI)
	catCmd.Flags().StringVar(&catViewer, "viewer", os.Getenv("KNOW_VIEWER"), "viewer command (env: KNOW_VIEWER)")
	catCmd.ValidArgsFunction = completeVaultPaths(catAPI, catVaultID, pathFilterFiles)
}

func runCat(_ *cobra.Command, args []string) error {
	path := args[0]

	client := catAPI.newClient()
	doc, err := client.GetDocument(context.Background(), *catVaultID, path)
	if err != nil {
		return fmt.Errorf("cat: %w", err)
	}

	if catViewer == "" {
		fmt.Print(doc.Content)
		return nil
	}

	return viewWithCommand(catViewer, filepath.Ext(path), doc.Content)
}

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
