package llm

import "testing"

func TestContextWindowSize(t *testing.T) {
	tests := []struct {
		name        string
		modelName   string
		envOverride int
		want        int
	}{
		{
			name:      "known anthropic model",
			modelName: "claude-sonnet-4-6",
			want:      200_000,
		},
		{
			name:      "known openai model",
			modelName: "gpt-5.4",
			want:      1_047_576,
		},
		{
			name:      "known google model",
			modelName: "gemini-3.1-pro",
			want:      1_048_576,
		},
		{
			name:        "env override takes priority over known model",
			modelName:   "claude-sonnet-4-6",
			envOverride: 50_000,
			want:        50_000,
		},
		{
			name:        "env override for unknown model",
			modelName:   "custom-model-v1",
			envOverride: 64_000,
			want:        64_000,
		},
		{
			name:      "unknown model without override uses default",
			modelName: "custom-model-v1",
			want:      128_000,
		},
		{
			name:        "unknown model with zero override uses default",
			modelName:   "custom-model-v1",
			envOverride: 0,
			want:        128_000,
		},
		{
			name:        "unknown model with negative override uses default",
			modelName:   "custom-model-v1",
			envOverride: -1,
			want:        128_000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ContextWindowSize(tt.modelName, tt.envOverride)
			if got != tt.want {
				t.Errorf("ContextWindowSize(%q, %d) = %d, want %d", tt.modelName, tt.envOverride, got, tt.want)
			}
		})
	}
}
