package models

import (
	"time"

	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
)

// MessageRole identifies the sender of a chat message.
type MessageRole string

const (
	RoleUser       MessageRole = "user"
	RoleAssistant  MessageRole = "assistant"
	RoleToolCall   MessageRole = "tool_call"
	RoleToolResult MessageRole = "tool_result"
)

// Conversation represents a multi-turn agent chat session.
type Conversation struct {
	ID        surrealmodels.RecordID `json:"id"`
	Vault     surrealmodels.RecordID `json:"vault"`
	User      surrealmodels.RecordID `json:"user"`
	Title     string                 `json:"title"`
	CreatedAt time.Time              `json:"created_at"`
	UpdatedAt time.Time              `json:"updated_at"`
}

// Message represents a single message in a conversation.
type Message struct {
	ID           surrealmodels.RecordID `json:"id"`
	Conversation surrealmodels.RecordID `json:"conversation"`
	Role         MessageRole            `json:"role"`
	Content      string                 `json:"content"`
	DocRefs      []string               `json:"doc_refs"`
	ToolName     *string                `json:"tool_name,omitempty"`
	ToolInput    *string                `json:"tool_input,omitempty"`
	ToolMeta     *string                `json:"tool_meta,omitempty"`
	ToolCallID   *string                `json:"tool_call_id,omitempty"`
	ToolCalls    *string                `json:"tool_calls,omitempty"`
	CreatedAt    time.Time              `json:"created_at"`
}
