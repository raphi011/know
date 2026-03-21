package record

import (
	"strings"

	lipgloss "charm.land/lipgloss/v2"
)

var (
	// Colors — reuse the project palette from internal/tui/styles.go.
	primaryColor = lipgloss.Color("#7C3AED") // violet
	errorColor   = lipgloss.Color("#EF4444") // red
	mutedColor   = lipgloss.Color("#9CA3AF") // light gray
	accentColor  = lipgloss.Color("#10B981") // green

	recordIndicatorStyle = lipgloss.NewStyle().
				Foreground(errorColor).
				Bold(true)

	elapsedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF")).
			Bold(true)

	waveformStyle = lipgloss.NewStyle().
			Foreground(primaryColor)

	hotkeyStyle = lipgloss.NewStyle().
			Foreground(mutedColor)

	hotkeyKeyStyle = lipgloss.NewStyle().
			Foreground(accentColor).
			Bold(true)

	savingStyle = lipgloss.NewStyle().
			Foreground(primaryColor)

	doneStyle = lipgloss.NewStyle().
			Foreground(accentColor)

	errorMsgStyle = lipgloss.NewStyle().
			Foreground(errorColor)

	bufferingBarStyle = lipgloss.NewStyle().
				Foreground(mutedColor)

	cursorStyle = lipgloss.NewStyle().
			Foreground(primaryColor).
			Bold(true)
)

// blocks maps amplitude (0.0–1.0) to Unicode block characters.
var blocks = [8]rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

// RenderWaveform returns a string of block characters representing the amplitude values.
func RenderWaveform(amplitudes []float64, width int) string {
	start := 0
	if len(amplitudes) > width {
		start = len(amplitudes) - width
	}

	runes := make([]rune, 0, width)
	for i := start; i < len(amplitudes); i++ {
		a := amplitudes[i]
		if a < 0 {
			a = 0
		}
		if a > 1 {
			a = 1
		}
		idx := int(a * 7)
		runes = append(runes, blocks[idx])
	}

	// Pad with lowest block if we don't have enough data yet.
	for len(runes) < width {
		runes = append([]rune{blocks[0]}, runes...)
	}

	return string(runes)
}

// RenderPlaybackWaveform renders a waveform with a playback cursor and
// buffering indicator. downloadedBars is how many bars have real data;
// the rest show as dim placeholder characters.
func RenderPlaybackWaveform(amplitudes []float64, width, cursorPos, downloadedBars int) string {
	runes := make([]string, 0, width)

	for i := range width {
		var ch rune
		if i < len(amplitudes) && i < downloadedBars {
			a := max(0, min(1, amplitudes[i]))
			ch = blocks[int(a*7)]
		} else {
			ch = '░'
		}

		s := string(ch)
		if i >= downloadedBars {
			runes = append(runes, bufferingBarStyle.Render(s))
		} else if i == cursorPos {
			runes = append(runes, cursorStyle.Render(s))
		} else {
			runes = append(runes, waveformStyle.Render(s))
		}
	}

	return strings.Join(runes, "")
}
