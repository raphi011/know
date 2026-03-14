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
