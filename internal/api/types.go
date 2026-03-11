// Package api provides REST API handlers for Knowhow.
package api

import "time"

// Vault is the JSON representation of a vault.
type Vault struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description *string   `json:"description,omitempty"`
	CreatedBy   string    `json:"createdBy"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
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

// ServerConfig holds the server's effective configuration.
type ServerConfig struct {
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
