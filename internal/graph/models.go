// Package graph provides GraphQL types and resolvers for Knowhow v2.
package graph

import "time"

type Vault struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description *string   `json:"description,omitempty"`
	CreatedBy   string    `json:"createdBy"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type Document struct {
	ID          string         `json:"id"`
	VaultID     string         `json:"vaultId"`
	Path        string         `json:"path"`
	Title       string         `json:"title"`
	Content     string         `json:"content"`
	ContentBody string         `json:"contentBody"`
	Labels      []string       `json:"labels"`
	DocType     *string        `json:"docType,omitempty"`
	Source      string         `json:"source"`
	SourcePath  *string        `json:"sourcePath,omitempty"`
	ContentHash *string        `json:"contentHash,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	CreatedAt   time.Time      `json:"createdAt"`
	UpdatedAt   time.Time      `json:"updatedAt"`
}

type Folder struct {
	Path     string `json:"path"`
	Name     string `json:"name"`
	DocCount int    `json:"docCount"`
}

type WikiLink struct {
	ID        string  `json:"id"`
	FromDocID string  `json:"fromDocId"`
	ToDocID   *string `json:"toDocId,omitempty"`
	RawTarget string  `json:"rawTarget"`
	Resolved  bool    `json:"resolved"`
}

type DocRelation struct {
	ID        string    `json:"id"`
	FromDocID string    `json:"fromDocId"`
	ToDocID   string    `json:"toDocId"`
	RelType   string    `json:"relType"`
	Source    string    `json:"source"`
	CreatedAt time.Time `json:"createdAt"`
}

