package httputil

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWriteProblem(t *testing.T) {
	tests := []struct {
		name   string
		status int
		detail string
	}{
		{"bad request", http.StatusBadRequest, "missing field"},
		{"not found", http.StatusNotFound, "document not found"},
		{"internal error", http.StatusInternalServerError, "unexpected failure"},
		{"unauthorized", http.StatusUnauthorized, "invalid token"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			WriteProblem(rec, tt.status, tt.detail)

			if rec.Code != tt.status {
				t.Errorf("status = %d, want %d", rec.Code, tt.status)
			}
			if ct := rec.Header().Get("Content-Type"); ct != ProblemContentType {
				t.Errorf("Content-Type = %q, want %q", ct, ProblemContentType)
			}

			var pd ProblemDetail
			if err := json.NewDecoder(rec.Body).Decode(&pd); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			if pd.Type != "about:blank" {
				t.Errorf("type = %q, want %q", pd.Type, "about:blank")
			}
			if pd.Title != http.StatusText(tt.status) {
				t.Errorf("title = %q, want %q", pd.Title, http.StatusText(tt.status))
			}
			if pd.Status != tt.status {
				t.Errorf("status in body = %d, want %d", pd.Status, tt.status)
			}
			if pd.Detail != tt.detail {
				t.Errorf("detail = %q, want %q", pd.Detail, tt.detail)
			}
		})
	}
}

func TestNewListResponse_NilSlice(t *testing.T) {
	resp := NewListResponse[string](nil, 0)

	if resp.Items == nil {
		t.Fatal("Items is nil, want non-nil empty slice")
	}
	if len(resp.Items) != 0 {
		t.Errorf("len(Items) = %d, want 0", len(resp.Items))
	}
	if resp.Total != 0 {
		t.Errorf("Total = %d, want 0", resp.Total)
	}

	// Verify JSON serializes as [] not null
	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatalf("unmarshal raw: %v", err)
	}
	if string(raw["items"]) != "[]" {
		t.Errorf("items JSON = %s, want []", raw["items"])
	}
}

func TestNewListResponse_WithItems(t *testing.T) {
	items := []int{1, 2, 3}
	resp := NewListResponse(items, 42)

	if len(resp.Items) != 3 {
		t.Errorf("len(Items) = %d, want 3", len(resp.Items))
	}
	if resp.Total != 42 {
		t.Errorf("Total = %d, want 42", resp.Total)
	}
}
