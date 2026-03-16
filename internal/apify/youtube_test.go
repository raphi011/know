package apify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNormalizeYouTubeURL(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "standard watch URL",
			input: "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
			want:  "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
		},
		{
			name:  "watch URL with extra params",
			input: "https://www.youtube.com/watch?v=dQw4w9WgXcQ&t=30s",
			want:  "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
		},
		{
			name:  "short URL",
			input: "https://youtu.be/dQw4w9WgXcQ",
			want:  "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
		},
		{
			name:  "shorts URL",
			input: "https://youtube.com/shorts/dQw4w9WgXcQ",
			want:  "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
		},
		{
			name:  "embed URL",
			input: "https://youtube.com/embed/dQw4w9WgXcQ",
			want:  "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
		},
		{
			name:  "live URL",
			input: "https://www.youtube.com/live/dQw4w9WgXcQ",
			want:  "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
		},
		{
			name:  "mobile URL",
			input: "https://m.youtube.com/watch?v=dQw4w9WgXcQ",
			want:  "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
		},
		{
			name:  "bare video ID",
			input: "dQw4w9WgXcQ",
			want:  "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
		},
		{
			name:  "no scheme",
			input: "youtube.com/watch?v=dQw4w9WgXcQ",
			want:  "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
		},
		{
			name:    "empty input",
			input:   "",
			wantErr: true,
		},
		{
			name:    "not a YouTube URL",
			input:   "https://vimeo.com/12345",
			wantErr: true,
		},
		{
			name:    "YouTube URL without video ID",
			input:   "https://www.youtube.com/",
			wantErr: true,
		},
		{
			name:    "youtu.be without path",
			input:   "https://youtu.be/",
			wantErr: true,
		},
		{
			name:    "too short to be video ID",
			input:   "abc",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeYouTubeURL(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFetchTranscript_Success(t *testing.T) {
	items := []actorOutput{
		{Title: "Test Video", ChannelName: "Test Channel", URL: "https://youtube.com/watch?v=abc", TranscriptRaw: "Hello world transcript text"},
	}
	itemsJSON, _ := json.Marshal(items)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, err := w.Write(itemsJSON); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	defer srv.Close()

	client := New("test-token")
	client.httpClient.Transport = rewriteTransport{target: srv.URL}

	result, err := FetchTranscript(context.Background(), client, "https://youtube.com/watch?v=abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Title != "Test Video" {
		t.Errorf("title = %q, want %q", result.Title, "Test Video")
	}
	if result.Channel != "Test Channel" {
		t.Errorf("channel = %q, want %q", result.Channel, "Test Channel")
	}
	if result.Content != "Hello world transcript text" {
		t.Errorf("content = %q, want %q", result.Content, "Hello world transcript text")
	}
}

func TestFetchTranscript_SegmentsFallback(t *testing.T) {
	type segmentOutput struct {
		Title    string `json:"title"`
		Segments []struct {
			Text string `json:"text"`
		} `json:"segments"`
	}
	items := []segmentOutput{
		{
			Title: "Segments Video",
			Segments: []struct {
				Text string `json:"text"`
			}{
				{Text: "Hello"},
				{Text: "world"},
			},
		},
	}
	itemsJSON, _ := json.Marshal(items)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, err := w.Write(itemsJSON); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	defer srv.Close()

	client := New("test-token")
	client.httpClient.Transport = rewriteTransport{target: srv.URL}

	result, err := FetchTranscript(context.Background(), client, "https://youtube.com/watch?v=abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Content != "Hello world" {
		t.Errorf("content = %q, want %q", result.Content, "Hello world")
	}
}

func TestFetchTranscript_EmptyDataset(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, err := w.Write([]byte(`[]`)); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	defer srv.Close()

	client := New("test-token")
	client.httpClient.Transport = rewriteTransport{target: srv.URL}

	_, err := FetchTranscript(context.Background(), client, "https://youtube.com/watch?v=abc")
	if err == nil {
		t.Fatal("expected error for empty dataset")
	}
	if !strings.Contains(err.Error(), "no transcript available") {
		t.Errorf("error = %q, want it to contain 'no transcript available'", err)
	}
}

func TestFetchTranscript_EmptyTranscript(t *testing.T) {
	items := []actorOutput{
		{Title: "No Captions", TranscriptRaw: ""},
	}
	itemsJSON, _ := json.Marshal(items)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, err := w.Write(itemsJSON); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	defer srv.Close()

	client := New("test-token")
	client.httpClient.Transport = rewriteTransport{target: srv.URL}

	_, err := FetchTranscript(context.Background(), client, "https://youtube.com/watch?v=abc")
	if err == nil {
		t.Fatal("expected error for empty transcript")
	}
	if !strings.Contains(err.Error(), "transcript is empty") {
		t.Errorf("error = %q, want it to contain 'transcript is empty'", err)
	}
}

func TestFetchTranscript_NilClient(t *testing.T) {
	_, err := FetchTranscript(context.Background(), nil, "https://youtube.com/watch?v=abc")
	if err == nil {
		t.Fatal("expected error for nil client")
	}
}

func TestFormatTranscript(t *testing.T) {
	tests := []struct {
		name   string
		result *TranscriptResult
		want   string
	}{
		{
			name: "full metadata",
			result: &TranscriptResult{
				Title:   "My Video",
				Channel: "My Channel",
				URL:     "https://youtube.com/watch?v=abc",
				Content: "transcript text",
			},
			want: "# My Video\nChannel: My Channel | URL: https://youtube.com/watch?v=abc\n\ntranscript text",
		},
		{
			name: "content only",
			result: &TranscriptResult{
				Content: "just the text",
			},
			want: "\njust the text",
		},
		{
			name: "title and content",
			result: &TranscriptResult{
				Title:   "Title Only",
				Content: "some text",
			},
			want: "# Title Only\n\nsome text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatTranscript(tt.result)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
