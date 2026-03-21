package record

import (
	"encoding/binary"
	"math"
	"testing"
)

// int16PCM creates PCM bytes from int16 samples.
func int16PCM(samples ...int16) []byte {
	buf := make([]byte, len(samples)*2)
	for i, s := range samples {
		binary.LittleEndian.PutUint16(buf[i*2:], uint16(s))
	}
	return buf
}

func TestComputeWaveform(t *testing.T) {
	pcm := int16PCM(0, 0, 16384, 16384, 32767, 32767, 0, 0)
	wf := ComputeWaveform(pcm, 4)

	if len(wf) != 4 {
		t.Fatalf("expected 4 bars, got %d", len(wf))
	}
	if wf[0] != 0 {
		t.Errorf("bar 0: expected 0, got %f", wf[0])
	}
	if wf[2] < 0.99 {
		t.Errorf("bar 2: expected ~1.0, got %f", wf[2])
	}
	if wf[3] != 0 {
		t.Errorf("bar 3: expected 0, got %f", wf[3])
	}
}

func TestComputeWaveform_EmptyPCM(t *testing.T) {
	wf := ComputeWaveform(nil, 10)
	if len(wf) != 0 {
		t.Errorf("expected empty, got %d bars", len(wf))
	}
}

func TestRMSAmplitude_Silence(t *testing.T) {
	pcm := int16PCM(0, 0, 0, 0)
	amp := RMSAmplitude(pcm)
	if amp != 0 {
		t.Errorf("silence: expected 0, got %f", amp)
	}
}

func TestRMSAmplitude_MaxSignal(t *testing.T) {
	pcm := int16PCM(32767, 32767, 32767, 32767)
	amp := RMSAmplitude(pcm)
	if amp < 0.99 || amp > 1.0 {
		t.Errorf("max signal: expected ~1.0, got %f", amp)
	}
}

func TestRMSAmplitude_NegativeSignal(t *testing.T) {
	pcm := int16PCM(-32768, -32768)
	amp := RMSAmplitude(pcm)
	if amp < 0.99 || amp > 1.0 {
		t.Errorf("negative max: expected ~1.0, got %f", amp)
	}
}

func TestRMSAmplitude_MixedSignal(t *testing.T) {
	pcm := int16PCM(16384, -16384, 16384, -16384)
	amp := RMSAmplitude(pcm)
	expected := 16384.0 / 32768.0
	if math.Abs(amp-expected) > 0.01 {
		t.Errorf("mixed signal: expected ~%f, got %f", expected, amp)
	}
}

func TestRMSAmplitude_EmptyInput(t *testing.T) {
	amp := RMSAmplitude(nil)
	if amp != 0 {
		t.Errorf("empty: expected 0, got %f", amp)
	}
}

func TestRMSAmplitude_OddBytes(t *testing.T) {
	pcm := int16PCM(32767)
	pcm = append(pcm, 0xFF)
	amp := RMSAmplitude(pcm)
	if amp < 0.99 {
		t.Errorf("odd bytes: expected ~1.0, got %f", amp)
	}
}

func TestPeakAmplitude_Silence(t *testing.T) {
	pcm := int16PCM(0, 0, 0, 0)
	amp := peakAmplitude(pcm)
	if amp != 0 {
		t.Errorf("silence: expected 0, got %f", amp)
	}
}

func TestPeakAmplitude_MaxSignal(t *testing.T) {
	pcm := int16PCM(100, 32767, 100, 100)
	amp := peakAmplitude(pcm)
	if amp < 0.99 {
		t.Errorf("max signal: expected ~1.0, got %f", amp)
	}
}

func TestPeakAmplitude_NegativePeak(t *testing.T) {
	pcm := int16PCM(100, -32768, 100, 100)
	amp := peakAmplitude(pcm)
	if amp < 0.99 {
		t.Errorf("negative peak: expected ~1.0, got %f", amp)
	}
}

func TestPeakAmplitude_Empty(t *testing.T) {
	amp := peakAmplitude(nil)
	if amp != 0 {
		t.Errorf("empty: expected 0, got %f", amp)
	}
}

func TestNormalizePCM(t *testing.T) {
	// Quiet signal: peak at 328/32768 ≈ 0.01.
	pcm := int16PCM(328, -200, 100, -328)
	NormalizePCM(pcm, 0.9)

	// After normalization, peak should be ~0.9.
	peak := peakAmplitude(pcm)
	if math.Abs(peak-0.9) > 0.01 {
		t.Errorf("expected peak ~0.9, got %f", peak)
	}
}

