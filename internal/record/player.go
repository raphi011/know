package record

import (
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/gen2brain/malgo"
)

// PlayerState represents the current state of the audio player.
type PlayerState int

const (
	PlayerLoading PlayerState = iota
	PlayerPlaying
	PlayerPaused
	PlayerStopped
)

// Player manages streaming WAV download and malgo playback.
type Player struct {
	mu sync.Mutex

	// Audio data.
	pcmBuf     []byte
	totalBytes int // expected total PCM bytes from WAV header
	downloaded bool

	// Playback state.
	playOffset int // byte offset into pcmBuf
	state      PlayerState

	// WAV metadata.
	sampleRate    uint32
	channels      uint16
	bitsPerSample uint16
	duration      time.Duration

	// malgo resources.
	ctx    *malgo.AllocatedContext
	device *malgo.Device
}

// NewPlayer creates a Player from a WAV data stream (e.g., HTTP response body).
// It reads the entire WAV synchronously (header + PCM data).
// Returns after parsing — playback can start immediately via Play().
func NewPlayer(r io.ReadCloser) (*Player, error) {
	hdr, err := ParseWAVHeader(r)
	r.Close()
	if err != nil {
		return nil, fmt.Errorf("parse WAV: %w", err)
	}

	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return nil, fmt.Errorf("init audio context: %w", err)
	}

	p := &Player{
		pcmBuf:        hdr.PCMData,
		totalBytes:    int(hdr.DataSize),
		downloaded:    true,
		state:         PlayerPaused,
		sampleRate:    hdr.SampleRate,
		channels:      hdr.Channels,
		bitsPerSample: hdr.BitsPerSample,
		duration:      hdr.Duration(),
		ctx:           ctx,
	}

	return p, nil
}

// Play starts or resumes playback.
func (p *Player) Play() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.state == PlayerPlaying {
		return nil
	}

	if p.device == nil {
		if err := p.initDevice(); err != nil {
			return err
		}
	}

	if err := p.device.Start(); err != nil {
		return fmt.Errorf("start playback: %w", err)
	}
	p.state = PlayerPlaying
	return nil
}

// Pause pauses playback.
func (p *Player) Pause() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.state != PlayerPlaying || p.device == nil {
		return
	}
	p.device.Stop()
	p.state = PlayerPaused
}

// PlayPause toggles between playing and paused.
func (p *Player) PlayPause() error {
	p.mu.Lock()
	state := p.state
	p.mu.Unlock()

	if state == PlayerPlaying {
		p.Pause()
		return nil
	}
	return p.Play()
}

// Seek moves the playback position by the given duration offset.
func (p *Player) Seek(d time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()

	bytesPerSecond := int(p.sampleRate) * int(p.channels) * int(p.bitsPerSample) / 8
	offsetBytes := int(d.Seconds()) * bytesPerSecond

	// Align to sample boundary.
	blockAlign := int(p.channels) * int(p.bitsPerSample) / 8
	if blockAlign > 0 {
		offsetBytes = (offsetBytes / blockAlign) * blockAlign
	}

	p.playOffset += offsetBytes
	if p.playOffset < 0 {
		p.playOffset = 0
	}
	if p.playOffset > len(p.pcmBuf) {
		p.playOffset = len(p.pcmBuf)
	}
}

// Position returns the current playback position as a duration.
func (p *Player) Position() time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()

	bytesPerSecond := int(p.sampleRate) * int(p.channels) * int(p.bitsPerSample) / 8
	if bytesPerSecond == 0 {
		return 0
	}
	return time.Duration(p.playOffset) * time.Second / time.Duration(bytesPerSecond)
}

// Duration returns the total audio duration.
func (p *Player) Duration() time.Duration {
	return p.duration
}

// State returns the current player state.
func (p *Player) State() PlayerState {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.state
}

// DownloadedRatio returns 0.0–1.0 indicating download progress.
func (p *Player) DownloadedRatio() float64 {
	if p.totalBytes == 0 {
		return 1
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return float64(len(p.pcmBuf)) / float64(p.totalBytes)
}

// PCMData returns the currently downloaded PCM data.
func (p *Player) PCMData() []byte {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.pcmBuf
}

// Close stops playback and releases resources. Safe to call multiple times.
func (p *Player) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.state = PlayerStopped
	if p.device != nil {
		p.device.Stop()
		p.device.Uninit()
		p.device = nil
	}
	if p.ctx != nil {
		_ = p.ctx.Uninit()
		p.ctx.Free()
		p.ctx = nil
	}
}

// initDevice sets up the malgo playback device. Must be called with mu held.
func (p *Player) initDevice() error {
	deviceConfig := malgo.DefaultDeviceConfig(malgo.Playback)
	deviceConfig.Playback.Format = malgo.FormatS16
	deviceConfig.Playback.Channels = uint32(p.channels)
	deviceConfig.SampleRate = p.sampleRate

	onData := func(output, _ []byte, frameCount uint32) {
		p.mu.Lock()
		defer p.mu.Unlock()

		bytesNeeded := len(output)
		available := len(p.pcmBuf) - p.playOffset

		if available <= 0 {
			// End of audio — fill with silence and reset to start.
			for i := range output {
				output[i] = 0
			}
			if p.downloaded {
				p.state = PlayerPaused
				p.playOffset = 0
			}
			return
		}

		toRead := min(bytesNeeded, available)
		copy(output[:toRead], p.pcmBuf[p.playOffset:p.playOffset+toRead])
		p.playOffset += toRead

		// Fill remainder with silence if buffer underrun.
		for i := toRead; i < bytesNeeded; i++ {
			output[i] = 0
		}
	}

	callbacks := malgo.DeviceCallbacks{
		Data: onData,
	}

	device, err := malgo.InitDevice(p.ctx.Context, deviceConfig, callbacks)
	if err != nil {
		return fmt.Errorf("init playback device: %w", err)
	}
	p.device = device
	return nil
}
