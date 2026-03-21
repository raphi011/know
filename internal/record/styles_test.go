package record

import (
	"testing"
)

func TestRenderWaveform(t *testing.T) {
	tests := []struct {
		name       string
		amplitudes []float64
		width      int
		wantLen    int
	}{
		{
			name:       "exact width",
			amplitudes: []float64{0, 0.5, 1.0},
			width:      3,
			wantLen:    3,
		},
		{
			name:       "pads when fewer amplitudes than width",
			amplitudes: []float64{1.0},
			width:      5,
			wantLen:    5,
		},
		{
			name:       "truncates from start when more amplitudes than width",
			amplitudes: []float64{0, 0.2, 0.4, 0.6, 0.8, 1.0},
			width:      3,
			wantLen:    3,
		},
		{
			name:       "empty amplitudes",
			amplitudes: nil,
			width:      4,
			wantLen:    4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RenderWaveform(tt.amplitudes, tt.width)
			runes := []rune(got)
			if len(runes) != tt.wantLen {
				t.Errorf("RenderWaveform() len = %d, want %d", len(runes), tt.wantLen)
			}
		})
	}
}

func TestRenderWaveform_BlockSelection(t *testing.T) {
	// Amplitude 0.0 should give lowest block, 1.0 should give highest.
	low := RenderWaveform([]float64{0.0}, 1)
	high := RenderWaveform([]float64{1.0}, 1)

	if []rune(low)[0] != '▁' {
		t.Errorf("amplitude 0.0: expected '▁', got %c", []rune(low)[0])
	}
	if []rune(high)[0] != '█' {
		t.Errorf("amplitude 1.0: expected '█', got %c", []rune(high)[0])
	}
}

func TestRenderWaveform_ClampsOutOfRange(t *testing.T) {
	// Negative and >1.0 should be clamped.
	result := RenderWaveform([]float64{-0.5, 1.5}, 2)
	runes := []rune(result)
	if runes[0] != '▁' {
		t.Errorf("negative amplitude: expected '▁', got %c", runes[0])
	}
	if runes[1] != '█' {
		t.Errorf("amplitude >1.0: expected '█', got %c", runes[1])
	}
}
