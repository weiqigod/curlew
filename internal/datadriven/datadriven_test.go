package datadriven

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		file    string // relative path within temp dir
		content string
		wantErr error
	}{
		{"auto-detect CSV from extension", Config{Source: "data.csv"}, "data.csv", "a\n1", nil},
		{"auto-detect JSON from extension", Config{Source: "data.json"}, "data.json", `[{"a":"1"}]`, nil},
		{"auto-detect YAML from .yaml extension", Config{Source: "data.yaml"}, "data.yaml", "- a: '1'", nil},
		{"auto-detect YAML from .yml extension", Config{Source: "data.yml"}, "data.yml", "- a: '1'", nil},
		{"explicit yaml format overrides extension", Config{Source: "data.txt", Format: "yaml"}, "data.txt", "- a: '1'", nil},
		{"explicit format overrides extension", Config{Source: "data.txt", Format: "csv"}, "data.txt", "a\n1", nil},
		{"file not found", Config{Source: "missing.csv"}, "", "", ErrFileNotFound},
		{"unsupported format", Config{Source: "data.xml"}, "data.xml", "<x/>", ErrUnsupportedFormat},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if tt.file != "" {
				if err := os.WriteFile(filepath.Join(dir, tt.file), []byte(tt.content), 0o644); err != nil {
					t.Fatalf("write test file: %v", err)
				}
			}

			ds, err := Load(tt.cfg, dir)
			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("expected error %v, got nil", tt.wantErr)
				}
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ds == nil {
				t.Fatal("expected non-nil DataSet")
			}
			if len(ds.Rows) == 0 {
				t.Error("expected at least one row")
			}
		})
	}
}

func TestLoadWithControls_FilterAndLimit(t *testing.T) {
	dir := t.TempDir()
	content := "name,age\nalice,25\nbob,15\ncarol,30\ndave,12\neve,18"
	if err := os.WriteFile(filepath.Join(dir, "data.csv"), []byte(content), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	limit := 2
	ds, err := LoadWithControls(Config{
		Source: "data.csv",
		Filter: "{{age}} >= 18",
		Limit:  &limit,
	}, dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ds.Rows) != 2 {
		t.Errorf("rows = %d, want 2 (filter then limit)", len(ds.Rows))
	}
}

func TestLoadWithControls_Range(t *testing.T) {
	dir := t.TempDir()
	content := "- idx: '0'\n- idx: '1'\n- idx: '2'\n- idx: '3'\n- idx: '4'\n- idx: '5'\n- idx: '6'\n- idx: '7'\n- idx: '8'\n- idx: '9'"
	if err := os.WriteFile(filepath.Join(dir, "data.yaml"), []byte(content), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	start, end := 2, 5
	ds, err := LoadWithControls(Config{
		Source:   "data.yaml",
		StartRow: &start,
		EndRow:   &end,
	}, dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ds.Rows) != 4 {
		t.Errorf("rows = %d, want 4 (rows 2-5 inclusive)", len(ds.Rows))
	}
}

func TestLoadWithControls_NoControls(t *testing.T) {
	dir := t.TempDir()
	content := "a\n1\n2\n3"
	if err := os.WriteFile(filepath.Join(dir, "data.csv"), []byte(content), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	ds, err := LoadWithControls(Config{Source: "data.csv"}, dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ds.Rows) != 3 {
		t.Errorf("rows = %d, want 3", len(ds.Rows))
	}
}

func TestConfig_EffectiveStoreResults(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want string
	}{
		{"default is all", Config{}, "all"},
		{"explicit all", Config{StoreResults: "all"}, "all"},
		{"summary", Config{StoreResults: "summary"}, "summary"},
		{"failed_only", Config{StoreResults: "failed_only"}, "failed_only"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.EffectiveStoreResults(); got != tt.want {
				t.Errorf("EffectiveStoreResults() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestConfig_ParallelFields(t *testing.T) {
	// Verify that the new parallel/rate_limit/store_results fields exist and have sensible zero values
	cfg := Config{}
	if cfg.Parallel {
		t.Error("Parallel default should be false")
	}
	if cfg.RateLimitRPS != nil {
		t.Error("RateLimitRPS default should be nil")
	}
	if cfg.StoreResults != "" {
		t.Error("StoreResults default should be empty string")
	}
}

func TestConstants(t *testing.T) {
	if DefaultMaxWorkers != 20 {
		t.Errorf("DefaultMaxWorkers = %d, want 20", DefaultMaxWorkers)
	}
	if DefaultChunkSize != 1000 {
		t.Errorf("DefaultChunkSize = %d, want 1000", DefaultChunkSize)
	}
	if LargeDatasetThreshold != 10000 {
		t.Errorf("LargeDatasetThreshold = %d, want 10000", LargeDatasetThreshold)
	}
	if StoreAll != "all" {
		t.Errorf("StoreAll = %q, want %q", StoreAll, "all")
	}
	if StoreSummary != "summary" {
		t.Errorf("StoreSummary = %q, want %q", StoreSummary, "summary")
	}
	if StoreFailedOnly != "failed_only" {
		t.Errorf("StoreFailedOnly = %q, want %q", StoreFailedOnly, "failed_only")
	}
}

func TestLoad_ResolvesRelativePath(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "subdir")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "test.csv"), []byte("x\n1"), 0o644); err != nil {
		t.Fatal(err)
	}
	ds, err := Load(Config{Source: "subdir/test.csv"}, dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ds.Rows) != 1 {
		t.Errorf("got %d rows, want 1", len(ds.Rows))
	}
}
