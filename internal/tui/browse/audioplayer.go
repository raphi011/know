package browse

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/raphi011/know/internal/record"
)

type audioTickMsg time.Time

type audioPlayerModel struct {
	player     *record.Player
	waveform   []float64
	transcript string
	viewport   viewport.Model
	width      int
	height     int
	maxBars    int
	err        string
}

func newAudioPlayer(player *record.Player, transcript string, width, height int) audioPlayerModel {
	maxBars := max(min(width-4, 60), 10)

	wf := record.ComputeWaveform(player.PCMData(), maxBars)

	playerHeight := 5
	vpHeight := max(height-2-playerHeight, 1)
	vp := viewport.New(viewport.WithWidth(width), viewport.WithHeight(vpHeight))
	vp.SetContent(transcript)

	return audioPlayerModel{
		player:     player,
		waveform:   wf,
		transcript: transcript,
		viewport:   vp,
		width:      width,
		height:     height,
		maxBars:    maxBars,
	}
}

func (m audioPlayerModel) tick() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		return audioTickMsg(t)
	})
}

func (m audioPlayerModel) Update(msg tea.Msg) (audioPlayerModel, tea.Cmd) {
	switch msg := msg.(type) {
	case audioTickMsg:
		wf := record.ComputeWaveform(m.player.PCMData(), m.maxBars)
		if len(wf) > 0 {
			m.waveform = wf
		}
		return m, m.tick()

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.maxBars = max(min(msg.Width-4, 60), 10)
		m.waveform = record.ComputeWaveform(m.player.PCMData(), m.maxBars)
		playerHeight := 5
		vpHeight := max(msg.Height-2-playerHeight, 1)
		m.viewport.SetWidth(msg.Width)
		m.viewport.SetHeight(vpHeight)
		return m, nil

	case tea.KeyPressMsg:
		switch msg.String() {
		case " ", "space":
			if err := m.player.PlayPause(); err != nil {
				m.err = err.Error()
			}
			return m, nil
		case "left":
			m.player.Seek(-5 * time.Second)
			return m, nil
		case "right":
			m.player.Seek(5 * time.Second)
			return m, nil
		case "+", "=":
			m.player.SetVolume(m.player.Volume() + 0.1)
			return m, nil
		case "-":
			m.player.SetVolume(m.player.Volume() - 0.1)
			return m, nil
		case "esc":
			m.player.Close()
			return m, func() tea.Msg { return backToFinderMsg{} }
		case "q":
			m.player.Close()
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m audioPlayerModel) View() string {
	var b strings.Builder

	pos := m.player.Position()
	dur := m.player.Duration()
	cursorPos := 0
	if dur > 0 {
		cursorPos = int(float64(m.maxBars) * float64(pos) / float64(dur))
	}
	cursorPos = min(cursorPos, m.maxBars-1)

	downloadedBars := int(m.player.DownloadedRatio() * float64(m.maxBars))

	wf := record.RenderPlaybackWaveform(m.waveform, m.maxBars, cursorPos, downloadedBars)
	b.WriteString("  " + wf + "\n")

	cursorLine := strings.Repeat(" ", cursorPos+2) + "▲"
	posStr := fmt.Sprintf(" %s / %s", formatDur(pos), formatDur(dur))
	b.WriteString(cursorLine + posStr + "\n")

	state := m.player.State()
	var status string
	switch state {
	case record.PlayerPlaying:
		status = playerStatusStyle.Render("▶ Playing")
	case record.PlayerPaused:
		status = playerPausedStyle.Render("⏸ Paused")
	default:
		status = playerPausedStyle.Render("⏹ Stopped")
	}
	vol := int(m.player.Volume() * 100)
	b.WriteString("  " + status + playerHotkeyStyle.Render(fmt.Sprintf("  vol: %d%%", vol)) + "\n")

	b.WriteString("  ")
	b.WriteString(playerHotkeyKeyStyle.Render("space") + playerHotkeyStyle.Render(" play/pause  "))
	b.WriteString(playerHotkeyKeyStyle.Render("←/→") + playerHotkeyStyle.Render(" seek  "))
	b.WriteString(playerHotkeyKeyStyle.Render("+/-") + playerHotkeyStyle.Render(" volume  "))
	b.WriteString(playerHotkeyKeyStyle.Render("esc") + playerHotkeyStyle.Render(" back"))
	b.WriteString("\n")

	if m.err != "" {
		b.WriteString("  " + errStyle.Render(m.err) + "\n")
	}

	if m.transcript != "" {
		b.WriteString("\n  " + separatorStyle.Render("── Transcript "+strings.Repeat("─", max(m.width-18, 0))) + "\n")
		b.WriteString(m.viewport.View())
	}

	return b.String()
}

func formatDur(d time.Duration) string {
	total := int(d.Seconds())
	m := total / 60
	s := total % 60
	return fmt.Sprintf("%d:%02d", m, s)
}
