package integration

import (
	"context"
	"strings"
	"testing"

	"github.com/raphi011/knowhow/internal/document"
	"github.com/raphi011/knowhow/internal/models"
	"github.com/raphi011/knowhow/internal/review"
	"github.com/raphi011/knowhow/internal/vault"
)

// TestProposalLifecycle exercises: create proposal, diff, approve-all,
// partial approve, reject, conflict detection, status guards, and cascade delete.
func TestProposalLifecycle(t *testing.T) {
	ctx := context.Background()

	// --- Bootstrap ---

	user, err := testDB.CreateUser(ctx, models.UserInput{Name: "proposal-test-user"})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	userID, err := models.RecordIDString(user.ID)
	if err != nil {
		t.Fatalf("user ID: %v", err)
	}

	vaultSvc := vault.NewService(testDB)
	v, err := vaultSvc.Create(ctx, userID, models.VaultInput{Name: "proposal-test-vault"})
	if err != nil {
		t.Fatalf("create vault: %v", err)
	}
	vaultID, err := models.RecordIDString(v.ID)
	if err != nil {
		t.Fatalf("vault ID: %v", err)
	}

	docSvc := document.NewService(testDB, nil)
	reviewSvc := review.NewService(testDB, docSvc)

	// --- Create a document ---

	doc, err := docSvc.Create(ctx, models.DocumentInput{
		VaultID: vaultID,
		Path:    "/docs/test.md",
		Content: "# Test\n\nOriginal content line 1.\nOriginal content line 2.\nOriginal content line 3.\n",
		Source:  models.SourceManual,
	})
	if err != nil {
		t.Fatalf("create doc: %v", err)
	}
	docID, err := models.RecordIDString(doc.ID)
	if err != nil {
		t.Fatalf("doc ID: %v", err)
	}

	// --- Test: reject no-op proposal ---

	_, err = reviewSvc.Create(ctx, vaultID, docID, doc.Content, nil, models.ProposalSourceAISuggested)
	if err == nil {
		t.Fatal("expected error for no-op proposal, got nil")
	}

	// --- Test: create valid proposal ---

	proposedContent := "# Test\n\nUpdated content line 1.\nOriginal content line 2.\nOriginal content line 3.\n"
	desc := "Updated first content line"
	proposal, err := reviewSvc.Create(ctx, vaultID, docID, proposedContent, &desc, models.ProposalSourceAISuggested)
	if err != nil {
		t.Fatalf("create proposal: %v", err)
	}
	proposalID, err := models.RecordIDString(proposal.ID)
	if err != nil {
		t.Fatalf("proposal ID: %v", err)
	}
	if proposal.Status != models.ProposalPending {
		t.Errorf("proposal status: got %q, want %q", proposal.Status, models.ProposalPending)
	}

	// --- Test: Get proposal ---

	fetched, err := reviewSvc.Get(ctx, proposalID)
	if err != nil {
		t.Fatalf("get proposal: %v", err)
	}
	if fetched == nil {
		t.Fatal("get proposal: got nil")
	}

	// --- Test: List proposals ---

	proposals, err := reviewSvc.List(ctx, vaultID, nil)
	if err != nil {
		t.Fatalf("list proposals: %v", err)
	}
	if len(proposals) != 1 {
		t.Fatalf("list proposals: got %d, want 1", len(proposals))
	}

	// List with status filter
	pendingStatus := models.ProposalPending
	proposals, err = reviewSvc.List(ctx, vaultID, &pendingStatus)
	if err != nil {
		t.Fatalf("list pending: %v", err)
	}
	if len(proposals) != 1 {
		t.Errorf("list pending: got %d, want 1", len(proposals))
	}

	// --- Test: Diff ---

	diffResult, err := reviewSvc.Diff(ctx, fetched)
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	if diffResult.HasConflict {
		t.Error("diff: unexpected conflict")
	}
	if len(diffResult.Hunks) == 0 {
		t.Error("diff: expected hunks")
	}
	if diffResult.Stats.Additions == 0 && diffResult.Stats.Deletions == 0 {
		t.Error("diff: expected non-zero stats")
	}

	// --- Test: ListForDocument ---

	docProposals, err := reviewSvc.ListForDocument(ctx, docID, nil)
	if err != nil {
		t.Fatalf("list by document: %v", err)
	}
	if len(docProposals) != 1 {
		t.Fatalf("list by document: got %d, want 1", len(docProposals))
	}

	// --- Test: ApproveAll ---

	notes := "looks good"
	updated, err := reviewSvc.ApproveAll(ctx, proposalID, &notes)
	if err != nil {
		t.Fatalf("approve all: %v", err)
	}
	if updated.Content != proposedContent {
		t.Errorf("approve all content mismatch:\ngot:  %q\nwant: %q", updated.Content, proposedContent)
	}

	// Verify proposal status is now approved
	approved, err := reviewSvc.Get(ctx, proposalID)
	if err != nil {
		t.Fatalf("get approved proposal: %v", err)
	}
	if approved.Status != models.ProposalApproved {
		t.Errorf("approved proposal status: got %q, want %q", approved.Status, models.ProposalApproved)
	}

	// --- Test: status guard — can't approve again ---

	_, err = reviewSvc.ApproveAll(ctx, proposalID, nil)
	if err == nil {
		t.Fatal("expected error approving already-approved proposal")
	}

	// --- Test: status guard — can't reject approved proposal ---

	err = reviewSvc.Reject(ctx, proposalID, nil)
	if err == nil {
		t.Fatal("expected error rejecting already-approved proposal")
	}

	// --- Test: partial approval (ApproveHunks) ---

	// First, update the doc to have enough content for multiple hunks
	var longContent strings.Builder
	for i := 1; i <= 20; i++ {
		longContent.WriteString("line " + itoa(i) + "\n")
	}
	_, err = docSvc.Update(ctx, vaultID, "/docs/test.md", longContent.String())
	if err != nil {
		t.Fatalf("update doc for partial: %v", err)
	}

	// Create proposal that changes two distant lines
	var modifiedContent strings.Builder
	for i := 1; i <= 20; i++ {
		if i == 2 {
			modifiedContent.WriteString("CHANGED 2\n")
		} else if i == 18 {
			modifiedContent.WriteString("CHANGED 18\n")
		} else {
			modifiedContent.WriteString("line " + itoa(i) + "\n")
		}
	}
	partialProposal, err := reviewSvc.Create(ctx, vaultID, docID, modifiedContent.String(), nil, models.ProposalSourceAIGenerated)
	if err != nil {
		t.Fatalf("create partial proposal: %v", err)
	}
	partialID, err := models.RecordIDString(partialProposal.ID)
	if err != nil {
		t.Fatalf("partial proposal ID: %v", err)
	}

	// Approve only the first hunk
	partialUpdated, err := reviewSvc.ApproveHunks(ctx, partialID, []int{0}, nil)
	if err != nil {
		t.Fatalf("approve hunks: %v", err)
	}

	// Verify: line 2 should be changed, line 18 should be original
	if partialUpdated == nil {
		t.Fatal("approve hunks returned nil doc")
	}

	// Check status
	partialResult, err := reviewSvc.Get(ctx, partialID)
	if err != nil {
		t.Fatalf("get partial proposal: %v", err)
	}
	if partialResult.Status != models.ProposalPartiallyApproved {
		t.Errorf("partial proposal status: got %q, want %q", partialResult.Status, models.ProposalPartiallyApproved)
	}

	// --- Test: reject ---

	rejectProposal, err := reviewSvc.Create(ctx, vaultID, docID, "completely different content\n", nil, models.ProposalSourceImport)
	if err != nil {
		t.Fatalf("create reject proposal: %v", err)
	}
	rejectID, err := models.RecordIDString(rejectProposal.ID)
	if err != nil {
		t.Fatalf("reject proposal ID: %v", err)
	}

	rejectNotes := "not relevant"
	if err := reviewSvc.Reject(ctx, rejectID, &rejectNotes); err != nil {
		t.Fatalf("reject: %v", err)
	}
	rejected, err := reviewSvc.Get(ctx, rejectID)
	if err != nil {
		t.Fatalf("get rejected: %v", err)
	}
	if rejected.Status != models.ProposalRejected {
		t.Errorf("rejected status: got %q, want %q", rejected.Status, models.ProposalRejected)
	}

	// --- Test: conflict detection ---

	conflictProposal, err := reviewSvc.Create(ctx, vaultID, docID, "conflict proposed content\n", nil, models.ProposalSourceAISuggested)
	if err != nil {
		t.Fatalf("create conflict proposal: %v", err)
	}
	conflictID, err := models.RecordIDString(conflictProposal.ID)
	if err != nil {
		t.Fatalf("conflict proposal ID: %v", err)
	}

	// Modify the document directly to create a conflict
	_, err = docSvc.Update(ctx, vaultID, "/docs/test.md", "modified after proposal\n")
	if err != nil {
		t.Fatalf("update doc for conflict: %v", err)
	}

	// Try to approve — should fail with conflict
	_, err = reviewSvc.ApproveAll(ctx, conflictID, nil)
	if err == nil {
		t.Fatal("expected conflict error, got nil")
	}

	// Verify proposal was marked as conflict
	conflicted, err := reviewSvc.Get(ctx, conflictID)
	if err != nil {
		t.Fatalf("get conflicted: %v", err)
	}
	if conflicted.Status != models.ProposalConflict {
		t.Errorf("conflict status: got %q, want %q", conflicted.Status, models.ProposalConflict)
	}

	// --- Test: empty hunkIndexes ---

	emptyProposal, err := reviewSvc.Create(ctx, vaultID, docID, "empty hunk test\n", nil, models.ProposalSourceAISuggested)
	if err != nil {
		t.Fatalf("create empty hunk proposal: %v", err)
	}
	emptyID, err := models.RecordIDString(emptyProposal.ID)
	if err != nil {
		t.Fatalf("empty proposal ID: %v", err)
	}
	_, err = reviewSvc.ApproveHunks(ctx, emptyID, []int{}, nil)
	if err == nil {
		t.Fatal("expected error for empty hunkIndexes")
	}

	// --- Test: cascade delete — proposals removed when document deleted ---

	// Count proposals before delete
	allProposals, err := reviewSvc.ListForDocument(ctx, docID, nil)
	if err != nil {
		t.Fatalf("list before cascade: %v", err)
	}
	if len(allProposals) == 0 {
		t.Fatal("expected proposals before cascade delete")
	}

	if err := docSvc.Delete(ctx, vaultID, "/docs/test.md"); err != nil {
		t.Fatalf("delete doc: %v", err)
	}

	// Proposals should be gone
	afterDelete, err := reviewSvc.ListForDocument(ctx, docID, nil)
	if err != nil {
		t.Fatalf("list after cascade: %v", err)
	}
	if len(afterDelete) != 0 {
		t.Errorf("proposals after cascade delete: got %d, want 0", len(afterDelete))
	}

	// --- Cleanup ---

	if err := vaultSvc.Delete(ctx, vaultID); err != nil {
		t.Fatalf("delete vault: %v", err)
	}
}

func itoa(n int) string {
	if n < 10 {
		return string(rune('0' + n))
	}
	return itoa(n/10) + string(rune('0'+n%10))
}
