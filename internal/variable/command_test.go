package variable

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestExecuteCommand(t *testing.T) {
	tests := []struct {
		name    string
		command string
		wantOut string
		wantErr error
	}{
		{"simple_echo_returns_stdout", "echo hello", "hello", nil},
		{"trailing_newline_stripped", "printf 'hello\n'", "hello", nil},
		{"pipe_syntax_works", "echo hello | tr a-z A-Z", "HELLO", nil},
		{"non_zero_exit_returns_error", "exit 1", "", ErrCommandFailed},
		{"error_includes_command_and_exit_code", "exit 42", "", ErrCommandFailed},
		{"error_includes_stderr", "echo err >&2; exit 1", "", ErrCommandFailed},
		{"empty_stdout_returns_empty_string", "true", "", nil},
		{"multiline_stdout_preserved", "printf 'a\nb'", "a\nb", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if runtime.GOOS == "windows" {
				switch tt.name {
				case "trailing_newline_stripped":
					tt.command = "Write-Output 'hello'"
				case "pipe_syntax_works":
					tt.command = "'hello' | ForEach-Object { $_.ToUpperInvariant() }"
				case "empty_stdout_returns_empty_string":
					tt.command = "$null"
				case "multiline_stdout_preserved":
					tt.command = "[Console]::Write(\"a`nb\")"
				}
			}
			got, err := ExecuteCommand(context.Background(), tt.command)
			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.wantOut {
				t.Errorf("output = %q, want %q", got, tt.wantOut)
			}
		})
	}
}

func TestExecuteCommand_error_includes_stderr(t *testing.T) {
	_, err := ExecuteCommand(context.Background(), "echo err >&2; exit 1")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "err") {
		t.Errorf("error = %q, want to contain stderr output 'err'", err.Error())
	}
}

func TestExecuteCommand_error_includes_command_and_exit_code(t *testing.T) {
	_, err := ExecuteCommand(context.Background(), "exit 42")
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "exit 42") {
		t.Errorf("error = %q, want to contain the command 'exit 42'", msg)
	}
}

func TestExecuteCommand_context_cancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := ExecuteCommand(ctx, "sleep 10")
	if err == nil {
		t.Fatal("expected error on cancelled context")
	}
}

func TestCommandCache(t *testing.T) {
	t.Run("cache_hit_within_ttl", func(t *testing.T) {
		c := NewCommandCache()
		c.Set("key", "val", 300)
		got, ok := c.Get("key")
		if !ok {
			t.Fatal("expected cache hit")
		}
		if got != "val" {
			t.Errorf("got %q, want %q", got, "val")
		}
	})

	t.Run("cache_miss_empty", func(t *testing.T) {
		c := NewCommandCache()
		_, ok := c.Get("nonexistent")
		if ok {
			t.Fatal("expected cache miss")
		}
	})

	t.Run("cache_expired_after_ttl", func(t *testing.T) {
		c := NewCommandCache()
		c.mu.Lock()
		c.entries["key"] = cacheEntry{
			value:   "val",
			expires: time.Now().Add(-1 * time.Second),
		}
		c.mu.Unlock()
		_, ok := c.Get("key")
		if ok {
			t.Fatal("expected cache miss for expired entry")
		}
	})

	t.Run("cache_zero_ttl_no_caching", func(t *testing.T) {
		c := NewCommandCache()
		c.Set("key", "val", 0)
		_, ok := c.Get("key")
		if ok {
			t.Fatal("expected cache miss for zero TTL")
		}
	})

	t.Run("cache_negative_ttl_no_caching", func(t *testing.T) {
		c := NewCommandCache()
		c.Set("key", "val", -5)
		_, ok := c.Get("key")
		if ok {
			t.Fatal("expected cache miss for negative TTL")
		}
	})
}
