package db

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/raphi011/knowhow/internal/models"
)

func TestCreateProposal(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())
	doc, err := testDB.CreateDocument(ctx, models.DocumentInput{
		VaultID: vaultID, Path: "/proposal-" + suffix + ".md", Title: "Proposal Doc",
		Content: "original content", ContentBody: "original content",
		Source: models.SourceManual, Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateDocument failed: %v", err)
	}
	docID := models.MustRecordIDString(doc.ID)

	desc := "Suggested improvement"
	proposal, err := testDB.CreateProposal(ctx, models.DocumentProposalInput{
		VaultID:         vaultID,
		DocumentID:      docID,
		ProposedContent: "improved content",
		Description:     &desc,
		Source:          models.ProposalSourceAISuggested,
		OriginalHash:    "hash123",
	})
	if err != nil {
		t.Fatalf("CreateProposal failed: %v", err)
	}
	if proposal.ProposedContent != "improved content" {
		t.Errorf("Expected proposed content 'improved content', got %q", proposal.ProposedContent)
	}
	if proposal.Status != models.ProposalPending {
		t.Errorf("Expected status 'pending', got %q", proposal.Status)
	}
	if proposal.Description == nil || *proposal.Description != desc {
		t.Errorf("Expected description %q, got %v", desc, proposal.Description)
	}
	if proposal.OriginalHash != "hash123" {
		t.Errorf("Expected original_hash 'hash123', got %q", proposal.OriginalHash)
	}
}

func TestGetProposal(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())
	doc, err := testDB.CreateDocument(ctx, models.DocumentInput{
		VaultID: vaultID, Path: "/getprop-" + suffix + ".md", Title: "GetProp Doc",
		Content: "content", ContentBody: "content",
		Source: models.SourceManual, Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateDocument failed: %v", err)
	}
	docID := models.MustRecordIDString(doc.ID)

	proposal, err := testDB.CreateProposal(ctx, models.DocumentProposalInput{
		VaultID:         vaultID,
		DocumentID:      docID,
		ProposedContent: "proposed",
		Source:          models.ProposalSourceAISuggested,
		OriginalHash:    "hash456",
	})
	if err != nil {
		t.Fatalf("CreateProposal failed: %v", err)
	}
	proposalID := models.MustRecordIDString(proposal.ID)

	fetched, err := testDB.GetProposal(ctx, proposalID)
	if err != nil {
		t.Fatalf("GetProposal failed: %v", err)
	}
	if fetched == nil {
		t.Fatal("GetProposal returned nil for existing proposal")
	}
	if fetched.ProposedContent != "proposed" {
		t.Errorf("Expected proposed content 'proposed', got %q", fetched.ProposedContent)
	}

	// Nonexistent proposal should return nil
	notFound, err := testDB.GetProposal(ctx, "document_proposal:nonexistent_"+suffix)
	if err != nil {
		t.Fatalf("GetProposal nonexistent error: %v", err)
	}
	if notFound != nil {
		t.Error("Expected nil for nonexistent proposal")
	}
}

