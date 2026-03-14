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

// Document is the JSON representation of a document.
type Document struct {
	ID          string    `json:"id"`
	VaultID     string    `json:"vaultId"`
	Path        string    `json:"path"`
	Title       string    `json:"title"`
	Content     string    `json:"content"`
	Source      string    `json:"source"`
	ContentHash *string   `json:"contentHash,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
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
	VaultID     string    `json:"vaultId"`
	Path        string    `json:"path"`
	MimeType    string    `json:"mimeType"`
	Size        int       `json:"size"`
	ContentHash string    `json:"contentHash"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
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
	TemplateCount     int   `json:"templateCount"`
	VersionCount      int   `json:"versionCount"`
	ConversationCount int   `json:"conversationCount"`
	TokenInput        int64 `json:"tokenInput"`
	TokenOutput       int64 `json:"tokenOutput"`
}

// SearchResultResponse is the JSON representation of a search result.
type SearchResultResponse struct {
	Path          string               `json:"path"`
	Title         string               `json:"title"`
	Score         float64              `json:"score"`
	MatchedChunks []ChunkMatchResponse `json:"matchedChunks"`
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
	Path string `json:"path"`
	Name string `json:"name"`
}

// VersionResponse is the JSON representation of a document version.
type VersionResponse struct {
	Version     int       `json:"version"`
	Title       string    `json:"title"`
	Source      string    `json:"source"`
	ContentHash string    `json:"contentHash"`
	CreatedAt   time.Time `json:"createdAt"`
}

// ServerConfig holds the server's effective configuration.
type ServerConfig struct {
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
}
