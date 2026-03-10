package config

import (
	"testing"
)

func TestEffectiveChunkConfig_NoLimit(t *testing.T) {
	cfg := Config{
		ChunkThreshold:     6000,
		ChunkTargetSize:    3000,
		ChunkMaxSize:       4000,
		EmbedMaxInputChars: 0,
	}
	cc := cfg.EffectiveChunkConfig()
	if cc.MaxSize != 4000 {
		t.Errorf("MaxSize = %d, want 4000", cc.MaxSize)
	}
	if cc.TargetSize != 3000 {
		t.Errorf("TargetSize = %d, want 3000", cc.TargetSize)
	}
}

func TestEffectiveChunkConfig_CapsMaxSize(t *testing.T) {
	cfg := Config{
		ChunkThreshold:     6000,
		ChunkTargetSize:    3000,
		ChunkMaxSize:       4000,
		EmbedMaxInputChars: 2048,
	}
	cc := cfg.EffectiveChunkConfig()
	wantMax := 2048 - maxEmbedContextOverhead // 1798
	if cc.MaxSize != wantMax {
		t.Errorf("MaxSize = %d, want %d", cc.MaxSize, wantMax)
	}
}

func TestEffectiveChunkConfig_AdjustsTargetSize(t *testing.T) {
	cfg := Config{
		ChunkThreshold:     6000,
		ChunkTargetSize:    3000,
		ChunkMaxSize:       4000,
		EmbedMaxInputChars: 2048,
	}
	cc := cfg.EffectiveChunkConfig()
	// TargetSize (3000) >= MaxSize (1798), so it should be adjusted to 3/4 of MaxSize
	wantTarget := cc.MaxSize * 3 / 4
	if cc.TargetSize != wantTarget {
		t.Errorf("TargetSize = %d, want %d", cc.TargetSize, wantTarget)
	}
}

func TestEffectiveChunkConfig_TargetAlreadyBelow(t *testing.T) {
	cfg := Config{
		ChunkThreshold:     6000,
		ChunkTargetSize:    500,
		ChunkMaxSize:       4000,
		EmbedMaxInputChars: 2048,
	}
	cc := cfg.EffectiveChunkConfig()
	// TargetSize (500) < MaxSize (1798), so it stays unchanged
	if cc.TargetSize != 500 {
		t.Errorf("TargetSize = %d, want 500", cc.TargetSize)
	}
}

func TestEffectiveChunkConfig_SmallLimit(t *testing.T) {
	cfg := Config{
		ChunkThreshold:     6000,
		ChunkTargetSize:    3000,
		ChunkMaxSize:       4000,
		EmbedMaxInputChars: 300, // very small limit
	}
	cc := cfg.EffectiveChunkConfig()
	// contentBudget = 300 - 250 = 50, but clamped to 100
	if cc.MaxSize != 100 {
		t.Errorf("MaxSize = %d, want 100", cc.MaxSize)
	}
}
