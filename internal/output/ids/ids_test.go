package ids_test

import (
	"regexp"
	"testing"

	"github.com/peterlindqvist/apitest/internal/output/ids"
)

var runIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

func TestNewRunID(t *testing.T) {
	tests := []struct {
		name  string
		check func(t *testing.T, id string)
	}{
		{
			name: "matches 32-char lowercase hex pattern",
			check: func(t *testing.T, id string) {
				t.Helper()
				if !runIDPattern.MatchString(id) {
					t.Errorf("id %q does not match %s", id, runIDPattern)
				}
			},
		},
		{
			name: "length is exactly 32",
			check: func(t *testing.T, id string) {
				t.Helper()
				if len(id) != 32 {
					t.Errorf("len(id) = %d, want 32", len(id))
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.check(t, ids.NewRunID())
		})
	}
}

func TestNewRunID_DistinctAcrossCalls(t *testing.T) {
	const n = 256
	seen := make(map[string]struct{}, n)
	for i := 0; i < n; i++ {
		id := ids.NewRunID()
		if _, dup := seen[id]; dup {
			t.Fatalf("duplicate id within %d calls: %q", n, id)
		}
		seen[id] = struct{}{}
	}
}
