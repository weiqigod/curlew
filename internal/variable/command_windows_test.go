package variable

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestWindowsCommand(t *testing.T) {
	for _, test := range []struct {
		name    string
		command string
		want    string
		failure bool
	}{
		{"echo", "Write-Output 'hello'", "hello", false},
		{"unicode", "[Console]::Write([char]0x00e5)", "\u00e5", false},
		{"spaces", "[Console]::Write('  hello  ')", "  hello  ", false},
		{"crlf", "[Console]::Write(\"a`r`nb`r`n`r`n\")", "a\r\nb", false},
		{"standalone_cr", "[Console]::Write(\"value`r\")", "value\r", false},
		{"exit_42", "exit 42", "", true},
		{"missing_program", "curlew_missing_program_78341", "", true},
		{"script_error", "throw 'failure'", "", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			got, err := ExecuteCommand(ctx, test.command)
			if test.failure {
				if !errors.Is(err, ErrCommandFailed) {
					t.Fatalf("error = %v, want ErrCommandFailed", err)
				}
				if test.name == "exit_42" && !strings.Contains(err.Error(), "42") {
					t.Fatalf("missing exit code: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("got %q, want %q", got, test.want)
			}
		})
	}
}

func TestWindowsCommandCancellation(t *testing.T) {
	for _, test := range []struct {
		name     string
		duration time.Duration
		want     error
	}{
		{"cancelled", 0, context.Canceled},
		{"deadline", 2 * time.Second, context.DeadlineExceeded},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), test.duration)
			defer cancel()
			if test.duration == 0 {
				ctx, cancel = context.WithCancel(context.Background())
				cancel()
			}
			_, err := ExecuteCommand(ctx, "Start-Sleep -Seconds 60")
			if !errors.Is(err, test.want) || !errors.Is(err, ErrCommandFailed) {
				t.Fatalf("got %v, want %v and ErrCommandFailed", err, test.want)
			}
		})
	}
}
