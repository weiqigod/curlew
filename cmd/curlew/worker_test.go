package main

import (
	"strings"
	"testing"
	"time"

	"github.com/weiqigod/curlew/internal/worker"
)

// TestWorkerCmd_EventsFlagRejected verifies that --events is not accepted by the
// worker subcommand; it must fail with exit 1 and the canonical rejection message.
func TestWorkerCmd_EventsFlagRejected(t *testing.T) {
	_, _, _, err := parseWorkerArgs([]string{"--events", "/tmp/x.jsonl"})
	if err == nil {
		t.Fatal("parseWorkerArgs --events: want error, got nil")
	}
	const wantMsg = "--events is supported only on run; use --format jsonl for streaming samples"
	if !strings.Contains(err.Error(), wantMsg) {
		t.Errorf("error = %q; want containing %q", err.Error(), wantMsg)
	}
}

func TestParseWorkerArgs(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		args    []string
		want    worker.Config
		wantErr string
		help    bool
	}{
		{
			name: "all_flags",
			args: []string{"--job", "job_x", "--org", "acme", "--coordinator-url", "http://c", "--token", "t", "--concurrency", "4"},
			want: worker.Config{
				JobID:             "job_x",
				Org:               "acme",
				CoordinatorURL:    "http://c",
				Token:             "t",
				Concurrency:       4,
				HeartbeatInterval: 15 * time.Second,
			},
		},
		{
			name: "env_defaults",
			env: map[string]string{
				"CURLEW_COORDINATOR_URL": "http://e",
				"CURLEW_BACKEND_TOKEN":   "et",
			},
			args: []string{"--job", "job_y", "--org", "beta"},
			want: worker.Config{
				JobID:             "job_y",
				Org:               "beta",
				CoordinatorURL:    "http://e",
				Token:             "et",
				Concurrency:       1,
				HeartbeatInterval: 15 * time.Second,
			},
		},
		{
			name: "help_flag",
			args: []string{"--help"},
			help: true,
		},
		{
			name:    "missing_value",
			args:    []string{"--job"},
			wantErr: "--job requires a value",
		},
		{
			name:    "unknown_flag",
			args:    []string{"--bogus"},
			wantErr: "unknown flag",
		},
		{
			name:    "concurrency_invalid",
			args:    []string{"--concurrency", "abc"},
			wantErr: "must be an integer",
		},
		{
			name:    "heartbeat_invalid",
			args:    []string{"--heartbeat-interval", "abc"},
			wantErr: "invalid",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Scope env vars per case
			for k, v := range tc.env {
				t.Setenv(k, v)
			}

			cfg, _, showHelp, err := parseWorkerArgs(tc.args)

			switch {
			case tc.wantErr != "":
				if err == nil {
					t.Errorf("parseWorkerArgs() error = nil; want error containing %q", tc.wantErr)
					return
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("parseWorkerArgs() error = %v; want error containing %q", err, tc.wantErr)
				}
			case tc.help:
				if !showHelp {
					t.Error("parseWorkerArgs() showHelp = false; want true")
				}
			default:
				if err != nil {
					t.Fatalf("parseWorkerArgs() error = %v; want nil", err)
				}
				if showHelp {
					t.Error("parseWorkerArgs() showHelp = true; want false")
				}
				// Compare all fields except WorkerID (auto-generated)
				if cfg.JobID != tc.want.JobID {
					t.Errorf("JobID = %q; want %q", cfg.JobID, tc.want.JobID)
				}
				if cfg.Org != tc.want.Org {
					t.Errorf("Org = %q; want %q", cfg.Org, tc.want.Org)
				}
				if cfg.CoordinatorURL != tc.want.CoordinatorURL {
					t.Errorf("CoordinatorURL = %q; want %q", cfg.CoordinatorURL, tc.want.CoordinatorURL)
				}
				if cfg.Token != tc.want.Token {
					t.Errorf("Token = %q; want %q", cfg.Token, tc.want.Token)
				}
				if cfg.Concurrency != tc.want.Concurrency {
					t.Errorf("Concurrency = %d; want %d", cfg.Concurrency, tc.want.Concurrency)
				}
				if cfg.HeartbeatInterval != tc.want.HeartbeatInterval {
					t.Errorf("HeartbeatInterval = %v; want %v", cfg.HeartbeatInterval, tc.want.HeartbeatInterval)
				}
			}
		})
	}
}

func TestParseWorkerArgs_RefreshVault(t *testing.T) {
	// --refresh-vault must be accepted and set the refreshVault flag.
	_, schedCfg, _, err := parseWorkerArgs([]string{"--schedule-pull", "--refresh-vault"})
	if err != nil {
		t.Fatalf("parseWorkerArgs --refresh-vault: unexpected error: %v", err)
	}
	if !schedCfg.refreshVault {
		t.Error("schedCfg.refreshVault = false, want true after --refresh-vault")
	}
}
