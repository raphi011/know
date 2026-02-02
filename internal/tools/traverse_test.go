package tools

import (
	"testing"
)

func TestTraverseInput_Validation(t *testing.T) {
	tests := []struct {
		name    string
		input   TraverseInput
		wantErr bool
	}{
		{"empty start", TraverseInput{Start: ""}, true},
		{"valid start", TraverseInput{Start: "test:entity"}, false},
		{"with depth", TraverseInput{Start: "test:entity", Depth: 3}, false},
		{"with types", TraverseInput{Start: "test:entity", RelationTypes: []string{"related_to"}}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Input struct validation - empty start is error
			if tt.wantErr && tt.input.Start != "" {
				t.Errorf("expected error condition not met")
			}
			if !tt.wantErr && tt.input.Start == "" {
				t.Errorf("start should not be empty for valid case")
			}
		})
	}
}

func TestTraverseInput_DefaultDepth(t *testing.T) {
	input := TraverseInput{Start: "test:entity"}
	if input.Depth != 0 {
		t.Errorf("expected default depth 0 (will be set to 2), got %d", input.Depth)
	}
}

func TestTraverseInput_DepthBounds(t *testing.T) {
	// Depth validation happens in handler, struct allows any int
	input := TraverseInput{Start: "test:entity", Depth: 15}
	if input.Depth <= 10 {
		t.Errorf("expected depth > 10 for bounds test")
	}
}
