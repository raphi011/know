package record

import (
	"sync"

	"github.com/gen2brain/malgo"
)

const (
	sampleRate    = 44100
	channels      = 1
	bitsPerSample = 16
)

// Recorder captures audio from the default input device via malgo.
// It uses auto-gain: tracks the peak amplitude seen so far and normalizes
// all values against it, so the waveform fills the visual range regardless
// of microphone gain level.
type Recorder struct {
	ctx     *malgo.AllocatedContext
	device  *malgo.Device
	stopped bool

	mu         sync.Mutex
	pcmBuf     []byte
	amplitudes []float64 // buffered raw peak amplitudes from callback
	peakSeen   float64   // max amplitude seen so far (auto-gain reference)
}

// NewRecorder creates a Recorder. Call Start to begin capturing.
func NewRecorder() (*Recorder, error) {
	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return nil, err
	}
	return &Recorder{ctx: ctx}, nil
}

// Start begins capturing audio from the default input device.
func (r *Recorder) Start() error {
	deviceConfig := malgo.DefaultDeviceConfig(malgo.Capture)
	deviceConfig.Capture.Format = malgo.FormatS16
	deviceConfig.Capture.Channels = channels
	deviceConfig.SampleRate = sampleRate

	onData := func(_, input []byte, framecount uint32) {
		// Copy input data — the slice is reused by malgo.
		buf := make([]byte, len(input))
		copy(buf, input)

		amp := peakAmplitude(buf)

		r.mu.Lock()
		r.pcmBuf = append(r.pcmBuf, buf...)
		r.amplitudes = append(r.amplitudes, amp)
		if amp > r.peakSeen {
			r.peakSeen = amp
		}
		r.mu.Unlock()
	}

	callbacks := malgo.DeviceCallbacks{
		Data: onData,
	}

	device, err := malgo.InitDevice(r.ctx.Context, deviceConfig, callbacks)
	if err != nil {
		return err
	}
	r.device = device

	return r.device.Start()
}

// DrainAmplitudes returns and clears the buffered amplitudes from the audio
// callback, normalized against the peak amplitude seen so far (auto-gain).
func (r *Recorder) DrainAmplitudes() []float64 {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.amplitudes) == 0 {
		return nil
	}

	peak := r.peakSeen
	out := r.amplitudes
	r.amplitudes = nil

	// Normalize against peak (auto-gain).
	if peak > 0 {
		for i, a := range out {
			out[i] = a / peak
		}
	}

	return out
}

// Stop stops capturing audio. Safe to call multiple times from any goroutine.
func (r *Recorder) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.device != nil && !r.stopped {
		r.device.Stop()
		r.device.Uninit()
		r.device = nil
		r.stopped = true
	}
}

// PCMData returns a copy of the captured PCM data.
func (r *Recorder) PCMData() []byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]byte, len(r.pcmBuf))
	copy(out, r.pcmBuf)
	return out
}

// Close releases the malgo context. Safe to call multiple times.
func (r *Recorder) Close() {
	r.Stop()
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.ctx != nil {
		_ = r.ctx.Uninit()
		r.ctx.Free()
		r.ctx = nil
	}
}
