package report

import (
	"errors"
	"testing"
)

func TestDetectFormat(t *testing.T) {
	tests := []struct {
		name     string
		flag     string
		want     Format
		wantPath string
		wantErr  bool
	}{
		{"empty flag defaults to stdout", "", FormatStdout, "", false},
		{"literal stdout string", "stdout", FormatStdout, "", false},
		{"json extension", "report.json", FormatJSON, "report.json", false},
		{"html extension", "out/perf.html", FormatHTML, "out/perf.html", false},
		{"uppercase extension treated as case-insensitive", "R.JSON", FormatJSON, "R.JSON", false},
		{"unsupported extension xyz", "report.xyz", FormatStdout, "", true},
		{"no extension", "perf-report", FormatStdout, "", true},
		{"hidden file without extension", ".perf", FormatStdout, "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f, p, err := DetectFormat(tc.flag)
			if tc.wantErr {
				if !errors.Is(err, ErrUnsupportedFormat) {
					t.Fatalf("err = %v, want ErrUnsupportedFormat", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if f != tc.want {
				t.Errorf("format = %v, want %v", f, tc.want)
			}
			if p != tc.wantPath {
				t.Errorf("path = %q, want %q", p, tc.wantPath)
			}
		})
	}
}
