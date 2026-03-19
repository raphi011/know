package models

import (
	"fmt"
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
	TemplatePath           string  `json:"template_path,omitempty"`
	DailyNotePath          string  `json:"daily_note_path,omitempty"`
	TranscriptTemplate     string  `json:"transcript_template,omitempty"` // path to template for LLM transcript summarization (empty = disabled)

	// Search tuning
	RRFK               int `json:"rrf_k,omitempty"`
	HNSWEF             int `json:"hnsw_ef,omitempty"`
	DefaultSearchLimit int `json:"default_search_limit,omitempty"`
	MaxSearchLimit     int `json:"max_search_limit,omitempty"`

	// Versioning
	VersionCoalesceMinutes int `json:"version_coalesce_minutes,omitempty"`
	VersionRetentionCount  int `json:"version_retention_count,omitempty"`

	// Web clipping
	WebClipPath string `json:"web_clip_path,omitempty"`

	// Feature toggles (nil = use server default, which is enabled)
	AgentEnabled     *bool `json:"agent_enabled,omitempty"`
	EmbeddingEnabled *bool `json:"embedding_enabled,omitempty"`
	MCPEnabled       *bool `json:"mcp_enabled,omitempty"`
}

const (
	// DefaultTemplatePath is the default folder for template documents.
	DefaultTemplatePath = "/templates"
	// DefaultDailyNotePath is the default folder for daily notes.
	DefaultDailyNotePath = "/daily"
	// DefaultMemoryPath is the default folder for memories.
	DefaultMemoryPath = "/memories"
	// DefaultMemoryMergeThreshold is the cosine similarity above which memories are merge candidates.
	DefaultMemoryMergeThreshold = 0.95
	// DefaultMemoryArchiveThreshold is the score below which memories are auto-archived.
	DefaultMemoryArchiveThreshold = 0.2
	// DefaultMemoryDecayHalfLife is the half-life in days for memory recency decay.
	DefaultMemoryDecayHalfLife = 30
	// DefaultRRFK is the default RRF K parameter for hybrid search fusion.
	DefaultRRFK = 60
	// DefaultHNSWEF is the default HNSW EF parameter for vector search.
	DefaultHNSWEF = 40
	// DefaultSearchLimit is the default number of search results.
	DefaultSearchLimit = 20
	// DefaultMaxSearchLimit is the maximum allowed search result limit.
	DefaultMaxSearchLimit = 100
	// DefaultVersionCoalesceMinutes is the default coalescing window for version snapshots.
	DefaultVersionCoalesceMinutes = 10
	// DefaultVersionRetentionCount is the default max versions per file.
	DefaultVersionRetentionCount = 50
	// DefaultWebClipPath is the default folder for web-clipped pages.
	DefaultWebClipPath = "/web"
)

// Validate checks that all non-zero fields in VaultSettings are within valid ranges.
func (s VaultSettings) Validate() error {
	if s.MemoryMergeThreshold < 0 || s.MemoryMergeThreshold > 1 {
		return fmt.Errorf("memory_merge_threshold must be between 0 and 1, got %g", s.MemoryMergeThreshold)
	}
	if s.MemoryArchiveThreshold < 0 || s.MemoryArchiveThreshold > 1 {
		return fmt.Errorf("memory_archive_threshold must be between 0 and 1, got %g", s.MemoryArchiveThreshold)
	}
	if s.MemoryDecayHalfLife < 0 {
		return fmt.Errorf("memory_decay_half_life must be non-negative, got %d", s.MemoryDecayHalfLife)
	}
	if s.RRFK < 0 {
		return fmt.Errorf("rrf_k must be non-negative, got %d", s.RRFK)
	}
	if s.HNSWEF < 0 {
		return fmt.Errorf("hnsw_ef must be non-negative, got %d", s.HNSWEF)
	}
	if s.DefaultSearchLimit < 0 {
		return fmt.Errorf("default_search_limit must be non-negative, got %d", s.DefaultSearchLimit)
	}
	if s.MaxSearchLimit < 0 {
		return fmt.Errorf("max_search_limit must be non-negative, got %d", s.MaxSearchLimit)
	}
	if s.DefaultSearchLimit > 0 && s.MaxSearchLimit > 0 && s.DefaultSearchLimit > s.MaxSearchLimit {
		return fmt.Errorf("default_search_limit (%d) must not exceed max_search_limit (%d)", s.DefaultSearchLimit, s.MaxSearchLimit)
	}
	if s.VersionCoalesceMinutes < 0 {
		return fmt.Errorf("version_coalesce_minutes must be non-negative, got %d", s.VersionCoalesceMinutes)
	}
	if s.VersionRetentionCount < 0 {
		return fmt.Errorf("version_retention_count must be non-negative, got %d", s.VersionRetentionCount)
	}
	return nil
}

