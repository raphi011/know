package models

import (
	"time"

	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
)

type Vault struct {
	ID          surrealmodels.RecordID `json:"id"`
	Name        string                 `json:"name"`
	Description *string                `json:"description,omitempty"`
	Settings    *VaultSettings         `json:"settings,omitempty"`
	CreatedBy   surrealmodels.RecordID `json:"created_by"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
}

// VaultSettings holds per-vault configuration.
type VaultSettings struct {
	MemoryPath             string  `json:"memory_path,omitempty"`
	MemoryMergeThreshold   float64 `json:"memory_merge_threshold,omitempty"`
	MemoryArchiveThreshold float64 `json:"memory_archive_threshold,omitempty"`
	MemoryDecayHalfLife    int     `json:"memory_decay_half_life,omitempty"`
}

// MemoryDefaults returns the vault's memory settings with defaults applied.
func (v *Vault) MemoryDefaults() VaultSettings {
	s := VaultSettings{
		MemoryPath:             "/memories",
		MemoryMergeThreshold:   0.95,
		MemoryArchiveThreshold: 0.2,
		MemoryDecayHalfLife:    30,
	}
	if v.Settings == nil {
		return s
	}
	if v.Settings.MemoryPath != "" {
		s.MemoryPath = v.Settings.MemoryPath
	}
	if v.Settings.MemoryMergeThreshold > 0 {
		s.MemoryMergeThreshold = v.Settings.MemoryMergeThreshold
	}
	if v.Settings.MemoryArchiveThreshold > 0 {
		s.MemoryArchiveThreshold = v.Settings.MemoryArchiveThreshold
	}
	if v.Settings.MemoryDecayHalfLife > 0 {
		s.MemoryDecayHalfLife = v.Settings.MemoryDecayHalfLife
	}
	return s
}

type VaultInput struct {
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
}
