package stt

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"time"
)

// OpenAITranscriber implements Transcriber using OpenAI's audio transcription API.
type OpenAITranscriber struct {
	apiKey  string
	model   string
	baseURL string
	client  *http.Client
}

// NewOpenAI creates an OpenAITranscriber with the given API key, model, and
// base URL. If model is empty, it defaults to "gpt-4o-transcribe". If baseURL
// is empty, it defaults to the OpenAI API.
func NewOpenAI(apiKey, model, baseURL string) *OpenAITranscriber {
	if model == "" {
		model = "gpt-4o-transcribe"
	}
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	return &OpenAITranscriber{
		apiKey:  apiKey,
		model:   model,
		baseURL: baseURL,
		client:  &http.Client{Timeout: 5 * time.Minute},
	}
}

// verboseResponse represents the verbose_json response from OpenAI's transcription API.
type verboseResponse struct {
	Text     string           `json:"text"`
	Segments []verboseSegment `json:"segments"`
}

type verboseSegment struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Text  string  `json:"text"`
}

// extForMIME maps a MIME type to a file extension for the multipart upload.
func extForMIME(mimeType string) string {
	switch mimeType {
	case "audio/mpeg":
		return ".mp3"
	case "audio/wav":
		return ".wav"
	case "audio/ogg":
		return ".ogg"
	case "audio/flac":
		return ".flac"
	case "audio/mp4":
		return ".m4a"
	case "audio/aac":
		return ".aac"
	case "audio/opus":
		return ".opus"
	case "audio/webm":
		return ".weba"
	default:
		return ".bin"
	}
}

// Transcribe sends audio bytes to the OpenAI transcription endpoint and returns
// a Result with the full text and timestamped segments.
func (t *OpenAITranscriber) Transcribe(ctx context.Context, audio []byte, mimeType string) (*Result, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	filename := "audio" + extForMIME(mimeType)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return nil, fmt.Errorf("create form file: %w", err)
	}
	if _, err := part.Write(audio); err != nil {
		return nil, fmt.Errorf("write audio: %w", err)
	}

	if err := writer.WriteField("model", t.model); err != nil {
		return nil, fmt.Errorf("write model field: %w", err)
	}
	if err := writer.WriteField("response_format", "verbose_json"); err != nil {
		return nil, fmt.Errorf("write response_format field: %w", err)
	}
	if err := writer.WriteField("timestamp_granularities[]", "segment"); err != nil {
		return nil, fmt.Errorf("write timestamp_granularities field: %w", err)
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("close multipart writer: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.baseURL+"/audio/transcriptions", body)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+t.apiKey)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 50<<20)) // 50MB cap
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("transcription failed (HTTP %d): %s", resp.StatusCode, respBody)
	}

	var vr verboseResponse
	if err := json.Unmarshal(respBody, &vr); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	result := &Result{
		Text:     vr.Text,
		Segments: make([]Segment, len(vr.Segments)),
	}
	for i, s := range vr.Segments {
		result.Segments[i] = Segment(s)
	}

	return result, nil
}
