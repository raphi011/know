package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/raphi011/know/internal/apiclient"
	"github.com/raphi011/know/internal/file"
	"github.com/raphi011/know/internal/tui"
	"github.com/spf13/cobra"
)

var (
	noteAPI     *apiFlags
	noteVaultID *string
	noteDate    string
	noteEdit    bool
)

var noteCmd = &cobra.Command{
	Use:   "note [message]",
	Short: "Add to today's daily note",
	Long: `Add an entry to today's daily note or open it in an editor.

Modes:
  know note "message"        Agent appends entry to today's note
  know note -e               Open today's note in $EDITOR
  know note -e "message"     Agent drafts, then open in editor for review
  know note                  Print today's note to stdout

The daily note is created automatically on first use with a daily-note label.
Folder paths are configured via vault settings (see: know vault settings).

Environment variables:
  KNOW_VAULT    vault name (alternative to --vault flag)`,
	Args: cobra.MaximumNArgs(1),
	RunE: runNote,
}

func init() {
	noteAPI = addAPIFlags(noteCmd)
	noteVaultID = addVaultFlag(noteCmd, noteAPI)
	noteCmd.Flags().StringVarP(&noteDate, "date", "d", "", "target date in YYYY-MM-DD format (default: today)")
	noteCmd.Flags().BoolVarP(&noteEdit, "edit", "e", false, "open daily note in $EDITOR")
}

func runNote(_ *cobra.Command, args []string) error {
	// Parse date.
	date := noteDate
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}
	if _, err := time.Parse("2006-01-02", date); err != nil {
		return fmt.Errorf("invalid date %q (expected YYYY-MM-DD): %w", date, err)
	}

	ctx := context.Background()
	client := noteAPI.newClient()

	// Fetch folder paths from vault settings.
	settings, err := client.GetVaultSettings(ctx, *noteVaultID)
	if err != nil {
		return fmt.Errorf("fetch vault settings: %w", err)
	}

	// Normalize folder path.
	folder := settings.DailyNotePath
	if !strings.HasPrefix(folder, "/") {
		folder = "/" + folder
	}
	if !strings.HasSuffix(folder, "/") {
		folder = folder + "/"
	}

	docPath := folder + date + ".md"
	prompt := ""
	if len(args) > 0 {
		prompt = args[0]
	}

	// Ensure daily note exists.
	doc, err := ensureDailyNote(ctx, client, *noteVaultID, docPath, date, settings.TemplatePath)
	if err != nil {
		return err
	}

	switch {
	case prompt == "" && !noteEdit:
		// Mode A: print note to stdout.
		fmt.Print(doc.Content)
		return nil

	case prompt != "" && !noteEdit:
		// Mode B: agent oneshot.
		agentClient := tui.NewClient(noteAPI.URL, noteAPI.Token)
		return agentAppend(ctx, agentClient, docPath, prompt)

	case prompt == "" && noteEdit:
		// Mode C: editor only.
		return editAndSave(ctx, client, *noteVaultID, docPath, doc.Content)

	default:
		// Mode D: agent draft then editor.
		agentClient := tui.NewClient(noteAPI.URL, noteAPI.Token)
		if err := agentAppend(ctx, agentClient, docPath, prompt); err != nil {
			return err
		}
		updated, err := client.GetDocument(ctx, *noteVaultID, docPath)
		if err != nil {
			return fmt.Errorf("fetch updated note: %w", err)
		}
		return editAndSave(ctx, client, *noteVaultID, docPath, updated.Content)
	}
}

