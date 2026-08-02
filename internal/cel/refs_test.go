package cel

import (
	"testing"
)

func TestCollectTopLevelRefs(t *testing.T) {
	ev, err := NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator: %v", err)
	}

	tests := []struct {
		name string
		src  string
		want []string
	}{
		{
			name: "simple equality both sides",
			src:  "response.body.total == response.body.items.map(i, i.price).sum()",
			want: []string{"response.body.total", "response.body.items.map(i, i.price).sum()"},
		},
		{
			name: "single field reference",
			src:  "response.status == 200",
			want: []string{"response.status"},
		},
		{
			name: "vars reference",
			src:  "vars.api_key == response.body.token",
			want: []string{"vars.api_key", "response.body.token"},
		},
		{
			name: "boolean and",
			src:  `response.status == 200 && response.body.id > 0`,
			want: []string{"response.status", "response.body.id"},
		},
		{
			name: "no activation references",
			src:  "1 + 1 == 2",
			want: nil,
		},
		{
			name: "env reference",
			src:  `env.REGION == "us-east-1"`,
			want: []string{"env.REGION"},
		},
		{
			name: "previous reference",
			src:  "previous.body.id == response.body.id",
			want: []string{"previous.body.id", "response.body.id"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := CollectTopLevelRefs(tc.src, ev)
			if len(got) != len(tc.want) {
				t.Fatalf("CollectTopLevelRefs(%q) = %v, want %v", tc.src, got, tc.want)
			}
			for i, ref := range tc.want {
				if got[i] != ref {
					t.Errorf("refs[%d] = %q, want %q", i, got[i], ref)
				}
			}
		})
	}
}
