package record

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"
)

func TestWriteWAV_Header(t *testing.T) {
	// 4 samples of 16-bit mono audio = 8 bytes of PCM data.
	pcm := make([]byte, 8)
	binary.LittleEndian.PutUint16(pcm[0:2], 0)
	binary.LittleEndian.PutUint16(pcm[2:4], 16383)
	binary.LittleEndian.PutUint16(pcm[4:6], 32767)
	binary.LittleEndian.PutUint16(pcm[6:8], 0)

	var buf bytes.Buffer
	err := WriteWAV(&buf, pcm, 44100, 1, 16)
	if err != nil {
		t.Fatalf("WriteWAV: %v", err)
	}

	data := buf.Bytes()

	// WAV file = 44-byte header + PCM data.
	if len(data) != 44+len(pcm) {
		t.Fatalf("expected %d bytes, got %d", 44+len(pcm), len(data))
	}

	// RIFF header.
	if string(data[0:4]) != "RIFF" {
		t.Errorf("expected RIFF, got %q", data[0:4])
	}
	chunkSize := binary.LittleEndian.Uint32(data[4:8])
	if chunkSize != uint32(36+len(pcm)) {
		t.Errorf("chunk size: expected %d, got %d", 36+len(pcm), chunkSize)
	}
	if string(data[8:12]) != "WAVE" {
		t.Errorf("expected WAVE, got %q", data[8:12])
	}

	// fmt sub-chunk.
	if string(data[12:16]) != "fmt " {
		t.Errorf("expected 'fmt ', got %q", data[12:16])
	}
	fmtSize := binary.LittleEndian.Uint32(data[16:20])
	if fmtSize != 16 {
		t.Errorf("fmt chunk size: expected 16, got %d", fmtSize)
	}
	audioFormat := binary.LittleEndian.Uint16(data[20:22])
	if audioFormat != 1 { // PCM
		t.Errorf("audio format: expected 1 (PCM), got %d", audioFormat)
	}
	numChannels := binary.LittleEndian.Uint16(data[22:24])
	if numChannels != 1 {
		t.Errorf("channels: expected 1, got %d", numChannels)
	}
	sampleRate := binary.LittleEndian.Uint32(data[24:28])
	if sampleRate != 44100 {
		t.Errorf("sample rate: expected 44100, got %d", sampleRate)
	}
	byteRate := binary.LittleEndian.Uint32(data[28:32])
	if byteRate != 88200 { // 44100 * 1 * 16/8
		t.Errorf("byte rate: expected 88200, got %d", byteRate)
	}
	blockAlign := binary.LittleEndian.Uint16(data[32:34])
	if blockAlign != 2 { // 1 * 16/8
		t.Errorf("block align: expected 2, got %d", blockAlign)
	}
	bitsPerSample := binary.LittleEndian.Uint16(data[34:36])
	if bitsPerSample != 16 {
		t.Errorf("bits per sample: expected 16, got %d", bitsPerSample)
	}

	// data sub-chunk.
	if string(data[36:40]) != "data" {
		t.Errorf("expected 'data', got %q", data[36:40])
	}
	dataSize := binary.LittleEndian.Uint32(data[40:44])
	if dataSize != uint32(len(pcm)) {
		t.Errorf("data size: expected %d, got %d", len(pcm), dataSize)
	}

	// PCM data preserved exactly.
	if !bytes.Equal(data[44:], pcm) {
		t.Error("PCM data mismatch")
	}
}

func TestWriteWAV_EmptyPCM(t *testing.T) {
	var buf bytes.Buffer
	err := WriteWAV(&buf, nil, 44100, 1, 16)
	if err != nil {
		t.Fatalf("WriteWAV with empty PCM: %v", err)
	}
	if buf.Len() != 44 {
		t.Errorf("expected 44-byte header only, got %d bytes", buf.Len())
	}
}

func TestParseWAVHeader_RoundTrip(t *testing.T) {
	pcm := int16PCM(100, 200, 300, 400, 500, 600, 700, 800)
	var buf bytes.Buffer
	err := WriteWAV(&buf, pcm, 44100, 1, 16)
	if err != nil {
		t.Fatalf("WriteWAV: %v", err)
	}

	hdr, err := ParseWAVHeader(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("ParseWAVHeader: %v", err)
	}

	if hdr.SampleRate != 44100 {
		t.Errorf("sample rate: expected 44100, got %d", hdr.SampleRate)
	}
	if hdr.Channels != 1 {
		t.Errorf("channels: expected 1, got %d", hdr.Channels)
	}
	if hdr.BitsPerSample != 16 {
		t.Errorf("bits per sample: expected 16, got %d", hdr.BitsPerSample)
	}
	if hdr.DataSize != uint32(len(pcm)) {
		t.Errorf("data size: expected %d, got %d", len(pcm), hdr.DataSize)
	}
	if !bytes.Equal(hdr.PCMData, pcm) {
		t.Error("PCM data mismatch")
	}
}

func TestParseWAVHeader_InvalidRIFF(t *testing.T) {
	_, err := ParseWAVHeader(bytes.NewReader([]byte("not a wav")))
	if err == nil {
		t.Error("expected error for invalid WAV")
	}
}

func TestWAVHeader_Duration(t *testing.T) {
	tests := []struct {
		name     string
		hdr      WAVHeader
		expected time.Duration
	}{
		{
			name: "mono 16-bit 44100Hz",
			hdr: WAVHeader{
				SampleRate:    44100,
				Channels:      1,
				BitsPerSample: 16,
				DataSize:      88200, // 1 second of audio
			},
			expected: time.Second,
		},
		{
			name: "stereo 16-bit 48000Hz",
			hdr: WAVHeader{
				SampleRate:    48000,
				Channels:      2,
				BitsPerSample: 16,
				DataSize:      192000, // 1 second
			},
			expected: time.Second,
		},
		{
			name:     "zero sample rate",
			hdr:      WAVHeader{SampleRate: 0, Channels: 1, BitsPerSample: 16, DataSize: 100},
			expected: 0,
		},
		{
			name:     "zero bits per sample",
			hdr:      WAVHeader{SampleRate: 44100, Channels: 1, BitsPerSample: 0, DataSize: 100},
			expected: 0,
		},
		{
			name:     "zero channels",
			hdr:      WAVHeader{SampleRate: 44100, Channels: 0, BitsPerSample: 16, DataSize: 100},
			expected: 0,
		},
		{
			name:     "zero data size",
			hdr:      WAVHeader{SampleRate: 44100, Channels: 1, BitsPerSample: 16, DataSize: 0},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.hdr.Duration()
			if got != tt.expected {
				t.Errorf("Duration() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestWriteWAV_Stereo(t *testing.T) {
	// 2 stereo frames = 8 bytes (2 channels * 2 bytes * 2 frames).
	pcm := make([]byte, 8)

	var buf bytes.Buffer
	err := WriteWAV(&buf, pcm, 48000, 2, 16)
	if err != nil {
		t.Fatalf("WriteWAV stereo: %v", err)
	}

	data := buf.Bytes()
	numChannels := binary.LittleEndian.Uint16(data[22:24])
	if numChannels != 2 {
		t.Errorf("channels: expected 2, got %d", numChannels)
	}
	byteRate := binary.LittleEndian.Uint32(data[28:32])
	if byteRate != 192000 { // 48000 * 2 * 16/8
		t.Errorf("byte rate: expected 192000, got %d", byteRate)
	}
	blockAlign := binary.LittleEndian.Uint16(data[32:34])
	if blockAlign != 4 { // 2 * 16/8
		t.Errorf("block align: expected 4, got %d", blockAlign)
	}
}