type Template struct {
	ID           string    `json:"id"`
	VaultID      *string   `json:"vaultId,omitempty"`
	Name         string    `json:"name"`
	Description  *string   `json:"description,omitempty"`
	Content      string    `json:"content"`
	IsAITemplate bool      `json:"isAITemplate"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type SearchResult struct {
	DocumentID    string       `json:"documentId"`
	Path          string       `json:"path"`
	Title         string       `json:"title"`
	Labels        []string     `json:"labels"`
	DocType       *string      `json:"docType,omitempty"`
	Score         float64      `json:"score"`
	MatchedChunks []ChunkMatch `json:"matchedChunks"`
}

type ChunkMatch struct {
	Snippet     string  `json:"snippet"`
	HeadingPath *string `json:"headingPath,omitempty"`
	Position    int     `json:"position"`
	Score       float64 `json:"score"`
}

type User struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Email     *string   `json:"email,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

type Me struct {
	User        User     `json:"user"`
	VaultAccess []string `json:"vaultAccess"`
}

type ScrapeResult struct {
	FilesProcessed   int      `json:"filesProcessed"`
	FilesSkipped     int      `json:"filesSkipped"`
	DocumentsCreated int      `json:"documentsCreated"`
	ChunksCreated    int      `json:"chunksCreated"`
	Errors           []string `json:"errors"`
}

// Query block types

type QueryFormat string

const (
	QueryFormatList  QueryFormat = "LIST"
	QueryFormatTable QueryFormat = "TABLE"
)

type QueryBlock struct {
	Index    int           `json:"index"`
	RawQuery string        `json:"rawQuery"`
	Format   QueryFormat   `json:"format"`
	Results  []QueryResult `json:"results"`
	Error    *string       `json:"error,omitempty"`
}

type QueryResult struct {
	DocID  string         `json:"docId"`
	Title  string         `json:"title"`
	Path   string         `json:"path"`
	Fields map[string]any `json:"fields,omitempty"`
}

// Proposal types

type DocumentProposal struct {
	ID              string             `json:"id"`
	VaultID         string             `json:"vaultId"`
	DocumentID      string             `json:"documentId"`
	ProposedContent string             `json:"proposedContent"`
	Description     *string            `json:"description,omitempty"`
	Source          ProposalSourceEnum `json:"source"`
	Status          ProposalStatus     `json:"status"`
	OriginalHash    string             `json:"originalHash"`
	ReviewedAt      *time.Time         `json:"reviewedAt,omitempty"`
	ReviewerNotes   *string            `json:"reviewerNotes,omitempty"`
	CreatedAt       time.Time          `json:"createdAt"`
}

type ProposalStatus string

const (
	ProposalStatusPending           ProposalStatus = "PENDING"
	ProposalStatusApproved          ProposalStatus = "APPROVED"
	ProposalStatusPartiallyApproved ProposalStatus = "PARTIALLY_APPROVED"
	ProposalStatusRejected          ProposalStatus = "REJECTED"
	ProposalStatusConflict          ProposalStatus = "CONFLICT"
	ProposalStatusExpired           ProposalStatus = "EXPIRED"
)

var AllProposalStatus = []ProposalStatus{
	ProposalStatusPending,
	ProposalStatusApproved,
	ProposalStatusPartiallyApproved,
	ProposalStatusRejected,
	ProposalStatusConflict,
	ProposalStatusExpired,
}

func (e ProposalStatus) IsValid() bool {
	switch e {
	case ProposalStatusPending, ProposalStatusApproved, ProposalStatusPartiallyApproved,
		ProposalStatusRejected, ProposalStatusConflict, ProposalStatusExpired:
		return true
	}
	return false
}

func (e ProposalStatus) String() string { return string(e) }

type ProposalDiff struct {
	Hunks       []*DiffHunk `json:"hunks"`
	HasConflict bool        `json:"hasConflict"`
	Stats       *DiffStats  `json:"stats"`
}

type DiffStats struct {
	Additions  int `json:"additions"`
	Deletions  int `json:"deletions"`
	HunksCount int `json:"hunksCount"`
}

type DiffHunk struct {
	Index    int         `json:"index"`
	OldStart int         `json:"oldStart"`
	OldLines int         `json:"oldLines"`
	NewStart int         `json:"newStart"`
	NewLines int         `json:"newLines"`
	Header   string      `json:"header"`
	Lines    []*DiffLine `json:"lines"`
}

type DiffLine struct {
	Type      DiffLineTypeEnum `json:"type"`
	Content   string           `json:"content"`
	OldLineNo *int             `json:"oldLineNo,omitempty"`
	NewLineNo *int             `json:"newLineNo,omitempty"`
}

type DiffLineTypeEnum string

const (
	DiffLineTypeContext DiffLineTypeEnum = "CONTEXT"
	DiffLineTypeAdd     DiffLineTypeEnum = "ADD"
	DiffLineTypeDelete  DiffLineTypeEnum = "DELETE"
)

var AllDiffLineType = []DiffLineTypeEnum{
	DiffLineTypeContext,
	DiffLineTypeAdd,
	DiffLineTypeDelete,
}

func (e DiffLineTypeEnum) IsValid() bool {
	switch e {
	case DiffLineTypeContext, DiffLineTypeAdd, DiffLineTypeDelete:
		return true
	}
	return false
}

func (e DiffLineTypeEnum) String() string { return string(e) }

// ProposalSourceEnum represents how a proposal was created (GraphQL enum).
type ProposalSourceEnum string

const (
	ProposalSourceAISuggested ProposalSourceEnum = "AI_SUGGESTED"
	ProposalSourceAIGenerated ProposalSourceEnum = "AI_GENERATED"
	ProposalSourceImport      ProposalSourceEnum = "IMPORT"
)

var AllProposalSource = []ProposalSourceEnum{
	ProposalSourceAISuggested,
	ProposalSourceAIGenerated,
	ProposalSourceImport,
}

func (e ProposalSourceEnum) IsValid() bool {
	switch e {
	case ProposalSourceAISuggested, ProposalSourceAIGenerated, ProposalSourceImport:
		return true
	}
	return false
}

func (e ProposalSourceEnum) String() string { return string(e) }

// Proposal input types

type ProposeDocumentUpdateInput struct {
	VaultID         string              `json:"vaultId"`
	Path            string              `json:"path"`
	ProposedContent string              `json:"proposedContent"`
	Description     *string             `json:"description,omitempty"`
	Source          *ProposalSourceEnum `json:"source,omitempty"`
}

type ApproveHunksInput struct {
	ProposalID  string  `json:"proposalId"`
	HunkIndexes []int   `json:"hunkIndexes"`
	Notes       *string `json:"notes,omitempty"`
}

// Input types

type VaultInput struct {
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
}

type SearchInput struct {
	VaultID string   `json:"vaultId"`
	Query   string   `json:"query"`
	Labels  []string `json:"labels,omitempty"`
	DocType *string  `json:"docType,omitempty"`
	Folder  *string  `json:"folder,omitempty"`
	Limit   *int     `json:"limit,omitempty"`
}

type FileInput struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type RelationInput struct {
	FromDocID string `json:"fromDocId"`
	ToDocID   string `json:"toDocId"`
	RelType   string `json:"relType"`
}

type TemplateInput struct {
	VaultID      *string `json:"vaultId,omitempty"`
	Name         string  `json:"name"`
	Description  *string `json:"description,omitempty"`
	Content      string  `json:"content"`
	IsAITemplate *bool   `json:"isAITemplate,omitempty"`
}
