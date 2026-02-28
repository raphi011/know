package review

import (
	"context"
	"fmt"

	"github.com/raphaelgruber/memcp-go/internal/db"
	"github.com/raphaelgruber/memcp-go/internal/document"
	"github.com/raphaelgruber/memcp-go/internal/models"
)

const defaultContextLines = 3

// DiffResult contains the computed diff for a proposal.
type DiffResult struct {
	Hunks       []Hunk
	HasConflict bool
	Stats       DiffStats
}

// Service manages document proposal lifecycle.
type Service struct {
	db         *db.Client
	docService *document.Service
}

// NewService creates a new review service.
func NewService(db *db.Client, docService *document.Service) *Service {
	return &Service{db: db, docService: docService}
}

// Create stores a new document proposal, capturing the document's current content hash.
func (s *Service) Create(ctx context.Context, vaultID, documentID, proposedContent string, description *string, source models.ProposalSource) (*models.DocumentProposal, error) {
	doc, err := s.db.GetDocumentByID(ctx, documentID)
	if err != nil {
		return nil, fmt.Errorf("get document: %w", err)
	}
	if doc == nil {
		return nil, fmt.Errorf("document not found: %s", documentID)
	}

	originalHash := ""
	if doc.ContentHash != nil {
		originalHash = *doc.ContentHash
	}

	return s.db.CreateProposal(ctx, models.DocumentProposalInput{
		VaultID:         vaultID,
		DocumentID:      documentID,
		ProposedContent: proposedContent,
		Description:     description,
		Source:          source,
		OriginalHash:    originalHash,
	})
}

// CreateByPath stores a new proposal by looking up the document by vault+path.
func (s *Service) CreateByPath(ctx context.Context, vaultID, path, proposedContent string, description *string, source models.ProposalSource) (*models.DocumentProposal, error) {
	doc, err := s.db.GetDocumentByPath(ctx, vaultID, path)
	if err != nil {
		return nil, fmt.Errorf("get document by path: %w", err)
	}
	if doc == nil {
		return nil, fmt.Errorf("document not found at path: %s", path)
	}

	docID, err := models.RecordIDString(doc.ID)
	if err != nil {
		return nil, fmt.Errorf("extract document ID: %w", err)
	}

	return s.Create(ctx, vaultID, docID, proposedContent, description, source)
}

// Get retrieves a single proposal by ID.
func (s *Service) Get(ctx context.Context, id string) (*models.DocumentProposal, error) {
	return s.db.GetProposal(ctx, id)
}

// List returns proposals for a vault, optionally filtered by status.
func (s *Service) List(ctx context.Context, vaultID string, status *models.ProposalStatus) ([]models.DocumentProposal, error) {
	var statusStr *string
	if status != nil {
		s := string(*status)
		statusStr = &s
	}
	return s.db.ListProposals(ctx, vaultID, statusStr)
}

// ListForDocument returns proposals for a specific document, optionally filtered by status.
func (s *Service) ListForDocument(ctx context.Context, documentID string, status *models.ProposalStatus) ([]models.DocumentProposal, error) {
	var statusStr *string
	if status != nil {
		s := string(*status)
		statusStr = &s
	}
	return s.db.ListProposalsByDocument(ctx, documentID, statusStr)
}

// Diff computes the diff between the current document and the proposal.
func (s *Service) Diff(ctx context.Context, proposal *models.DocumentProposal) (*DiffResult, error) {
	docID, err := models.RecordIDString(proposal.Document)
	if err != nil {
		return nil, fmt.Errorf("extract document ID: %w", err)
	}

	doc, err := s.db.GetDocumentByID(ctx, docID)
	if err != nil {
		return nil, fmt.Errorf("get document: %w", err)
	}
	if doc == nil {
		return nil, fmt.Errorf("document not found: %s", docID)
	}

	// Check for conflict
	currentHash := ""
	if doc.ContentHash != nil {
		currentHash = *doc.ContentHash
	}
	hasConflict := proposal.OriginalHash != "" && currentHash != "" && proposal.OriginalHash != currentHash

	hunks, err := ComputeHunks(doc.Content, proposal.ProposedContent, defaultContextLines)
	if err != nil {
		return nil, fmt.Errorf("compute hunks: %w", err)
	}

	return &DiffResult{
		Hunks:       hunks,
		HasConflict: hasConflict,
		Stats:       ComputeStats(hunks),
	}, nil
}

