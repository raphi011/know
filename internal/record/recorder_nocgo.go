//go:build !cgo

package record

// Recorder captures audio. This is a stub for non-CGO builds.
type Recorder struct{}

// NewRecorder returns an error in non-CGO builds because malgo requires CGO.
func NewRecorder() (*Recorder, error) {
	return nil, errNoCGO
}

func (r *Recorder) Start() error               { return errNoCGO }
func (r *Recorder) DrainAmplitudes() []float64 { return nil }
func (r *Recorder) Stop()                      {}
func (r *Recorder) PCMData() []byte            { return nil }
func (r *Recorder) Close()                     {}