func TestListProposals(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())
	doc1, err := testDB.CreateDocument(ctx, models.DocumentInput{
		VaultID: vaultID, Path: "/listprop-1-" + suffix + ".md", Title: "ListProp 1",
		Content: "content", ContentBody: "content",
		Source: models.SourceManual, Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateDocument 1 failed: %v", err)
	}
	doc2, err := testDB.CreateDocument(ctx, models.DocumentInput{
		VaultID: vaultID, Path: "/listprop-2-" + suffix + ".md", Title: "ListProp 2",
		Content: "content", ContentBody: "content",
		Source: models.SourceManual, Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateDocument 2 failed: %v", err)
	}
	doc1ID := models.MustRecordIDString(doc1.ID)
	doc2ID := models.MustRecordIDString(doc2.ID)

	_, err = testDB.CreateProposal(ctx, models.DocumentProposalInput{
		VaultID: vaultID, DocumentID: doc1ID, ProposedContent: "proposal 1",
		Source: models.ProposalSourceAISuggested, OriginalHash: "h1",
	})
	if err != nil {
		t.Fatalf("CreateProposal 1 failed: %v", err)
	}
	_, err = testDB.CreateProposal(ctx, models.DocumentProposalInput{
		VaultID: vaultID, DocumentID: doc2ID, ProposedContent: "proposal 2",
		Source: models.ProposalSourceAISuggested, OriginalHash: "h2",
	})
	if err != nil {
		t.Fatalf("CreateProposal 2 failed: %v", err)
	}

	// List with nil status
	proposals, err := testDB.ListProposals(ctx, vaultID, nil)
	if err != nil {
		t.Fatalf("ListProposals failed: %v", err)
	}
	if len(proposals) < 2 {
		t.Errorf("Expected at least 2 proposals, got %d", len(proposals))
	}
}

func TestListProposalsByDocument(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())
	doc, err := testDB.CreateDocument(ctx, models.DocumentInput{
		VaultID: vaultID, Path: "/listbydoc-" + suffix + ".md", Title: "ListByDoc",
		Content: "content", ContentBody: "content",
		Source: models.SourceManual, Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateDocument failed: %v", err)
	}
	docID := models.MustRecordIDString(doc.ID)

	_, err = testDB.CreateProposal(ctx, models.DocumentProposalInput{
		VaultID: vaultID, DocumentID: docID, ProposedContent: "proposal A",
		Source: models.ProposalSourceAISuggested, OriginalHash: "hA",
	})
	if err != nil {
		t.Fatalf("CreateProposal A failed: %v", err)
	}
	_, err = testDB.CreateProposal(ctx, models.DocumentProposalInput{
		VaultID: vaultID, DocumentID: docID, ProposedContent: "proposal B",
		Source: models.ProposalSourceAISuggested, OriginalHash: "hB",
	})
	if err != nil {
		t.Fatalf("CreateProposal B failed: %v", err)
	}

	proposals, err := testDB.ListProposalsByDocument(ctx, docID, nil)
	if err != nil {
		t.Fatalf("ListProposalsByDocument failed: %v", err)
	}
	if len(proposals) != 2 {
		t.Errorf("Expected 2 proposals for document, got %d", len(proposals))
	}
}

func TestUpdateProposalStatus(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())
	doc, err := testDB.CreateDocument(ctx, models.DocumentInput{
		VaultID: vaultID, Path: "/updatestatus-" + suffix + ".md", Title: "UpdateStatus",
		Content: "content", ContentBody: "content",
		Source: models.SourceManual, Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateDocument failed: %v", err)
	}
	docID := models.MustRecordIDString(doc.ID)

	proposal, err := testDB.CreateProposal(ctx, models.DocumentProposalInput{
		VaultID: vaultID, DocumentID: docID, ProposedContent: "proposed change",
		Source: models.ProposalSourceAISuggested, OriginalHash: "hx",
	})
	if err != nil {
		t.Fatalf("CreateProposal failed: %v", err)
	}
	proposalID := models.MustRecordIDString(proposal.ID)

	notes := "Looks good, approved by reviewer"
	err = testDB.UpdateProposalStatus(ctx, proposalID, models.ProposalApproved, &notes)
	if err != nil {
		t.Fatalf("UpdateProposalStatus failed: %v", err)
	}

	updated, err := testDB.GetProposal(ctx, proposalID)
	if err != nil {
		t.Fatalf("GetProposal after status update failed: %v", err)
	}
	if updated == nil {
		t.Fatal("GetProposal returned nil after status update")
	}
	if updated.Status != models.ProposalApproved {
		t.Errorf("Expected status 'approved', got %q", updated.Status)
	}
	if updated.ReviewerNotes == nil || *updated.ReviewerNotes != notes {
		t.Errorf("Expected reviewer notes %q, got %v", notes, updated.ReviewerNotes)
	}
	if updated.ReviewedAt == nil {
		t.Error("Expected reviewed_at to be set after status update")
	}
}
