package stt

import "fmt"

// NewTranscriber creates a Transcriber based on the provider name.
// Currently supports "openai". Returns an error for unknown providers.
// When baseURL is set (e.g. local whisper.cpp server), apiKey is optional.
func NewTranscriber(provider, model, apiKey, baseURL string) (Transcriber, error) {
	switch provider {
	case "openai":
		if apiKey == "" && baseURL == "" {
			return nil, fmt.Errorf("OPENAI_API_KEY required for STT provider 'openai' (or set KNOW_STT_BASE_URL for local server)")
		}
		return NewOpenAI(apiKey, model, baseURL), nil
	default:
		return nil, fmt.Errorf("unknown STT provider: %q", provider)
	}
}
