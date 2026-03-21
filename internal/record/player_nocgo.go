//go:build !cgo

package record

import (
	"io"
	"time"
)

// PlayerState represents the current state of the audio player.
type PlayerState int

const (
	_ PlayerState = iota // reserved
	PlayerPlaying
	PlayerPaused
	PlayerStopped
)

// Player manages WAV playback. This is a stub for non-CGO builds.
type Player struct{}

// NewPlayer returns an error in non-CGO builds because malgo requires CGO.
func NewPlayer(r io.ReadCloser) (*Player, error) {
	_ = r.Close() // best-effort; errNoCGO is the primary error to report
	return nil, errNoCGO
}

func (p *Player) Play() error             { return errNoCGO }
func (p *Player) Pause() error            { return errNoCGO }
func (p *Player) PlayPause() error        { return errNoCGO }
func (p *Player) Seek(time.Duration)      {}
func (p *Player) Position() time.Duration { return 0 }
func (p *Player) Duration() time.Duration { return 0 }
func (p *Player) Volume() float64         { return 0 }
func (p *Player) SetVolume(float64)       {}
func (p *Player) State() PlayerState      { return PlayerStopped }
func (p *Player) PCMData() []byte         { return nil }
func (p *Player) Close()                  {}
