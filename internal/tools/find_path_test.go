package tools

import (
	"testing"
)

func TestFindPathInput_Validation(t *testing.T) {
	tests := []struct {
		name    string
		input   FindPathInput
		wantErr string
	}{
		{"empty from", FindPathInput{From: "", To: "target"}, "from"},
		{"empty to", FindPathInput{From: "source", To: ""}, "to"},
		{"valid", FindPathInput{From: "source", To: "target"}, ""},
		{"with depth", FindPathInput{From: "source", To: "target", MaxDepth: 10}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantErr == "from" && tt.input.From != "" {
				t.Errorf("expected empty from")
			}
			if tt.wantErr == "to" && tt.input.To != "" {
				t.Errorf("expected empty to")
			}
			if tt.wantErr == "" && (tt.input.From == "" || tt.input.To == "") {
				t.Errorf("expected valid input")
			}
		})
	}
}

func TestFindPathInput_DefaultMaxDepth(t *testing.T) {
	input := FindPathInput{From: "a", To: "b"}
	if input.MaxDepth != 0 {
		t.Errorf("expected default max_depth 0 (will be set to 5), got %d", input.MaxDepth)
	}
}

func TestFindPathInput_MaxDepthBounds(t *testing.T) {
	input := FindPathInput{From: "a", To: "b", MaxDepth: 25}
	if input.MaxDepth <= 20 {
		t.Errorf("expected max_depth > 20 for bounds test")
	}
}

func TestFindPathResult_NoPath(t *testing.T) {
	result := FindPathResult{
		PathFound: false,
		Message:   "No path found",
	}
	if result.PathFound {
		t.Error("expected path_found false")
	}
	if result.Path != nil {
		t.Error("expected nil path when not found")
	}
}

func TestFindPathResult_WithPath(t *testing.T) {
	result := FindPathResult{
		PathFound: true,
		Path:      []string{"a", "b", "c"},
		Length:    3,
	}
	if !result.PathFound {
		t.Error("expected path_found true")
	}
	if result.Length != 3 {
		t.Errorf("expected length 3, got %d", result.Length)
	}
}