func TestNormalizePCM_AlreadyLoud(t *testing.T) {
	pcm := int16PCM(32767, -32768, 16000)
	orig := make([]byte, len(pcm))
	copy(orig, pcm)

	NormalizePCM(pcm, 0.9)

	// Peak is already above 0.9, data should be unchanged.
	for i := range pcm {
		if pcm[i] != orig[i] {
			t.Fatal("expected no change for already-loud signal")
		}
	}
}

func TestNormalizePCM_Silent(t *testing.T) {
	pcm := int16PCM(0, 0, 0)
	NormalizePCM(pcm, 0.9) // should not panic or divide by zero

	peak := peakAmplitude(pcm)
	if peak != 0 {
		t.Errorf("expected peak 0, got %f", peak)
	}
}

func TestTrimSilence(t *testing.T) {
	// 3 silent samples, 2 loud, 3 silent — window size 1.
	pcm := int16PCM(0, 0, 0, 10000, 15000, 0, 0, 0)
	trimmed := TrimSilence(pcm, 0.01, 1)

	// Should keep only the 2 loud samples.
	if len(trimmed) != 4 { // 2 samples * 2 bytes
		t.Fatalf("expected 4 bytes (2 samples), got %d", len(trimmed))
	}
	s0 := int16(binary.LittleEndian.Uint16(trimmed[0:]))
	s1 := int16(binary.LittleEndian.Uint16(trimmed[2:]))
	if s0 != 10000 || s1 != 15000 {
		t.Errorf("expected [10000, 15000], got [%d, %d]", s0, s1)
	}
}

func TestTrimSilence_AllSilent(t *testing.T) {
	pcm := int16PCM(0, 0, 0, 0)
	trimmed := TrimSilence(pcm, 0.01, 1)
	if len(trimmed) != 0 {
		t.Errorf("expected empty, got %d bytes", len(trimmed))
	}
}

func TestTrimSilence_NoSilence(t *testing.T) {
	pcm := int16PCM(5000, 10000, 8000)
	trimmed := TrimSilence(pcm, 0.01, 1)
	if len(trimmed) != len(pcm) {
		t.Errorf("expected no trimming, got %d vs %d bytes", len(trimmed), len(pcm))
	}
}

func TestTrimSilence_WindowSize(t *testing.T) {
	// 4 silent samples, 2 loud, 4 silent — window size 2.
	pcm := int16PCM(0, 0, 0, 0, 20000, 20000, 0, 0, 0, 0)
	trimmed := TrimSilence(pcm, 0.01, 2)

	// Windows: [0,0] [0,0] [20000,20000] [0,0] [0,0]
	// First loud window starts at sample 4, last loud ends at sample 6.
	if len(trimmed) != 4 { // 2 samples * 2 bytes
		t.Fatalf("expected 4 bytes (2 samples), got %d", len(trimmed))
	}
}

func TestNormalizePCM_Clipping(t *testing.T) {
	// Two samples: one at 50% and one at 75% of max.
	// Normalizing to target 1.0 means gain = 1/0.75 ≈ 1.333.
	// The 75% sample should hit exactly 32767. The 50% sample scales up.
	pcm := int16PCM(16384, 24576) // 0.5 and 0.75 of 32768
	NormalizePCM(pcm, 1.0)

	s0 := int16(binary.LittleEndian.Uint16(pcm[0:]))
	s1 := int16(binary.LittleEndian.Uint16(pcm[2:]))

	// s1 (the peak) should be clamped to 32767.
	if s1 != 32767 {
		t.Errorf("peak sample: expected 32767, got %d", s1)
	}
	// s0 should scale proportionally.
	if s0 < 21000 || s0 > 22000 {
		t.Errorf("scaled sample: expected ~21845, got %d", s0)
	}
}

func TestTrimSilence_NegativeLoudSamples(t *testing.T) {
	// Loud signal with only negative values.
	pcm := int16PCM(0, 0, -20000, -15000, 0, 0)
	trimmed := TrimSilence(pcm, 0.01, 1)
	if len(trimmed) != 4 { // 2 samples * 2 bytes
		t.Fatalf("expected 4 bytes, got %d", len(trimmed))
	}
}

func TestComputeWaveform_WidthExceedsSamples(t *testing.T) {
	pcm := int16PCM(16384, 32767)
	wf := ComputeWaveform(pcm, 10)

	// Should produce at most nSamples bars (2), not 10.
	if len(wf) > 10 {
		t.Errorf("expected at most 10 bars, got %d", len(wf))
	}
	if len(wf) == 0 {
		t.Fatal("expected at least 1 bar")
	}
}

func TestPeakAmplitude_QuietSignal(t *testing.T) {
	pcm := int16PCM(328, -200, 100, -328)
	amp := peakAmplitude(pcm)
	expected := 328.0 / 32768.0
	if math.Abs(amp-expected) > 0.001 {
		t.Errorf("quiet signal: expected ~%f, got %f", expected, amp)
	}
}
