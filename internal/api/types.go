// Package api provides REST API handlers for Know.
package api

import (
	"time"

	"github.com/raphi011/know/internal/models"
)

// Vault is the JSON representation of a vault.
type Vault struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description *string   `json:"description,omitempty"`
	CreatedBy   string    `json:"createdBy"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
	Remote      *string   `json:"remote,omitempty"`
}

// WikiLinkInfo is the JSON representation of an outgoing wiki-link.
type WikiLinkInfo struct {
	RawTarget string  `json:"rawTarget"`
	Path      *string `json:"path"`
	Title     *string `json:"title"`
}

// Document is the JSON representation of a document.
type Document struct {
	ID        string         `json:"id"`
	VaultID   string         `json:"vaultId"`
	Path      string         `json:"path"`
	Title     string         `json:"title"`
	Content   string         `json:"content"`
	Labels    []string       `json:"labels"`
	DocType   *string        `json:"docType,omitempty"`
	Hash      *string        `json:"hash,omitempty"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	WikiLinks []WikiLinkInfo `json:"wikiLinks,omitempty"`
}

// Conversation is the JSON representation of a conversation.
type Conversation struct {
	ID          string         `json:"id"`
	VaultID     string         `json:"vaultId"`
	Title       string         `json:"title"`
	TokenInput  int64          `json:"tokenInput"`
	TokenOutput int64          `json:"tokenOutput"`
	BgStatus    *string        `json:"bgStatus,omitempty"`
	CreatedAt   time.Time      `json:"createdAt"`
	UpdatedAt   time.Time      `json:"updatedAt"`
	Messages    []*ChatMessage `json:"messages,omitempty"`
}

// ChatMessage is the JSON representation of a chat message.
type ChatMessage struct {
	ID         string    `json:"id"`
	Role       string    `json:"role"`
	Content    string    `json:"content"`
	DocRefs    []string  `json:"docRefs"`
	ToolName   *string   `json:"toolName,omitempty"`
	ToolInput  *string   `json:"toolInput,omitempty"`
	ToolCallID *string   `json:"toolCallId,omitempty"`
	ToolCalls  *string   `json:"toolCalls,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
}

// AssetMeta is the JSON representation of asset metadata.
type AssetMeta struct {
	VaultID   string    `json:"vaultId"`
	Path      string    `json:"path"`
	MimeType  string    `json:"mimeType"`
	Size      int       `json:"size"`
	Hash      *string   `json:"hash,omitempty"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// VaultInfo holds comprehensive stats about a vault.
type VaultInfo struct {
	Name        string    `json:"name"`
	Description *string   `json:"description,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`

	// Documents & Chunks
	DocumentCount      int `json:"documentCount"`
	UnprocessedDocs    int `json:"unprocessedDocs"`
	ChunkTotal         int `json:"chunkTotal"`
	ChunkWithEmbedding int `json:"chunkWithEmbedding"`
	ChunkPending       int `json:"chunkPending"`

	// Labels
	LabelCount int                `json:"labelCount"`
	TopLabels  []models.LabelStat `json:"topLabels"`

	// Members
	Members []models.MemberStat `json:"members"`

	// Assets
	AssetCount     int   `json:"assetCount"`
	AssetTotalSize int64 `json:"assetTotalSize"`

	// Cross-references
	WikiLinkTotal  int `json:"wikiLinkTotal"`
	WikiLinkBroken int `json:"wikiLinkBroken"`

	// Other
	VersionCount      int   `json:"versionCount"`
	ConversationCount int   `json:"conversationCount"`
	TokenInput        int64 `json:"tokenInput"`
	TokenOutput       int64 `json:"tokenOutput"`
}

// SearchResultResponse is the JSON representation of a search result.
type SearchResultResponse struct {
	DocumentID    string               `json:"documentId"`
	Path          string               `json:"path"`
	Title         string               `json:"title"`
	Labels        []string             `json:"labels"`
	DocType       *string              `json:"docType,omitempty"`
	Score         float64              `json:"score"`
	MatchedChunks []ChunkMatchResponse `json:"matchedChunks"`
}

// nonNilLabels returns an empty slice instead of nil so JSON serialization
// produces [] instead of null.
func nonNilLabels(labels []string) []string {
	if labels == nil {
		return []string{}
	}
	return labels
}

// ChunkMatchResponse is the JSON representation of a matched chunk.
type ChunkMatchResponse struct {
	Snippet     string  `json:"snippet"`
	HeadingPath *string `json:"headingPath,omitempty"`
	Position    int     `json:"position"`
	Score       float64 `json:"score"`
}

// FolderResponse is the JSON representation of a folder.
type FolderResponse struct {
	Path    string `json:"path"`
	Name    string `json:"name"`
	NoEmbed bool   `json:"noEmbed"`
}

// VersionResponse is the JSON representation of a document version.
type VersionResponse struct {
	Version   int       `json:"version"`
	Title     string    `json:"title"`
	Hash      string    `json:"hash"`
	CreatedAt time.Time `json:"createdAt"`
}

// ChangesResponse is the JSON representation of incremental changes since a timestamp.
type ChangesResponse struct {
	Updated   []FileChange `json:"updated"`
	Deleted   []FileChange `json:"deleted"`
	SyncToken string       `json:"syncToken"` // RFC3339Nano — use as next "since" value
	Truncated bool         `json:"truncated"` // true if results were capped at the server limit
}

// FileChange represents a single file change in the changes response.
type FileChange struct {
	FileID    string    `json:"fileId"`
	Path      string    `json:"path"`
	Hash      *string   `json:"hash,omitempty"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// ServerConfig holds the server's effective configuration.
type ServerConfig struct {
	Version                string `json:"version"`
	Commit                 string `json:"commit"`
	SurrealDBURL           string `json:"surrealdbURL"`
	AuthEnabled            bool   `json:"authEnabled"`
	LLMProvider            string `json:"llmProvider"`
	LLMModel               string `json:"llmModel"`
	EmbedProvider          string `json:"embedProvider"`
	EmbedModel             string `json:"embedModel"`
	EmbedDimension         int    `json:"embedDimension"`
	SemanticSearchEnabled  bool   `json:"semanticSearchEnabled"`
	AgentChatEnabled       bool   `json:"agentChatEnabled"`
	WebSearchEnabled       bool   `json:"webSearchEnabled"`
	ChunkThreshold         int    `json:"chunkThreshold"`
	ChunkTargetSize        int    `json:"chunkTargetSize"`
	ChunkMaxSize           int    `json:"chunkMaxSize"`
	VersionCoalesceMinutes int    `json:"versionCoalesceMinutes"`
	VersionRetentionCount  int    `json:"versionRetentionCount"`

	// Audio pipeline
	STTProvider          string `json:"sttProvider"`
	STTModel             string `json:"sttModel"`
	TranscriptionEnabled bool   `json:"transcriptionEnabled"`
	FFmpegInstalled      bool   `json:"ffmpegInstalled"`

	// PDF pipeline
	PopplerInstalled        bool   `json:"popplerInstalled"`
	PDFIngestionEnabled     bool   `json:"pdfIngestionEnabled"`
	MultimodalEmbedProvider string `json:"multimodalEmbedProvider"`
	MultimodalEmbedModel    string `json:"multimodalEmbedModel"`
	MultimodalEmbedEnabled  bool   `json:"multimodalEmbedEnabled"`
	TextExtractorModel      string `json:"textExtractorModel"`
	TextExtractorEnabled    bool   `json:"textExtractorEnabled"`

	// Auth
	OIDCEnabled bool `json:"oidcEnabled"`
}
