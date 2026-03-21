package record

import (
	"encoding/binary"
	"math"
)

// RMSAmplitude computes the root-mean-square amplitude of 16-bit little-endian
// PCM samples, normalized to 0.0–1.0. Leftover bytes are ignored.
func RMSAmplitude(pcm []byte) float64 {
	nSamples := len(pcm) / 2
	if nSamples == 0 {
		return 0
	}

	var sumSquares float64
	for i := range nSamples {
		sample := int16(binary.LittleEndian.Uint16(pcm[i*2:]))
		norm := float64(sample) / 32768.0
		sumSquares += norm * norm
	}

	return math.Sqrt(sumSquares / float64(nSamples))
}

// ComputeWaveform splits PCM data into `width` equal chunks and computes
// the peak amplitude for each chunk. Returns a slice of amplitudes (0.0–1.0).
func ComputeWaveform(pcm []byte, width int) []float64 {
	nSamples := len(pcm) / 2
	if nSamples == 0 || width == 0 {
		return nil
	}

	samplesPerBar := nSamples / width
	if samplesPerBar == 0 {
		samplesPerBar = 1
	}

	bars := make([]float64, 0, width)
	for i := 0; i < nSamples && len(bars) < width; i += samplesPerBar {
		end := min(i+samplesPerBar, nSamples)
		chunk := pcm[i*2 : end*2]
		bars = append(bars, peakAmplitude(chunk))
	}

	// Auto-gain: normalize against peak so the waveform fills the visual range.
	var peak float64
	for _, a := range bars {
		if a > peak {
			peak = a
		}
	}
	if peak > 0 {
		for i, a := range bars {
			bars[i] = a / peak
		}
	}

	return bars
}

// TrimSilence removes leading and trailing 16-bit LE PCM samples whose
// absolute amplitude is below threshold (0.0–1.0). Works in windows of
// windowSize samples to avoid cutting mid-word on a single quiet sample.
// Returns a sub-slice of the original data (no copy).
func TrimSilence(pcm []byte, threshold float64, windowSize int) []byte {
	nSamples := len(pcm) / 2
	if nSamples == 0 || windowSize <= 0 {
		return pcm
	}

	isLoud := func(start, end int) bool {
		for i := start; i < end && i < nSamples; i++ {
			abs := math.Abs(float64(int16(binary.LittleEndian.Uint16(pcm[i*2:])))) / 32768.0
			if abs >= threshold {
				return true
			}
		}
		return false
	}

	// Find first loud window.
	startSample := 0
	for startSample < nSamples {
		if isLoud(startSample, startSample+windowSize) {
			break
		}
		startSample += windowSize
	}

	// Find last loud window.
	endSample := nSamples
	for endSample > startSample {
		windowStart := max(endSample-windowSize, startSample)
		if isLoud(windowStart, endSample) {
			break
		}
		endSample = windowStart
	}

	if startSample >= endSample {
		return pcm[:0]
	}
	return pcm[startSample*2 : endSample*2]
}

// NormalizePCM applies peak normalization to 16-bit LE PCM data in-place,
// scaling all samples so the loudest reaches targetPeak (0.0–1.0).
// Returns immediately if the audio is silent or already loud enough.
func NormalizePCM(pcm []byte, targetPeak float64) {
	peak := peakAmplitude(pcm)
	if peak == 0 || peak >= targetPeak {
		return
	}

	gain := targetPeak / peak
	nSamples := len(pcm) / 2
	for i := range nSamples {
		off := i * 2
		sample := float64(int16(binary.LittleEndian.Uint16(pcm[off:]))) * gain
		// Clamp to int16 range.
		if sample > 32767 {
			sample = 32767
		} else if sample < -32768 {
			sample = -32768
		}
		binary.LittleEndian.PutUint16(pcm[off:], uint16(int16(sample)))
	}
}

// peakAmplitude returns the max absolute sample value, normalized to 0.0–1.0.
func peakAmplitude(pcm []byte) float64 {
	nSamples := len(pcm) / 2
	if nSamples == 0 {
		return 0
	}

	var peak float64
	for i := range nSamples {
		sample := int16(binary.LittleEndian.Uint16(pcm[i*2:]))
		abs := math.Abs(float64(sample)) / 32768.0
		if abs > peak {
			peak = abs
		}
	}

	return peak
}
