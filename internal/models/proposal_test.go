package models

import "testing"

func TestProposalStatus_CanTransitionTo(t *testing.T) {
	tests := []struct {
		from   ProposalStatus
		to     ProposalStatus
		expect bool
	}{
		// Pending → any active state
		{ProposalPending, ProposalApproved, true},
		{ProposalPending, ProposalPartiallyApproved, true},
		{ProposalPending, ProposalRejected, true},
		{ProposalPending, ProposalConflict, true},
		{ProposalPending, ProposalExpired, false},
		{ProposalPending, ProposalPending, false},

		// Conflict can be re-reviewed
		{ProposalConflict, ProposalApproved, true},
		{ProposalConflict, ProposalPartiallyApproved, true},
		{ProposalConflict, ProposalRejected, true},
		{ProposalConflict, ProposalConflict, false},

		// Terminal states cannot transition
		{ProposalApproved, ProposalRejected, false},
		{ProposalApproved, ProposalPending, false},
		{ProposalRejected, ProposalApproved, false},
		{ProposalPartiallyApproved, ProposalApproved, false},
		{ProposalExpired, ProposalPending, false},
	}
	for _, tt := range tests {
		got := tt.from.CanTransitionTo(tt.to)
		if got != tt.expect {
			t.Errorf("%s → %s: got %v, want %v", tt.from, tt.to, got, tt.expect)
		}
	}
}

func TestProposalStatus_Valid(t *testing.T) {
	valid := []ProposalStatus{
		ProposalPending, ProposalApproved, ProposalPartiallyApproved,
		ProposalRejected, ProposalConflict, ProposalExpired,
	}
	for _, s := range valid {
		if !s.Valid() {
			t.Errorf("expected %q to be valid", s)
		}
	}
	if ProposalStatus("invalid").Valid() {
		t.Error("expected 'invalid' to not be valid")
	}
}

func TestProposalSource_Valid(t *testing.T) {
	valid := []ProposalSource{
		ProposalSourceAISuggested, ProposalSourceAIGenerated, ProposalSourceImport,
	}
	for _, s := range valid {
		if !s.Valid() {
			t.Errorf("expected %q to be valid", s)
		}
	}
	if ProposalSource("invalid").Valid() {
		t.Error("expected 'invalid' to not be valid")
	}
}

func TestDocumentProposalInput_Validate(t *testing.T) {
	valid := DocumentProposalInput{
		VaultID:         "test-vault",
		DocumentID:      "test-doc",
		ProposedContent: "content",
		Source:          ProposalSourceAISuggested,
	}
	if err := valid.Validate(); err != nil {
		t.Errorf("expected valid input, got error: %v", err)
	}

	tests := []struct {
		name  string
		input DocumentProposalInput
	}{
		{"empty vault", DocumentProposalInput{DocumentID: "d", ProposedContent: "c", Source: ProposalSourceAISuggested}},
		{"empty document", DocumentProposalInput{VaultID: "v", ProposedContent: "c", Source: ProposalSourceAISuggested}},
		{"empty content", DocumentProposalInput{VaultID: "v", DocumentID: "d", Source: ProposalSourceAISuggested}},
		{"invalid source", DocumentProposalInput{VaultID: "v", DocumentID: "d", ProposedContent: "c", Source: "bad"}},
	}
	for _, tt := range tests {
		if err := tt.input.Validate(); err == nil {
			t.Errorf("%s: expected error, got nil", tt.name)
		}
	}
}
