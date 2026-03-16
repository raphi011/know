package stt

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTranscribe_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/audio/transcriptions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer test-key" {
			t.Errorf("unexpected auth header: %s", auth)
		}

		// Verify multipart form
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			t.Fatalf("parse multipart: %v", err)
		}

		if model := r.FormValue("model"); model != "gpt-4o-transcribe" {
			t.Errorf("expected model gpt-4o-transcribe, got %s", model)
		}
		if rf := r.FormValue("response_format"); rf != "verbose_json" {
			t.Errorf("expected response_format verbose_json, got %s", rf)
		}
		if tg := r.FormValue("timestamp_granularities[]"); tg != "segment" {
			t.Errorf("expected timestamp_granularities[] segment, got %s", tg)
		}

		file, header, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("get form file: %v", err)
		}
		defer file.Close()

		if header.Filename != "audio.mp3" {
			t.Errorf("expected filename audio.mp3, got %s", header.Filename)
		}

		audioData, err := io.ReadAll(file)
		if err != nil {
			t.Fatalf("read file: %v", err)
		}
		if string(audioData) != "fake-audio" {
			t.Errorf("unexpected audio data: %s", audioData)
		}

		resp := verboseResponse{
			Text: "Hello world",
			Segments: []verboseSegment{
				{Start: 0.0, End: 1.5, Text: "Hello"},
				{Start: 1.5, End: 3.0, Text: " world"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	tr := NewOpenAI("test-key", "")
	tr.baseURL = server.URL

	result, err := tr.Transcribe(context.Background(), []byte("fake-audio"), "audio/mpeg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Text != "Hello world" {
		t.Errorf("expected text 'Hello world', got %q", result.Text)
	}
	if len(result.Segments) != 2 {
		t.Fatalf("expected 2 segments, got %d", len(result.Segments))
	}
	if result.Segments[0].Start != 0.0 || result.Segments[0].End != 1.5 || result.Segments[0].Text != "Hello" {
		t.Errorf("unexpected segment 0: %+v", result.Segments[0])
	}
	if result.Segments[1].Start != 1.5 || result.Segments[1].End != 3.0 || result.Segments[1].Text != " world" {
		t.Errorf("unexpected segment 1: %+v", result.Segments[1])
	}
}

func TestTranscribe_ErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":{"message":"invalid audio"}}`))
	}))
	defer server.Close()

	tr := NewOpenAI("test-key", "")
	tr.baseURL = server.URL

	_, err := tr.Transcribe(context.Background(), []byte("bad"), "audio/mpeg")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := err.Error(); got != `transcription failed (HTTP 400): {"error":{"message":"invalid audio"}}` {
		t.Errorf("unexpected error message: %s", got)
	}
}

func TestExtForMIME(t *testing.T) {
	tests := []struct {
		mime string
		want string
	}{
		{"audio/mpeg", ".mp3"},
		{"audio/wav", ".wav"},
		{"audio/ogg", ".ogg"},
		{"audio/flac", ".flac"},
		{"audio/mp4", ".m4a"},
		{"audio/aac", ".aac"},
		{"audio/opus", ".opus"},
		{"audio/webm", ".weba"},
		{"audio/unknown", ".bin"},
		{"", ".bin"},
	}

	for _, tt := range tests {
		if got := extForMIME(tt.mime); got != tt.want {
			t.Errorf("extForMIME(%q) = %q, want %q", tt.mime, got, tt.want)
		}
	}
}
