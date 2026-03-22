package browse

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"

	tea "charm.land/bubbletea/v2"
	"github.com/raphi011/know/internal/apiclient"
)

// editorFinishedMsg is sent after the editor process exits.
type editorFinishedMsg struct {
	path    string
	content string // updated content
	changed bool   // true if content was modified and saved
	err     error
}

// editorErr returns a Cmd that sends an editorFinishedMsg with an error.
func editorErr(path string, err error) tea.Cmd {
	return func() tea.Msg {
		return editorFinishedMsg{path: path, err: err}
	}
}

// openInEditor launches $EDITOR with the document content and returns an
// ExecProcess command that suspends the TUI while the editor runs.
func openInEditor(path, content string, client *apiclient.Client, vaultID string) tea.Cmd {
	ext := filepath.Ext(path)
	tmpFile, err := os.CreateTemp("", "know-*"+ext)
	if err != nil {
		return editorErr(path, fmt.Errorf("create temp file: %w", err))
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()

	if err := os.WriteFile(tmpPath, []byte(content), 0600); err != nil {
		os.Remove(tmpPath)
		return editorErr(path, fmt.Errorf("write temp file: %w", err))
	}

	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}
	if _, err := exec.LookPath(editor); err != nil {
		os.Remove(tmpPath)
		return editorErr(path, fmt.Errorf("editor %q not found: set $EDITOR", editor))
	}

	c := exec.Command(editor, tmpPath)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		defer func() {
			if removeErr := os.Remove(tmpPath); removeErr != nil {
				slog.Warn("failed to remove temp file", "path", tmpPath, "error", removeErr)
			}
		}()

		if err != nil {
			return editorFinishedMsg{path: path, err: fmt.Errorf("run editor: %w", err)}
		}

		data, err := os.ReadFile(tmpPath)
		if err != nil {
			return editorFinishedMsg{path: path, err: fmt.Errorf("read edited file: %w", err)}
		}

		updated := string(data)
		if updated == content {
			return editorFinishedMsg{path: path}
		}

		// Save changes back to the server. context.Background is required
		// because tea.ExecProcess callbacks don't receive a context.
		if _, err := client.EditDocument(context.Background(), apiclient.EditDocumentRequest{
			VaultName: vaultID,
			Path:      path,
			Content:   updated,
		}); err != nil {
			return editorFinishedMsg{path: path, err: fmt.Errorf("save: %w", err)}
		}

		return editorFinishedMsg{path: path, content: updated, changed: true}
	})
}
