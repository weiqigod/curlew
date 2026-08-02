package schedule_test

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/peterlindqvist/apitest/internal/worker/schedule"
)

func TestResolveCollection(t *testing.T) {
	wd := "/home/user/project"

	tests := []struct {
		name       string
		ref        string
		workingDir string
		wantPath   string
		wantErr    error
	}{
		{
			name:       "file: relative path",
			ref:        "file:./testdata/api.yaml",
			workingDir: wd,
			wantPath:   filepath.Join(wd, "testdata/api.yaml"),
		},
		{
			name:       "file: relative path without ./",
			ref:        "file:testdata/api.yaml",
			workingDir: wd,
			wantPath:   filepath.Join(wd, "testdata/api.yaml"),
		},
		{
			name:       "file: absolute path stays absolute",
			ref:        "file:/etc/apitest/api.yaml",
			workingDir: wd,
			wantPath:   "/etc/apitest/api.yaml",
		},
		{
			name:       "git: ref is rejected with ErrUnsupportedCollectionRef",
			ref:        "git:https://github.com/org/repo.git",
			workingDir: wd,
			wantErr:    schedule.ErrUnsupportedCollectionRef,
		},
		{
			name:       "unknown scheme is rejected",
			ref:        "s3://bucket/api.yaml",
			workingDir: wd,
			wantErr:    schedule.ErrUnsupportedCollectionRef,
		},
		{
			name:       "bare path without scheme is rejected",
			ref:        "testdata/api.yaml",
			workingDir: wd,
			wantErr:    schedule.ErrUnsupportedCollectionRef,
		},
		{
			name:       "file: empty path returns error",
			ref:        "file:",
			workingDir: wd,
			wantErr:    schedule.ErrUnsupportedCollectionRef,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := schedule.ResolveCollection(tc.ref, tc.workingDir)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("error = %v; want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.wantPath {
				t.Errorf("path = %q; want %q", got, tc.wantPath)
			}
		})
	}
}