// Merge overlays non-zero fields from patch onto s and returns the result.
func (s VaultSettings) Merge(patch VaultSettings) VaultSettings {
	if patch.MemoryPath != "" {
		s.MemoryPath = patch.MemoryPath
	}
	if patch.MemoryMergeThreshold > 0 {
		s.MemoryMergeThreshold = patch.MemoryMergeThreshold
	}
	if patch.MemoryArchiveThreshold > 0 {
		s.MemoryArchiveThreshold = patch.MemoryArchiveThreshold
	}
	if patch.MemoryDecayHalfLife > 0 {
		s.MemoryDecayHalfLife = patch.MemoryDecayHalfLife
	}
	if patch.TemplatePath != "" {
		s.TemplatePath = patch.TemplatePath
	}
	if patch.DailyNotePath != "" {
		s.DailyNotePath = patch.DailyNotePath
	}
	if patch.TranscriptTemplate == "-" {
		s.TranscriptTemplate = ""
	} else if patch.TranscriptTemplate != "" {
		s.TranscriptTemplate = patch.TranscriptTemplate
	}
	if patch.RRFK > 0 {
		s.RRFK = patch.RRFK
	}
	if patch.HNSWEF > 0 {
		s.HNSWEF = patch.HNSWEF
	}
	if patch.DefaultSearchLimit > 0 {
		s.DefaultSearchLimit = patch.DefaultSearchLimit
	}
	if patch.MaxSearchLimit > 0 {
		s.MaxSearchLimit = patch.MaxSearchLimit
	}
	if patch.VersionCoalesceMinutes > 0 {
		s.VersionCoalesceMinutes = patch.VersionCoalesceMinutes
	}
	if patch.VersionRetentionCount > 0 {
		s.VersionRetentionCount = patch.VersionRetentionCount
	}
	if patch.WebClipPath != "" {
		s.WebClipPath = patch.WebClipPath
	}
	if patch.AgentEnabled != nil {
		s.AgentEnabled = patch.AgentEnabled
	}
	if patch.EmbeddingEnabled != nil {
		s.EmbeddingEnabled = patch.EmbeddingEnabled
	}
	if patch.MCPEnabled != nil {
		s.MCPEnabled = patch.MCPEnabled
	}
	return s
}

// Defaults returns the vault's settings with all defaults applied.
func (v *Vault) Defaults() VaultSettings {
	s := VaultSettings{
		MemoryPath:             DefaultMemoryPath,
		MemoryMergeThreshold:   DefaultMemoryMergeThreshold,
		MemoryArchiveThreshold: DefaultMemoryArchiveThreshold,
		MemoryDecayHalfLife:    DefaultMemoryDecayHalfLife,
		TemplatePath:           DefaultTemplatePath,
		DailyNotePath:          DefaultDailyNotePath,
		RRFK:                   DefaultRRFK,
		HNSWEF:                 DefaultHNSWEF,
		DefaultSearchLimit:     DefaultSearchLimit,
		MaxSearchLimit:         DefaultMaxSearchLimit,
		VersionCoalesceMinutes: DefaultVersionCoalesceMinutes,
		VersionRetentionCount:  DefaultVersionRetentionCount,
		WebClipPath:            DefaultWebClipPath,
	}
	if v.Settings == nil {
		return s
	}
	return s.Merge(*v.Settings)
}

// IsAgentEnabled returns whether agent chat is enabled for this vault.
// Defaults to true if not explicitly set.
func (s VaultSettings) IsAgentEnabled() bool {
	return s.AgentEnabled == nil || *s.AgentEnabled
}

// IsEmbeddingEnabled returns whether embedding generation is enabled for this vault.
// Defaults to true if not explicitly set.
func (s VaultSettings) IsEmbeddingEnabled() bool {
	return s.EmbeddingEnabled == nil || *s.EmbeddingEnabled
}

// IsMCPEnabled returns whether MCP tool access is enabled for this vault.
// Defaults to true if not explicitly set.
func (s VaultSettings) IsMCPEnabled() bool {
	return s.MCPEnabled == nil || *s.MCPEnabled
}

// MemoryDefaults returns the vault's memory settings with defaults applied.
func (v *Vault) MemoryDefaults() VaultSettings {
	return v.Defaults()
}

// TemplatePath returns the vault's configured template path, or the default.
func (v *Vault) TemplatePath() string {
	return v.Defaults().TemplatePath
}

// DailyNotePath returns the vault's configured daily note path, or the default.
func (v *Vault) DailyNotePath() string {
	return v.Defaults().DailyNotePath
}

type VaultInput struct {
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
}
