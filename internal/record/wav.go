package record

import (
	"encoding/binary"
	"fmt"
	"io"
	"time"
)

// Default recording format: 44.1 kHz, mono, 16-bit signed PCM.
const (
	sampleRate    = 44100
	channels      = 1
	bitsPerSample = 16
)

// WAVHeader contains parsed WAV file metadata and PCM data.
type WAVHeader struct {
	SampleRate    uint32
	Channels      uint16
	BitsPerSample uint16
	DataSize      uint32
	PCMData       []byte
}

// Duration returns the audio duration.
func (h *WAVHeader) Duration() time.Duration {
	bytesPerSample := uint32(h.BitsPerSample) / 8 * uint32(h.Channels)
	if bytesPerSample == 0 || h.SampleRate == 0 {
		return 0
	}
	totalSamples := h.DataSize / bytesPerSample
	return time.Duration(totalSamples) * time.Second / time.Duration(h.SampleRate)
}

// ParseWAVHeader reads a WAV file, extracting the header metadata and PCM data.
func ParseWAVHeader(r io.Reader) (*WAVHeader, error) {
	var riffHeader [12]byte
	if _, err := io.ReadFull(r, riffHeader[:]); err != nil {
		return nil, fmt.Errorf("read RIFF header: %w", err)
	}
	if string(riffHeader[0:4]) != "RIFF" || string(riffHeader[8:12]) != "WAVE" {
		return nil, fmt.Errorf("not a WAV file")
	}

	hdr := &WAVHeader{}

	for {
		var chunkHeader [8]byte
		if _, err := io.ReadFull(r, chunkHeader[:]); err != nil {
			if hdr.PCMData != nil {
				return hdr, nil
			}
			return nil, fmt.Errorf("read chunk header: %w", err)
		}
		chunkID := string(chunkHeader[0:4])
		chunkSize := binary.LittleEndian.Uint32(chunkHeader[4:8])

		switch chunkID {
		case "fmt ":
			fmtData := make([]byte, chunkSize)
			if _, err := io.ReadFull(r, fmtData); err != nil {
				return nil, fmt.Errorf("read fmt chunk: %w", err)
			}
			hdr.Channels = binary.LittleEndian.Uint16(fmtData[2:4])
			hdr.SampleRate = binary.LittleEndian.Uint32(fmtData[4:8])
			hdr.BitsPerSample = binary.LittleEndian.Uint16(fmtData[14:16])

		case "data":
			hdr.DataSize = chunkSize
			hdr.PCMData = make([]byte, chunkSize)
			if _, err := io.ReadFull(r, hdr.PCMData); err != nil {
				return nil, fmt.Errorf("read data chunk: %w", err)
			}
			return hdr, nil

		default:
			if _, err := io.CopyN(io.Discard, r, int64(chunkSize)); err != nil {
				return nil, fmt.Errorf("skip chunk %q: %w", chunkID, err)
			}
		}
	}
}

// WriteWAV writes a WAV file (RIFF header + raw PCM data) to w.
func WriteWAV(w io.Writer, pcm []byte, sampleRate, channels, bitsPerSample uint32) error {
	byteRate := sampleRate * channels * bitsPerSample / 8
	blockAlign := uint16(channels * bitsPerSample / 8)
	dataSize := uint32(len(pcm))

	// RIFF header.
	if _, err := w.Write([]byte("RIFF")); err != nil {
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, uint32(36+dataSize)); err != nil {
		return err
	}
	if _, err := w.Write([]byte("WAVE")); err != nil {
		return err
	}

	// fmt sub-chunk.
	if _, err := w.Write([]byte("fmt ")); err != nil {
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, uint32(16)); err != nil { // sub-chunk size
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, uint16(1)); err != nil { // PCM format
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, uint16(channels)); err != nil {
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, sampleRate); err != nil {
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, byteRate); err != nil {
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, blockAlign); err != nil {
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, uint16(bitsPerSample)); err != nil {
		return err
	}

	// data sub-chunk.
	if _, err := w.Write([]byte("data")); err != nil {
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, dataSize); err != nil {
		return err
	}
	if len(pcm) > 0 {
		if _, err := w.Write(pcm); err != nil {
			return err
		}
	}

	return nil
}
