package worker

import (
	"errors"
	"strings"
	"testing"
)

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr error
		wantMsg string // substring match when wantErr is nil but error expected
	}{
		{
			name:    "missing url",
			cfg:     Config{Token: "t", JobID: "job_x", Org: "acme", Concurrency: 1},
			wantErr: ErrCoordinatorURLMissing,
		},
		{
			name:    "missing token",
			cfg:     Config{CoordinatorURL: "http://x", JobID: "job_x", Org: "acme", Concurrency: 1},
			wantErr: ErrTokenMissing,
		},
		{
			name:    "missing job",
			cfg:     Config{CoordinatorURL: "http://x", Token: "t", Org: "acme", Concurrency: 1},
			wantMsg: "--job is required",
		},
		{
			name:    "missing org",
			cfg:     Config{CoordinatorURL: "http://x", Token: "t", JobID: "job_x", Concurrency: 1},
			wantMsg: "--org is required",
		},
		{
			name:    "zero concurrency",
			cfg:     Config{CoordinatorURL: "http://x", Token: "t", JobID: "job_x", Org: "acme"},
			wantMsg: ">= 1",
		},
		{
			name: "valid",
			cfg:  Config{CoordinatorURL: "http://x", Token: "t", JobID: "job_x", Org: "acme", Concurrency: 1},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.Validate()
			switch {
			case tc.wantErr != nil:
				if !errors.Is(err, tc.wantErr) {
					t.Errorf("Validate() = %v; want %v", err, tc.wantErr)
				}
			case tc.wantMsg != "":
				if err == nil {
					t.Errorf("Validate() = nil; want error containing %q", tc.wantMsg)
				} else if !strings.Contains(err.Error(), tc.wantMsg) {
					t.Errorf("Validate() = %v; want error containing %q", err, tc.wantMsg)
				}
			default:
				if err != nil {
					t.Errorf("Validate() = %v; want nil", err)
				}
			}
		})
	}
}
