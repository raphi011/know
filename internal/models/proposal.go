package models

import (
	"fmt"
	"time"

	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
)

// ProposalStatus represents the review state of a document proposal.
type ProposalStatus string

const (
	ProposalPending           ProposalStatus = "pending"
	ProposalApproved          ProposalStatus = "approved"
	ProposalPartiallyApproved ProposalStatus = "partially_approved"
	ProposalRejected          ProposalStatus = "rejected"
	ProposalConflict          ProposalStatus = "conflict"
	ProposalExpired           ProposalStatus = "expired"
)

// Valid returns true if the status is a known value.
func (s ProposalStatus) Valid() bool {
	switch s {
	case ProposalPending, ProposalApproved, ProposalPartiallyApproved,
		ProposalRejected, ProposalConflict, ProposalExpired:
		return true
	}
	return false
}

// CanTransitionTo returns true if transitioning from s to target is valid.
func (s ProposalStatus) CanTransitionTo(target ProposalStatus) bool {
	switch s {
	case ProposalPending:
		return target == ProposalApproved ||
			target == ProposalPartiallyApproved ||
			target == ProposalRejected ||
			target == ProposalConflict
	case ProposalConflict:
		return target == ProposalApproved ||
			target == ProposalPartiallyApproved ||
			target == ProposalRejected
	default:
		return false // approved, rejected, partially_approved, expired are terminal
	}
}

// ProposalSource indicates how a proposal was created.
type ProposalSource string

const (
	ProposalSourceAISuggested ProposalSource = "ai_suggested"
	ProposalSourceAIGenerated ProposalSource = "ai_generated"
	ProposalSourceImport      ProposalSource = "import"
)

// Valid returns true if the source is a known value.
func (s ProposalSource) Valid() bool {
	switch s {
	case ProposalSourceAISuggested, ProposalSourceAIGenerated, ProposalSourceImport:
		return true
	}
	return false
}

// DocumentProposal represents a proposed change to an existing document.
type DocumentProposal struct {
	ID              surrealmodels.RecordID `json:"id"`
	Vault           surrealmodels.RecordID `json:"vault"`
	Document        surrealmodels.RecordID `json:"document"`
	ProposedContent string                 `json:"proposed_content"`
	Description     *string                `json:"description,omitempty"`
	Source          ProposalSource         `json:"source"`
	Status          ProposalStatus         `json:"status"`
	OriginalHash    string                 `json:"original_hash"`
	ReviewedAt      *time.Time             `json:"reviewed_at,omitempty"`
	ReviewerNotes   *string                `json:"reviewer_notes,omitempty"`
	CreatedAt       time.Time              `json:"created_at"`
}

// DocumentProposalInput holds the data needed to create a proposal.
type DocumentProposalInput struct {
	VaultID         string         `json:"vault_id"`
	DocumentID      string         `json:"document_id"`
	ProposedContent string         `json:"proposed_content"`
	Description     *string        `json:"description,omitempty"`
	Source          ProposalSource `json:"source"`
	OriginalHash    string         `json:"original_hash"`
}

// Validate checks that all required fields are populated and valid.
func (i DocumentProposalInput) Validate() error {
	if i.VaultID == "" {
		return fmt.Errorf("vault ID is required")
	}
	if i.DocumentID == "" {
		return fmt.Errorf("document ID is required")
	}
	if i.ProposedContent == "" {
		return fmt.Errorf("proposed content is required")
	}
	if !i.Source.Valid() {
		return fmt.Errorf("invalid proposal source: %q", i.Source)
	}
	return nil
}
