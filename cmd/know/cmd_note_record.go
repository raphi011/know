package main

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/raphi011/know/internal/record"
	"github.com/spf13/cobra"
)

var (
	noteRecordVaultID *string
	noteRecordPath    string
)

var noteRecordCmd = &cobra.Command{
	Use:   "record",
	Short: "Record audio and upload to vault",
	Long: `Record audio from the microphone and upload to a vault for transcription.

The recording is saved as a WAV file and uploaded to the vault. The server's
transcription pipeline will automatically process it.

Controls:
  Enter    Stop recording and upload
  Escape   Cancel and discard

Environment variables:
  KNOW_VAULT    vault name (alternative to --vault flag)`,
	RunE: runNoteRecord,
}

func init() {
	noteRecordVaultID = addVaultFlag(noteRecordCmd)
	noteRecordCmd.Flags().StringVar(&noteRecordPath, "path", "/recordings/", "vault folder for recordings")
}

func runNoteRecord(_ *cobra.Command, _ []string) error {
	// Normalize path.
	path := noteRecordPath
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if !strings.HasSuffix(path, "/") {
		path = path + "/"
	}

	// Initialize recorder.
	rec, err := record.NewRecorder()
	if err != nil {
		return fmt.Errorf("init audio: %w", err)
	}
	defer rec.Close()

	client := globalAPI.newClient()
	model := record.NewModel(rec, client, *noteRecordVaultID, path)

	p := tea.NewProgram(model)
	finalModel, err := p.Run()
	if err != nil {
		return fmt.Errorf("record: %w", err)
	}
	if m, ok := finalModel.(record.Model); ok {
		if errMsg := m.Err(); errMsg != "" {
			return fmt.Errorf("record: %s", errMsg)
		}
	}

	return nil
}
