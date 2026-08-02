package discovery

import "testing"

func TestMatchPattern(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		path    string
		want    bool
	}{
		{"star matches single segment", "*.yaml", "a.yaml", true},
		{"star does not cross separator", "*.yaml", "sub/a.yaml", false},
		{"double star matches zero segments", "**/*.yaml", "a.yaml", true},
		{"double star matches one segment", "**/*.yaml", "sub/a.yaml", true},
		{"double star matches multiple segments", "**/*.yaml", "x/y/z.yaml", true},
		{"double star standalone", "**", "a/b/c", true},
		{"double star standalone matches file", "**", "file.yaml", true},
		{"question mark matches one char", "a?.yaml", "ab.yaml", true},
		{"question mark does not match empty", "a?.yaml", "a.yaml", false},
		{"question mark does not cross separator", "a?.yaml", "a/.yaml", false},
		{"char class matches", "a[12].yaml", "a1.yaml", true},
		{"char class miss", "a[12].yaml", "a3.yaml", false},
		{"prefix match", "sub/**", "sub/a/b.yaml", true},
		{"prefix match shallow", "sub/**", "sub/a.yaml", true},
		{"suffix match", "**/c.yaml", "sub/c.yaml", true},
		{"suffix match deep", "**/c.yaml", "a/b/c.yaml", true},
		{"suffix match no sep prefix", "**/c.yaml", "c.yaml", true},
		{"exact literal match", "a.yaml", "a.yaml", true},
		{"exact literal mismatch", "a.yaml", "b.yaml", false},
		{"middle double star", "a/**/b.yaml", "a/x/y/b.yaml", true},
		{"middle double star zero", "a/**/b.yaml", "a/b.yaml", true},
		{"no match empty path", "*.yaml", "", false},
		{"negated char class", "a[^12].yaml", "a3.yaml", true},
		{"negated char class miss", "a[^12].yaml", "a1.yaml", false},
		{"char class with range", "a[a-z].yaml", "ab.yaml", true},
		{"char class with range miss", "a[a-z].yaml", "a1.yaml", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchPattern(tt.pattern, tt.path)
			if got != tt.want {
				t.Errorf("matchPattern(%q, %q) = %v, want %v", tt.pattern, tt.path, got, tt.want)
			}
		})
	}
}
