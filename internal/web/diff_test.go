package web

import (
	"testing"
)

func TestComputeDiff(t *testing.T) {
	tests := []struct {
		name       string
		old        string
		new        string
		wantAdds   int
		wantRemove int
	}{
		{
			name:       "identical",
			old:        "hello\nworld",
			new:        "hello\nworld",
			wantAdds:   0,
			wantRemove: 0,
		},
		{
			name:       "added line",
			old:        "hello",
			new:        "hello\nworld",
			wantAdds:   1,
			wantRemove: 0,
		},
		{
			name:       "removed line",
			old:        "hello\nworld",
			new:        "hello",
			wantAdds:   0,
			wantRemove: 1,
		},
		{
			name:       "changed line",
			old:        "hello\nworld",
			new:        "hello\nearth",
			wantAdds:   1,
			wantRemove: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines := computeDiff(tt.old, tt.new)

			adds, removes := 0, 0
			for _, l := range lines {
				switch l.Type {
				case "add":
					adds++
				case "remove":
					removes++
				}
			}

			if adds != tt.wantAdds {
				t.Errorf("adds = %d, want %d", adds, tt.wantAdds)
			}
			if removes != tt.wantRemove {
				t.Errorf("removes = %d, want %d", removes, tt.wantRemove)
			}
		})
	}
}
