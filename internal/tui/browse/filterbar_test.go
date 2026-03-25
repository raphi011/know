package browse

import (
	"testing"
)

var testSupported = map[string]bool{
	"status": true,
	"label":  true,
	"from":   true,
}

func TestParseFilterInput(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantQuery   string
		wantFilters map[string][]string
	}{
		{
			name:        "plain text only",
			input:       "fix bug",
			wantQuery:   "fix bug",
			wantFilters: map[string][]string{},
		},
		{
			name:        "single filter",
			input:       "status:open",
			wantQuery:   "",
			wantFilters: map[string][]string{"status": {"open"}},
		},
		{
			name:        "mixed filter and text",
			input:       "fix bug status:open",
			wantQuery:   "fix bug",
			wantFilters: map[string][]string{"status": {"open"}},
		},
		{
			name:        "multi-value filter",
			input:       "label:go label:rust",
			wantQuery:   "",
			wantFilters: map[string][]string{"label": {"go", "rust"}},
		},
		{
			name:        "unsupported key becomes query text",
			input:       "unknown:value fix",
			wantQuery:   "unknown:value fix",
			wantFilters: map[string][]string{},
		},
		{
			name:        "empty input",
			input:       "",
			wantQuery:   "",
			wantFilters: map[string][]string{},
		},
		{
			name:        "colon in value",
			input:       "from:/docs/notes",
			wantQuery:   "",
			wantFilters: map[string][]string{"from": {"/docs/notes"}},
		},
		{
			name:        "key with no value treated as query text",
			input:       "status:",
			wantQuery:   "status:",
			wantFilters: map[string][]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseFilterInput(tt.input, testSupported)

			if result.Query != tt.wantQuery {
				t.Errorf("Query = %q, want %q", result.Query, tt.wantQuery)
			}

			if len(result.Filters) != len(tt.wantFilters) {
				t.Errorf("Filters len = %d, want %d: got %v", len(result.Filters), len(tt.wantFilters), result.Filters)
				return
			}

			for key, wantVals := range tt.wantFilters {
				gotVals := result.Filters[key]
				if len(gotVals) != len(wantVals) {
					t.Errorf("Filters[%q] = %v, want %v", key, gotVals, wantVals)
					continue
				}
				for i, v := range wantVals {
					if gotVals[i] != v {
						t.Errorf("Filters[%q][%d] = %q, want %q", key, i, gotVals[i], v)
					}
				}
			}
		})
	}
}

func TestFilterResult_Filter(t *testing.T) {
	r := FilterResult{
		Query:   "",
		Filters: map[string][]string{"status": {"open", "closed"}},
	}

	if got := r.Filter("status"); got != "open" {
		t.Errorf("Filter(status) = %q, want %q", got, "open")
	}
	if got := r.Filter("missing"); got != "" {
		t.Errorf("Filter(missing) = %q, want %q", got, "")
	}
}

func TestFilterResult_FilterAll(t *testing.T) {
	r := FilterResult{
		Query:   "",
		Filters: map[string][]string{"label": {"go", "rust"}},
	}

	vals := r.FilterAll("label")
	if len(vals) != 2 || vals[0] != "go" || vals[1] != "rust" {
		t.Errorf("FilterAll(label) = %v, want [go rust]", vals)
	}
	if got := r.FilterAll("missing"); len(got) != 0 {
		t.Errorf("FilterAll(missing) = %v, want []", got)
	}
}
