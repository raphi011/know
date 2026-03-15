package models

import (
	"fmt"
	"time"

	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
)

// TaskStatus represents the status of a task checkbox.
type TaskStatus string

const (
	TaskStatusOpen TaskStatus = "open"
	TaskStatusDone TaskStatus = "done"
)

// Valid returns true if the status is a recognized value.
func (s TaskStatus) Valid() bool {
	return s == TaskStatusOpen || s == TaskStatusDone
}

// IsValidDate checks whether a string is a valid YYYY-MM-DD date.
func IsValidDate(s string) bool {
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}

// Task represents a markdown checkbox extracted from a document.
type Task struct {
	ID          surrealmodels.RecordID `json:"id"`
	Document    surrealmodels.RecordID `json:"document"`
	Vault       surrealmodels.RecordID `json:"vault"`
	Status      TaskStatus             `json:"status"`       // "open" or "done"
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
	DocumentID  string     `json:"document_id"`
	VaultID     string     `json:"vault_id"`
	Status      TaskStatus `json:"status"`
	RawLine     string     `json:"raw_line"`
	Text        string     `json:"text"`
	Labels      []string   `json:"labels"`
	DueDate     *string    `json:"due_date"`
	LineNumber  int        `json:"line_number"`
	HeadingPath *string    `json:"heading_path"`
	ContentHash string     `json:"content_hash"`
}

// Validate checks that TaskInput fields are within expected bounds.
func (t TaskInput) Validate() error {
	if !t.Status.Valid() {
		return fmt.Errorf("invalid status %q", t.Status)
	}
	if t.ContentHash == "" {
		return fmt.Errorf("content hash required")
	}
	if t.LineNumber < 1 {
		return fmt.Errorf("line number must be positive")
	}
	if t.DueDate != nil && !IsValidDate(*t.DueDate) {
		return fmt.Errorf("invalid due date %q", *t.DueDate)
	}
	return nil
}

// TaskWithDoc extends Task with denormalized document fields for list queries.
type TaskWithDoc struct {
	Task
	DocPath  string `json:"doc_path"`
	DocTitle string `json:"doc_title"`
}
