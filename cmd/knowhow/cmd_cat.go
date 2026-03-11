package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/raphi011/knowhow/internal/apiclient"
	"github.com/spf13/cobra"
)

var (
	catVaultID string
	catViewer  string
)

var catCmd = &cobra.Command{
	Use:   "cat <path>",
	Short: "Print a document's content",
	Long: `Fetch a document from a vault and print its content to stdout.

If --viewer is set (or KNOWHOW_VIEWER env var), the content is displayed
through the specified viewer command (e.g. bat, glow).

Environment variables:
  KNOWHOW_VAULT    vault name (alternative to --vault flag)
  KNOWHOW_VIEWER   viewer command (alternative to --viewer flag)

Examples:
  knowhow cat /docs/readme.md
  knowhow cat /docs/readme.md --vault my-vault
  knowhow cat /docs/readme.md --viewer "bat -l md"
  KNOWHOW_VIEWER=glow knowhow cat /notes/todo.md`,
	Args: cobra.ExactArgs(1),
	RunE: runCat,
}

func init() {
	catCmd.Flags().StringVar(&catVaultID, "vault", envOrDefault("KNOWHOW_VAULT", "default"), "vault name (env: KNOWHOW_VAULT)")
	catCmd.Flags().StringVar(&catViewer, "viewer", os.Getenv("KNOWHOW_VIEWER"), "viewer command (env: KNOWHOW_VIEWER)")
}

func runCat(_ *cobra.Command, args []string) error {
	path := args[0]

	client := apiclient.New(apiURL, apiToken)
	doc, err := client.GetDocument(context.Background(), catVaultID, path)
	if err != nil {
		return fmt.Errorf("cat: %w", err)
	}

	if catViewer == "" {
		fmt.Print(doc.Content)
		return nil
	}

	return viewWithCommand(catViewer, filepath.Base(path), doc.Content)
}

func viewWithCommand(viewer, filename, content string) error {
	tmpFile, err := os.CreateTemp("", "knowhow-*-"+filename)
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

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
	// Replace each ' with '\'' (end quote, escaped quote, start quote)
	escaped := ""
	for _, c := range s {
		if c == '\'' {
			escaped += `'\''`
		} else {
			escaped += string(c)
		}
	}
	return "'" + escaped + "'"
}
