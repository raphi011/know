package models

import "testing"

func TestVaultRole_Level(t *testing.T) {
	tests := []struct {
		role VaultRole
		want int
	}{
		{RoleRead, 1},
		{RoleWrite, 2},
		{RoleAdmin, 3},
		{VaultRole("unknown"), 0},
		{VaultRole(""), 0},
	}
	for _, tt := range tests {
		if got := tt.role.Level(); got != tt.want {
			t.Errorf("VaultRole(%q).Level() = %d, want %d", tt.role, got, tt.want)
		}
	}
}

func TestVaultRole_Valid(t *testing.T) {
	if !RoleRead.Valid() {
		t.Error("RoleRead should be valid")
	}
	if !RoleWrite.Valid() {
		t.Error("RoleWrite should be valid")
	}
	if !RoleAdmin.Valid() {
		t.Error("RoleAdmin should be valid")
	}
	if VaultRole("unknown").Valid() {
		t.Error("unknown role should be invalid")
	}
	if VaultRole("").Valid() {
		t.Error("empty role should be invalid")
	}
}

func TestVaultRole_AtLeast(t *testing.T) {
	tests := []struct {
		role     VaultRole
		required VaultRole
		want     bool
	}{
		{RoleAdmin, RoleAdmin, true},
		{RoleAdmin, RoleWrite, true},
		{RoleAdmin, RoleRead, true},
		{RoleWrite, RoleAdmin, false},
		{RoleWrite, RoleWrite, true},
		{RoleWrite, RoleRead, true},
		{RoleRead, RoleAdmin, false},
		{RoleRead, RoleWrite, false},
		{RoleRead, RoleRead, true},
		// Invalid role has level 0 — fails all checks
		{VaultRole("bad"), RoleRead, false},
	}
	for _, tt := range tests {
		if got := tt.role.AtLeast(tt.required); got != tt.want {
			t.Errorf("%q.AtLeast(%q) = %v, want %v", tt.role, tt.required, got, tt.want)
		}
	}
}

func TestParseVaultRole(t *testing.T) {
	tests := []struct {
		input   string
		want    VaultRole
		wantErr bool
	}{
		{"read", RoleRead, false},
		{"write", RoleWrite, false},
		{"admin", RoleAdmin, false},
		{"", "", true},
		{"unknown", "", true},
		{"READ", "", true}, // case-sensitive
	}
	for _, tt := range tests {
		got, err := ParseVaultRole(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("ParseVaultRole(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			continue
		}
		if got != tt.want {
			t.Errorf("ParseVaultRole(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
