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

func TestPeakAmplitude_QuietSignal(t *testing.T) {
	pcm := int16PCM(328, -200, 100, -328)
	amp := peakAmplitude(pcm)
	expected := 328.0 / 32768.0
	if math.Abs(amp-expected) > 0.001 {
		t.Errorf("quiet signal: expected ~%f, got %f", expected, amp)
	}
}
