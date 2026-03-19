package config

import (
	"testing"
)

func TestEffectiveChunkConfig(t *testing.T) {
	tests := []struct {
		name           string
		cfg            Config
		wantThreshold  int
		wantMaxSize    int
		wantTargetSize int
	}{
		{
			name: "no limit passes through unchanged",
			cfg: Config{
				ChunkThreshold:     6000,
				ChunkTargetSize:    3000,
				ChunkMaxSize:       4000,
				EmbedMaxInputChars: 0,
			},
			wantThreshold:  6000,
			wantMaxSize:    4000,
			wantTargetSize: 3000,
		},
		{
			name: "caps MaxSize and Threshold",
			cfg: Config{
				ChunkThreshold:     6000,
				ChunkTargetSize:    3000,
				ChunkMaxSize:       4000,
				EmbedMaxInputChars: 2048,
			},
			wantThreshold:  2048 - maxEmbedContextOverhead, // 1798
			wantMaxSize:    2048 - maxEmbedContextOverhead, // 1798
			wantTargetSize: (2048 - maxEmbedContextOverhead) * 3 / 4,
		},
		{
			name: "adjusts TargetSize when it exceeds capped MaxSize",
			cfg: Config{
				ChunkThreshold:     6000,
				ChunkTargetSize:    3000,
				ChunkMaxSize:       4000,
				EmbedMaxInputChars: 2048,
			},
			wantThreshold:  2048 - maxEmbedContextOverhead,
			wantMaxSize:    2048 - maxEmbedContextOverhead,
			wantTargetSize: (2048 - maxEmbedContextOverhead) * 3 / 4,
		},
		{
			name: "TargetSize below capped MaxSize stays unchanged",
			cfg: Config{
				ChunkThreshold:     6000,
				ChunkTargetSize:    500,
				ChunkMaxSize:       4000,
				EmbedMaxInputChars: 2048,
			},
			wantThreshold:  2048 - maxEmbedContextOverhead,
			wantMaxSize:    2048 - maxEmbedContextOverhead,
			wantTargetSize: 500,
		},
		{
			name: "small limit clamps contentBudget to 100",
			cfg: Config{
				ChunkThreshold:     6000,
				ChunkTargetSize:    3000,
				ChunkMaxSize:       4000,
				EmbedMaxInputChars: 300,
			},
			wantThreshold:  100,
			wantMaxSize:    100,
			wantTargetSize: 75, // 100 * 3/4
		},
		{
			name: "generous limit does not cap anything",
			cfg: Config{
				ChunkThreshold:     6000,
				ChunkTargetSize:    3000,
				ChunkMaxSize:       4000,
				EmbedMaxInputChars: 10000,
			},
			wantThreshold:  6000,
			wantMaxSize:    4000,
			wantTargetSize: 3000,
		},
		{
			name: "Threshold already below content budget stays unchanged",
			cfg: Config{
				ChunkThreshold:     1000,
				ChunkTargetSize:    500,
				ChunkMaxSize:       4000,
				EmbedMaxInputChars: 2048,
			},
			wantThreshold:  1000,
			wantMaxSize:    2048 - maxEmbedContextOverhead,
			wantTargetSize: 500,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cc := tt.cfg.EffectiveChunkConfig()
			if cc.Threshold != tt.wantThreshold {
				t.Errorf("Threshold = %d, want %d", cc.Threshold, tt.wantThreshold)
			}
			if cc.MaxSize != tt.wantMaxSize {
				t.Errorf("MaxSize = %d, want %d", cc.MaxSize, tt.wantMaxSize)
			}
			if cc.TargetSize != tt.wantTargetSize {
				t.Errorf("TargetSize = %d, want %d", cc.TargetSize, tt.wantTargetSize)
			}
		})
	}
}

func TestIsProduction(t *testing.T) {
	tests := []struct {
		env  string
		want bool
	}{
		{"production", true},
		{"development", false},
		{"", false},
		{"Production", false}, // case-sensitive
		{"staging", false},
	}
	for _, tt := range tests {
		t.Run(tt.env, func(t *testing.T) {
			c := Config{Environment: tt.env}
			if got := c.IsProduction(); got != tt.want {
				t.Errorf("IsProduction() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEffectiveChunkConfig_PassesValidate(t *testing.T) {
	// EffectiveChunkConfig output must always pass Validate for reasonable inputs.
	limits := []int{0, 300, 500, 1000, 2048, 5000, 10000}
	for _, limit := range limits {
		cfg := Config{
			ChunkThreshold:     6000,
			ChunkTargetSize:    3000,
			ChunkMaxSize:       4000,
			EmbedMaxInputChars: limit,
		}
		cc := cfg.EffectiveChunkConfig()
		if err := cc.Validate(); err != nil {
			t.Errorf("EmbedMaxInputChars=%d: EffectiveChunkConfig().Validate() = %v", limit, err)
		}
	}
}
