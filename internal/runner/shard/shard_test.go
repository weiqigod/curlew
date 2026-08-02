package shard_test

import (
	"fmt"
	"testing"

	"github.com/peterlindqvist/apitest/internal/parser"
	"github.com/peterlindqvist/apitest/internal/runner/shard"
)

func makeReq(name string) parser.RequestItem {
	return parser.RequestItem{Name: name}
}

func makeItems(n int) []parser.RequestItem {
	items := make([]parser.RequestItem, n)
	for i := range items {
		items[i] = parser.RequestItem{Name: fmt.Sprintf("r%d", i+1)}
	}
	return items
}

func TestSplit(t *testing.T) {
	tests := []struct {
		name  string
		items []parser.RequestItem
		n     int
		want  [][]string // per-shard name order
	}{
		{
			"evenly divisible 6/3",
			[]parser.RequestItem{makeReq("a"), makeReq("b"), makeReq("c"), makeReq("d"), makeReq("e"), makeReq("f")},
			3,
			[][]string{{"a", "d"}, {"b", "e"}, {"c", "f"}},
		},
		{
			"remainder — 7 items over 3 shards",
			[]parser.RequestItem{makeReq("a"), makeReq("b"), makeReq("c"), makeReq("d"), makeReq("e"), makeReq("f"), makeReq("g")},
			3,
			[][]string{{"a", "d", "g"}, {"b", "e"}, {"c", "f"}},
		},
		{
			"fewer items than shards",
			[]parser.RequestItem{makeReq("a"), makeReq("b")},
			4,
			[][]string{{"a"}, {"b"}, {}, {}},
		},
		{
			"single shard",
			[]parser.RequestItem{makeReq("a"), makeReq("b"), makeReq("c")},
			1,
			[][]string{{"a", "b", "c"}},
		},
		{
			"empty input",
			nil,
			3,
			[][]string{{}, {}, {}},
		},
		{
			"observable 12/4",
			makeItems(12),
			4,
			[][]string{{"r1", "r5", "r9"}, {"r2", "r6", "r10"}, {"r3", "r7", "r11"}, {"r4", "r8", "r12"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plans := shard.Split(tt.items, tt.n)
			if len(plans) != len(tt.want) {
				t.Fatalf("Split returned %d plans, want %d", len(plans), len(tt.want))
			}
			for i, plan := range plans {
				wantNames := tt.want[i]
				if len(plan.Requests) != len(wantNames) {
					t.Errorf("shard[%d]: got %d requests, want %d", i, len(plan.Requests), len(wantNames))
					continue
				}
				for j, req := range plan.Requests {
					if req.Name != wantNames[j] {
						t.Errorf("shard[%d][%d]: name = %q, want %q", i, j, req.Name, wantNames[j])
					}
				}
			}
		})
	}
}

func TestSplit_OriginalIndexRoundTrip(t *testing.T) {
	items := makeItems(12)
	n := 4
	plans := shard.Split(items, n)

	// Reconstruct original slice from OriginalIndex.
	reconstructed := make([]parser.RequestItem, len(items))
	for shardIdx, plan := range plans {
		for j, req := range plan.Requests {
			origIdx := plan.OriginalIndex[j]
			if origIdx < 0 || origIdx >= len(items) {
				t.Fatalf("shard[%d][%d]: OriginalIndex %d out of range", shardIdx, j, origIdx)
			}
			reconstructed[origIdx] = req
		}
	}

	for i, req := range reconstructed {
		if req.Name != items[i].Name {
			t.Errorf("reconstructed[%d].Name = %q, want %q", i, req.Name, items[i].Name)
		}
	}

	// Verify the formula: items[i] → shard[i%n], position i/n within that shard.
	// So OriginalIndex for plans[s][j] should be j*n + s.
	for s, plan := range plans {
		for j := range plan.Requests {
			want := j*n + s
			if plan.OriginalIndex[j] != want {
				t.Errorf("plans[%d].OriginalIndex[%d] = %d, want %d", s, j, plan.OriginalIndex[j], want)
			}
		}
	}
}

func TestSplit_PanicsOnZero(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for n=0")
		}
	}()
	shard.Split(nil, 0)
}
