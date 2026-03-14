package models

import "testing"

func TestVaultTemplatePath(t *testing.T) {
	tests := []struct {
		name     string
		settings *VaultSettings
		want     string
	}{
		{"nil settings", nil, DefaultTemplatePath},
		{"empty template path", &VaultSettings{}, DefaultTemplatePath},
		{"custom path", &VaultSettings{TemplatePath: "/custom-templates"}, "/custom-templates"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := &Vault{Settings: tt.settings}
			if got := v.TemplatePath(); got != tt.want {
				t.Errorf("TemplatePath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestVaultDefaults(t *testing.T) {
	t.Run("nil settings returns all defaults", func(t *testing.T) {
		v := &Vault{}
		d := v.Defaults()
		if d.MemoryPath != DefaultMemoryPath {
			t.Errorf("MemoryPath = %q, want %q", d.MemoryPath, DefaultMemoryPath)
		}
		if d.TemplatePath != DefaultTemplatePath {
			t.Errorf("TemplatePath = %q, want %q", d.TemplatePath, DefaultTemplatePath)
		}
		if d.DailyNotePath != DefaultDailyNotePath {
			t.Errorf("DailyNotePath = %q, want %q", d.DailyNotePath, DefaultDailyNotePath)
		}
		if d.MemoryDecayHalfLife != 30 {
			t.Errorf("MemoryDecayHalfLife = %d, want 30", d.MemoryDecayHalfLife)
		}
		if d.MemoryMergeThreshold != 0.95 {
			t.Errorf("MemoryMergeThreshold = %f, want 0.95", d.MemoryMergeThreshold)
		}
		if d.MemoryArchiveThreshold != 0.2 {
			t.Errorf("MemoryArchiveThreshold = %f, want 0.2", d.MemoryArchiveThreshold)
		}
	})

	t.Run("custom settings override defaults", func(t *testing.T) {
		v := &Vault{Settings: &VaultSettings{
			MemoryPath:             "/custom-mem",
			TemplatePath:           "/custom-tpl",
			DailyNotePath:          "/custom-daily",
			MemoryDecayHalfLife:    60,
			MemoryMergeThreshold:   0.8,
			MemoryArchiveThreshold: 0.1,
		}}
		d := v.Defaults()
		if d.MemoryPath != "/custom-mem" {
			t.Errorf("MemoryPath = %q, want /custom-mem", d.MemoryPath)
		}
		if d.TemplatePath != "/custom-tpl" {
			t.Errorf("TemplatePath = %q, want /custom-tpl", d.TemplatePath)
		}
		if d.DailyNotePath != "/custom-daily" {
			t.Errorf("DailyNotePath = %q, want /custom-daily", d.DailyNotePath)
		}
		if d.MemoryDecayHalfLife != 60 {
			t.Errorf("MemoryDecayHalfLife = %d, want 60", d.MemoryDecayHalfLife)
		}
		if d.MemoryMergeThreshold != 0.8 {
			t.Errorf("MemoryMergeThreshold = %f, want 0.8", d.MemoryMergeThreshold)
		}
		if d.MemoryArchiveThreshold != 0.1 {
			t.Errorf("MemoryArchiveThreshold = %f, want 0.1", d.MemoryArchiveThreshold)
		}
	})

	t.Run("MemoryDefaults delegates to Defaults", func(t *testing.T) {
		v := &Vault{Settings: &VaultSettings{MemoryPath: "/custom"}}
		md := v.MemoryDefaults()
		d := v.Defaults()
		if md != d {
			t.Errorf("MemoryDefaults() != Defaults()")
		}
	})
}

func TestVaultDailyNotePath(t *testing.T) {
	tests := []struct {
		name     string
		settings *VaultSettings
		want     string
	}{
		{"nil settings", nil, DefaultDailyNotePath},
		{"empty daily path", &VaultSettings{}, DefaultDailyNotePath},
		{"custom path", &VaultSettings{DailyNotePath: "/notes"}, "/notes"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := &Vault{Settings: tt.settings}
			if got := v.DailyNotePath(); got != tt.want {
				t.Errorf("DailyNotePath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestVaultSettingsValidate(t *testing.T) {
	tests := []struct {
		name    string
		s       VaultSettings
		wantErr bool
	}{
		{"valid defaults", VaultSettings{MemoryMergeThreshold: 0.95, MemoryArchiveThreshold: 0.2, MemoryDecayHalfLife: 30}, false},
		{"zero values", VaultSettings{}, false},
		{"merge threshold too high", VaultSettings{MemoryMergeThreshold: 1.5}, true},
		{"merge threshold negative", VaultSettings{MemoryMergeThreshold: -0.1}, true},
		{"archive threshold too high", VaultSettings{MemoryArchiveThreshold: 2.0}, true},
		{"archive threshold negative", VaultSettings{MemoryArchiveThreshold: -0.5}, true},
		{"negative decay half life", VaultSettings{MemoryDecayHalfLife: -1}, true},
		{"zero decay half life", VaultSettings{MemoryDecayHalfLife: 0}, false},
		{"boundary merge threshold 0", VaultSettings{MemoryMergeThreshold: 0}, false},
		{"boundary merge threshold 1", VaultSettings{MemoryMergeThreshold: 1}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.s.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestVaultSettingsMerge(t *testing.T) {
	base := VaultSettings{
		MemoryPath:             "/memories",
		MemoryMergeThreshold:   0.95,
		MemoryArchiveThreshold: 0.2,
		MemoryDecayHalfLife:    30,
		TemplatePath:           "/templates",
		DailyNotePath:          "/daily",
	}

	t.Run("empty patch changes nothing", func(t *testing.T) {
		got := base.Merge(VaultSettings{})
		if got != base {
			t.Errorf("Merge(empty) changed values: got %+v", got)
		}
	})

	t.Run("patch overrides specific fields", func(t *testing.T) {
		got := base.Merge(VaultSettings{
			DailyNotePath:       "/journal",
			MemoryDecayHalfLife: 60,
		})
		if got.DailyNotePath != "/journal" {
			t.Errorf("DailyNotePath = %q, want /journal", got.DailyNotePath)
		}
		if got.MemoryDecayHalfLife != 60 {
			t.Errorf("MemoryDecayHalfLife = %d, want 60", got.MemoryDecayHalfLife)
		}
		// Unchanged fields preserved
		if got.TemplatePath != "/templates" {
			t.Errorf("TemplatePath = %q, want /templates", got.TemplatePath)
		}
	})
}
