package stt

import (
	"testing"
)

func TestNewTranscriber(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		model    string
		apiKey   string
		baseURL  string
		wantErr  bool
	}{
		{"openai with api key", "openai", "", "sk-test", "", false},
		{"openai with base url only", "openai", "", "", "http://localhost:8090", false},
		{"openai with both", "openai", "whisper-1", "sk-test", "http://localhost:8090", false},
		{"openai missing both", "openai", "", "", "", true},
		{"unknown provider", "whisper-local", "", "key", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr, err := NewTranscriber(tt.provider, tt.model, tt.apiKey, tt.baseURL)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewTranscriber() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && tr == nil {
				t.Error("NewTranscriber() returned nil without error")
			}
		})
	}
}
