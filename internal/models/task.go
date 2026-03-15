package models

import (
	"time"

	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
)

const (
	TaskStatusOpen = "open"
	TaskStatusDone = "done"
)

// Task represents a markdown checkbox extracted from a document.
type Task struct {
	ID          surrealmodels.RecordID `json:"id"`
	Document    surrealmodels.RecordID `json:"document"`
	Vault       surrealmodels.RecordID `json:"vault"`
	Status      string                 `json:"status"`       // "open" or "done"
	RawLine     string                 `json:"raw_line"`     // original markdown line
	Text        string                 `json:"text"`         // cleaned text (no checkbox marker/metadata)
	Labels      []string               `json:"labels"`       // inline #labels
	DueDate     *string                `json:"due_date"`     // "YYYY-MM-DD" or nil
	LineNumber  int                    `json:"line_number"`  // 1-based position in document
	HeadingPath *string                `json:"heading_path"` // section context, e.g. "Daily Note > Tasks"
	ContentHash string                 `json:"content_hash"` // SHA256 of text with #labels and due:dates stripped, for stable identity
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
}

// TaskInput is used to create a new task record.
type TaskInput struct {
	DocumentID  string   `json:"document_id"`
	VaultID     string   `json:"vault_id"`
	Status      string   `json:"status"`
	RawLine     string   `json:"raw_line"`
	Text        string   `json:"text"`
	Labels      []string `json:"labels"`
	DueDate     *string  `json:"due_date"`
	LineNumber  int      `json:"line_number"`
	HeadingPath *string  `json:"heading_path"`
	ContentHash string   `json:"content_hash"`
}

// TaskWithDoc extends Task with denormalized document fields for list queries.
type TaskWithDoc struct {
	Task
	DocPath  string `json:"doc_path"`
	DocTitle string `json:"doc_title"`
}