// ensureDailyNote fetches the daily note or creates it with a template if it doesn't exist.
// It tries to read a "daily-note" template from the template folder for the initial content,
// falling back to a hardcoded default if no template is found.
func ensureDailyNote(ctx context.Context, client *apiclient.Client, vaultID, path, date, templatePath string) (*apiclient.Document, error) {
	doc, err := client.GetDocument(ctx, vaultID, path)
	if err == nil {
		return doc, nil
	}

	var httpErr *apiclient.HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != 404 {
		return nil, fmt.Errorf("check daily note: %w", err)
	}

	content, err := dailyNoteContent(ctx, client, vaultID, date, templatePath)
	if err != nil {
		return nil, fmt.Errorf("daily note content: %w", err)
	}
	doc, err = client.CreateDocument(ctx, apiclient.CreateDocumentRequest{
		VaultID: vaultID,
		Path:    path,
		Content: content,
	})
	if err != nil {
		return nil, fmt.Errorf("create daily note: %w", err)
	}

	return doc, nil
}

// dailyNoteContent returns the initial content for a new daily note.
// Tries the template folder first, falls back to a hardcoded default if no template exists.
// Returns an error for non-404 failures (network, auth, server errors).
func dailyNoteContent(ctx context.Context, client *apiclient.Client, vaultID, date, templatePath string) (string, error) {
	tplFolder := strings.TrimSuffix(templatePath, "/")
	tplPath := tplFolder + "/daily-note.md"

	tplDoc, err := client.GetDocument(ctx, vaultID, tplPath)
	if err != nil {
		var httpErr *apiclient.HTTPError
		if !errors.As(err, &httpErr) || httpErr.StatusCode != 404 {
			return "", fmt.Errorf("fetch daily-note template: %w", err)
		}
		// Template doesn't exist — use hardcoded default.
		return fmt.Sprintf("---\ntitle: %q\nlabels:\n  - daily-note\n---\n# %s\n", date, date), nil
	}

	now, err := time.Parse("2006-01-02", date)
	if err != nil {
		return "", fmt.Errorf("parse date for template vars: %w", err)
	}
	vars := file.DefaultTemplateVars(now, date, vaultID)
	return file.ApplyTemplateVars(tplDoc.Content, vars), nil
}

// agentAppend sends a prompt to the agent to update the daily note.
func agentAppend(ctx context.Context, agentClient *tui.Client, path, message string) error {
	prompt := fmt.Sprintf(
		"Update the daily note at %s. Add the following entry:\n\n%s\n\nKeep all existing content intact. Add a timestamp (%s) prefix to the new entry.",
		path, message, time.Now().Format("15:04"),
	)

	events, err := agentClient.Chat(ctx, "", *noteVaultID, prompt, nil, true)
	if err != nil {
		return fmt.Errorf("start agent: %w", err)
	}

	return streamToStdout(events)
}

// streamToStdout prints agent text events to stdout until the stream ends.
func streamToStdout(events <-chan tui.StreamEvent) error {
	for event := range events {
		switch event.Type {
		case "text":
			fmt.Print(event.Content)
		case "error":
			return fmt.Errorf("agent: %s", event.Content)
		case "interrupted":
			return fmt.Errorf("agent: interrupted")
		case "msg_end":
			fmt.Println()
			return nil
		}
	}
	return fmt.Errorf("agent: stream ended unexpectedly")
}

// openInEditor writes content to a temp file, opens $EDITOR, and returns the edited content.
// ext is the file extension including the dot (e.g. ".md", ".txt").
func openInEditor(content, ext string) (string, error) {
	tmpFile, err := os.CreateTemp("", "know-*"+ext)
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer func() {
		if err := os.Remove(tmpPath); err != nil {
			slog.Warn("failed to remove temp file", "path", tmpPath, "error", err)
		}
	}()

	if _, err := tmpFile.WriteString(content); err != nil {
		tmpFile.Close()
		return "", fmt.Errorf("write temp file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return "", fmt.Errorf("close temp file: %w", err)
	}

	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}
	if _, err := exec.LookPath(editor); err != nil {
		return "", fmt.Errorf("editor %q not found: set $EDITOR to your preferred editor", editor)
	}

	cmd := exec.Command(editor, tmpPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("run editor: %w", err)
	}

	data, err := os.ReadFile(tmpPath)
	if err != nil {
		return "", fmt.Errorf("read edited file: %w", err)
	}

	return string(data), nil
}
