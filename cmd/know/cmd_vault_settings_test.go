package main

import (
	"testing"
)

func TestParseSettingsPatch(t *testing.T) {
	t.Run("string settings", func(t *testing.T) {
		s, err := parseSettingsPatch([]string{"memory_path=/mem", "template_path=/tpl", "daily_note_path=/daily"})
		if err != nil {
			t.Fatal(err)
		}
		if s.MemoryPath != "/mem" {
			t.Errorf("MemoryPath = %q, want /mem", s.MemoryPath)
		}
		if s.TemplatePath != "/tpl" {
			t.Errorf("TemplatePath = %q, want /tpl", s.TemplatePath)
		}
		if s.DailyNotePath != "/daily" {
			t.Errorf("DailyNotePath = %q, want /daily", s.DailyNotePath)
		}
	})

	t.Run("numeric settings", func(t *testing.T) {
		s, err := parseSettingsPatch([]string{"memory_merge_threshold=0.8", "memory_archive_threshold=0.1", "memory_decay_half_life=60"})
		if err != nil {
			t.Fatal(err)
		}
		if s.MemoryMergeThreshold != 0.8 {
			t.Errorf("MemoryMergeThreshold = %f, want 0.8", s.MemoryMergeThreshold)
		}
		if s.MemoryArchiveThreshold != 0.1 {
			t.Errorf("MemoryArchiveThreshold = %f, want 0.1", s.MemoryArchiveThreshold)
		}
		if s.MemoryDecayHalfLife != 60 {
			t.Errorf("MemoryDecayHalfLife = %d, want 60", s.MemoryDecayHalfLife)
		}
	})

	errTests := []struct {
		name  string
		pairs []string
	}{
		{"missing equals", []string{"memory_path"}},
		{"unknown key", []string{"unknown_key=value"}},
		{"invalid float", []string{"memory_merge_threshold=abc"}},
		{"invalid int", []string{"memory_decay_half_life=abc"}},
	}
	for _, tt := range errTests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseSettingsPatch(tt.pairs)
			if err == nil {
				t.Error("expected error")
			}
		})
	}
}