// ApproveAll approves the entire proposal and applies it to the document.
func (s *Service) ApproveAll(ctx context.Context, proposalID string, notes *string) (*models.Document, error) {
	proposal, err := s.db.GetProposal(ctx, proposalID)
	if err != nil {
		return nil, fmt.Errorf("get proposal: %w", err)
	}
	if proposal == nil {
		return nil, fmt.Errorf("proposal not found: %s", proposalID)
	}

	vaultID, err := models.RecordIDString(proposal.Vault)
	if err != nil {
		return nil, fmt.Errorf("extract vault ID: %w", err)
	}

	docID, err := models.RecordIDString(proposal.Document)
	if err != nil {
		return nil, fmt.Errorf("extract document ID: %w", err)
	}

	doc, err := s.db.GetDocumentByID(ctx, docID)
	if err != nil {
		return nil, fmt.Errorf("get document: %w", err)
	}
	if doc == nil {
		return nil, fmt.Errorf("document not found: %s", docID)
	}

	// Apply via document service (re-parses, re-chunks, re-embeds)
	updated, err := s.docService.Update(ctx, vaultID, doc.Path, proposal.ProposedContent)
	if err != nil {
		return nil, fmt.Errorf("apply proposal: %w", err)
	}

	if err := s.db.UpdateProposalStatus(ctx, proposalID, string(models.ProposalApproved), notes); err != nil {
		return nil, fmt.Errorf("update proposal status: %w", err)
	}

	return updated, nil
}

// ApproveHunks approves specific hunks, merges them, and updates the document.
func (s *Service) ApproveHunks(ctx context.Context, proposalID string, hunkIndexes []int, notes *string) (*models.Document, error) {
	proposal, err := s.db.GetProposal(ctx, proposalID)
	if err != nil {
		return nil, fmt.Errorf("get proposal: %w", err)
	}
	if proposal == nil {
		return nil, fmt.Errorf("proposal not found: %s", proposalID)
	}

	vaultID, err := models.RecordIDString(proposal.Vault)
	if err != nil {
		return nil, fmt.Errorf("extract vault ID: %w", err)
	}

	docID, err := models.RecordIDString(proposal.Document)
	if err != nil {
		return nil, fmt.Errorf("extract document ID: %w", err)
	}

	doc, err := s.db.GetDocumentByID(ctx, docID)
	if err != nil {
		return nil, fmt.Errorf("get document: %w", err)
	}
	if doc == nil {
		return nil, fmt.Errorf("document not found: %s", docID)
	}

	// Compute hunks against current document content
	hunks, err := ComputeHunks(doc.Content, proposal.ProposedContent, defaultContextLines)
	if err != nil {
		return nil, fmt.Errorf("compute hunks: %w", err)
	}

	// Apply selected hunks
	merged, err := ApplyHunks(doc.Content, hunks, hunkIndexes)
	if err != nil {
		return nil, fmt.Errorf("apply hunks: %w", err)
	}

	// Apply via document service
	updated, err := s.docService.Update(ctx, vaultID, doc.Path, merged)
	if err != nil {
		return nil, fmt.Errorf("apply merged content: %w", err)
	}

	if err := s.db.UpdateProposalStatus(ctx, proposalID, string(models.ProposalPartiallyApproved), notes); err != nil {
		return nil, fmt.Errorf("update proposal status: %w", err)
	}

	return updated, nil
}

// Reject marks a proposal as rejected.
func (s *Service) Reject(ctx context.Context, proposalID string, notes *string) error {
	proposal, err := s.db.GetProposal(ctx, proposalID)
	if err != nil {
		return fmt.Errorf("get proposal: %w", err)
	}
	if proposal == nil {
		return fmt.Errorf("proposal not found: %s", proposalID)
	}

	return s.db.UpdateProposalStatus(ctx, proposalID, string(models.ProposalRejected), notes)
}
