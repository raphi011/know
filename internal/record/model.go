package record

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/raphi011/know/internal/apiclient"
)

type recordState int

const (
	stateRecording recordState = iota
	stateSaving
	stateDone
	stateError
	stateCancelled
)

// tickMsg triggers elapsed time and waveform updates.
type tickMsg time.Time

// saveCompleteMsg signals that the upload finished.
type saveCompleteMsg struct {
	path string
	err  error
}

// startedMsg is sent after recorder.Start completes.
type startedMsg struct {
	err error
}

// Model is the bubbletea v2 model for the recording screen.
type Model struct {
	recorder *Recorder
	client   *apiclient.Client
	vaultID  string
	basePath string // vault folder for recordings

	// Waveform state.
	waveform []float64 // rolling amplitude window
	maxBars  int       // waveform display width

	// Timing.
	startTime time.Time
	elapsed   time.Duration

	// State machine.
	state  recordState
	errMsg string
	result string // success message

	width int
}

// Err returns the error message if the model ended in an error state.
func (m Model) Err() string {
	if m.state == stateError {
		return m.errMsg
	}
	return ""
}

// NewModel creates a new recording TUI model.
func NewModel(recorder *Recorder, client *apiclient.Client, vaultID, basePath string) Model {
	return Model{
		recorder: recorder,
		client:   client,
		vaultID:  vaultID,
		basePath: basePath,
		maxBars:  60, // default, updated by WindowSizeMsg
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.startRecording(),
		m.tick(),
	)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.maxBars = max(min(msg.Width-4, 60), 10)
		return m, nil

	case tickMsg:
		if m.state != stateRecording {
			return m, nil
		}
		m.elapsed = time.Since(m.startTime)

		// Poll new amplitudes from the recorder.
		if amps := m.recorder.DrainAmplitudes(); len(amps) > 0 {
			m.waveform = append(m.waveform, amps...)
			// Trim to avoid unbounded growth.
			if len(m.waveform) > m.maxBars*2 {
				m.waveform = m.waveform[len(m.waveform)-m.maxBars:]
			}
		}

		return m, m.tick()

	case saveCompleteMsg:
		if msg.err != nil {
			m.state = stateError
			m.errMsg = msg.err.Error()
			return m, tea.Quit
		}
		m.state = stateDone
		m.result = msg.path
		return m, tea.Quit

	case startedMsg:
		if msg.err != nil {
			m.state = stateError
			m.errMsg = msg.err.Error()
			return m, tea.Quit
		}
		m.startTime = time.Now()
		return m, nil

	case tea.KeyPressMsg:
		switch {
		case msg.Key().Code == tea.KeyEnter:
			if m.state != stateRecording {
				return m, nil
			}
			pcm := m.recorder.PCMData()
			if len(pcm) == 0 {
				m.state = stateError
				m.errMsg = "no audio recorded"
				return m, tea.Quit
			}
			m.recorder.Stop()
			m.state = stateSaving
			return m, m.save(pcm)

		case msg.Key().Code == tea.KeyEscape:
			m.recorder.Stop()
			m.state = stateCancelled
			return m, tea.Quit
		}
	}

	return m, nil
}

func (m Model) View() tea.View {
	var b strings.Builder

	switch m.state {
	case stateRecording:
		// Line 1: indicator + elapsed.
		indicator := recordIndicatorStyle.Render("●")
		label := recordIndicatorStyle.Render(" Recording")
		elapsed := elapsedStyle.Render("  " + formatDuration(m.elapsed))
		b.WriteString(indicator + label + elapsed)
		b.WriteString("\n")

		// Line 2: waveform.
		wf := RenderWaveform(m.waveform, m.maxBars)
		b.WriteString("  " + waveformStyle.Render(wf))
		b.WriteString("\n")

		// Line 3: hotkeys.
		b.WriteString("  ")
		b.WriteString(hotkeyKeyStyle.Render("enter"))
		b.WriteString(hotkeyStyle.Render(" save  "))
		b.WriteString(hotkeyKeyStyle.Render("esc"))
		b.WriteString(hotkeyStyle.Render(" cancel"))
		b.WriteString("\n")

	case stateSaving:
		b.WriteString(savingStyle.Render("Uploading recording..."))
		b.WriteString("\n")

	case stateDone:
		b.WriteString(doneStyle.Render("✓ Uploaded " + m.result))
		b.WriteString("\n")

	case stateError:
		b.WriteString(errorMsgStyle.Render("✗ " + m.errMsg))
		b.WriteString("\n")

	case stateCancelled:
		b.WriteString(hotkeyStyle.Render("Recording cancelled."))
		b.WriteString("\n")
	}

	return tea.NewView(b.String())
}

func (m Model) startRecording() tea.Cmd {
	return func() tea.Msg {
		return startedMsg{err: m.recorder.Start()}
	}
}

func (m Model) tick() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m Model) save(pcm []byte) tea.Cmd {
	return func() tea.Msg {
		// Write WAV to temp file.
		tmpFile, err := os.CreateTemp("", "know-record-*.wav")
		if err != nil {
			return saveCompleteMsg{err: fmt.Errorf("create temp file: %w", err)}
		}
		defer os.Remove(tmpFile.Name())

		// Trim leading/trailing silence, then normalize volume.
		pcm = TrimSilence(pcm, 0.01, sampleRate/10) // 100ms windows, ~1% threshold
		NormalizePCM(pcm, 0.9)

		if err := WriteWAV(tmpFile, pcm, sampleRate, channels, bitsPerSample); err != nil {
			tmpFile.Close()
			return saveCompleteMsg{err: fmt.Errorf("write WAV: %w", err)}
		}
		if err := tmpFile.Close(); err != nil {
			return saveCompleteMsg{err: fmt.Errorf("flush WAV: %w", err)}
		}

		// Re-open for reading.
		f, err := os.Open(tmpFile.Name())
		if err != nil {
			return saveCompleteMsg{err: fmt.Errorf("reopen WAV: %w", err)}
		}
		defer f.Close()

		// Build vault path.
		filename := fmt.Sprintf("recording-%s.wav", time.Now().Format("2006-01-02-150405"))
		vaultPath := m.basePath
		if !strings.HasSuffix(vaultPath, "/") {
			vaultPath += "/"
		}
		vaultPath += filename

		// Upload.
		ctx := context.Background()
		results, err := m.client.BulkUpload(ctx, m.vaultID, apiclient.BulkMeta{Force: true}, []apiclient.BulkFile{
			{Path: vaultPath, Data: f},
		})
		if err != nil {
			return saveCompleteMsg{err: fmt.Errorf("upload: %w", err)}
		}
		for _, r := range results {
			if r.Status == "error" {
				return saveCompleteMsg{err: fmt.Errorf("upload %s: %s", r.Path, r.Error)}
			}
		}

		return saveCompleteMsg{path: vaultPath}
	}
}

// formatDuration formats a duration as M:SS.
func formatDuration(d time.Duration) string {
	total := int(d.Seconds())
	minutes := total / 60
	seconds := total % 60
	return fmt.Sprintf("%d:%02d", minutes, seconds)
}
