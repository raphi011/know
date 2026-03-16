package stt

import "fmt"

// NewTranscriber creates a Transcriber based on the provider name.
// Currently supports "openai". Returns an error for unknown providers.
func NewTranscriber(provider, model, apiKey string) (Transcriber, error) {
	switch provider {
	case "openai":
		if apiKey == "" {
			return nil, fmt.Errorf("OPENAI_API_KEY required for STT provider 'openai'")
		}
		return NewOpenAI(apiKey, model), nil
	default:
		return nil, fmt.Errorf("unknown STT provider: %q", provider)
	}
}
