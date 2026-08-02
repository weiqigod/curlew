package vault

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// mockExecutor returns a CommandExecutor that matches commands and returns preset results.
func mockExecutor(results map[string]struct {
	output string
	err    error
},
) CommandExecutor {
	return func(ctx context.Context, command string) (string, error) {
		for pattern, r := range results {
			if strings.Contains(command, pattern) {
				return r.output, r.err
			}
		}
		return "", fmt.Errorf("unexpected command: %s", command)
	}
}

func TestAWSProvider(t *testing.T) {
	t.Run("fetch_single_secret_success", func(t *testing.T) {
		exec := mockExecutor(map[string]struct {
			output string
			err    error
		}{
			"get-secret-value": {"my-secret-value", nil},
		})
		p := NewAWSProvider("us-east-1", exec)
		got, err := p.Fetch(context.Background(), "prod/api-key")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "my-secret-value" {
			t.Errorf("got %q, want %q", got, "my-secret-value")
		}
	})

	t.Run("fetch_returns_raw_value_untrimmed_internally", func(t *testing.T) {
		exec := mockExecutor(map[string]struct {
			output string
			err    error
		}{
			"get-secret-value": {"  spaces  ", nil},
		})
		p := NewAWSProvider("us-east-1", exec)
		got, err := p.Fetch(context.Background(), "prod/key")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// CommandExecutor (variable.ExecuteCommand) already trims trailing newlines.
		// The provider returns what the executor gives it.
		if got != "  spaces  " {
			t.Errorf("got %q, want %q", got, "  spaces  ")
		}
	})

	t.Run("fetch_nonexistent_secret_returns_not_found", func(t *testing.T) {
		exec := mockExecutor(map[string]struct {
			output string
			err    error
		}{
			"get-secret-value": {"", fmt.Errorf("ResourceNotFoundException: Secrets Manager can't find the specified secret")},
		})
		p := NewAWSProvider("us-east-1", exec)
		_, err := p.Fetch(context.Background(), "nonexistent")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, ErrSecretNotFound) {
			t.Fatalf("expected ErrSecretNotFound, got %v", err)
		}
	})

	t.Run("fetch_invalid_credentials_returns_auth_error", func(t *testing.T) {
		exec := mockExecutor(map[string]struct {
			output string
			err    error
		}{
			"get-secret-value": {"", fmt.Errorf("InvalidClientTokenId: The security token included in the request is invalid")},
		})
		p := NewAWSProvider("us-east-1", exec)
		_, err := p.Fetch(context.Background(), "prod/key")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, ErrProviderAuth) {
			t.Fatalf("expected ErrProviderAuth, got %v", err)
		}
	})

	t.Run("fetch_expired_credentials_returns_auth_error", func(t *testing.T) {
		exec := mockExecutor(map[string]struct {
			output string
			err    error
		}{
			"get-secret-value": {"", fmt.Errorf("ExpiredToken: The security token has expired")},
		})
		p := NewAWSProvider("us-east-1", exec)
		_, err := p.Fetch(context.Background(), "prod/key")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, ErrProviderAuth) {
			t.Fatalf("expected ErrProviderAuth, got %v", err)
		}
	})

	t.Run("bulk_fetch_calls_fetch_per_path", func(t *testing.T) {
		callCount := 0
		exec := func(ctx context.Context, command string) (string, error) {
			callCount++
			if strings.Contains(command, "secret-a") {
				return "value-a", nil
			}
			if strings.Contains(command, "secret-b") {
				return "value-b", nil
			}
			return "", fmt.Errorf("unexpected: %s", command)
		}
		p := NewAWSProvider("us-east-1", exec)
		got, err := p.BulkFetch(context.Background(), []string{"secret-a", "secret-b"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if callCount != 2 {
			t.Errorf("expected 2 calls, got %d", callCount)
		}
		if got["secret-a"] != "value-a" || got["secret-b"] != "value-b" {
			t.Errorf("unexpected results: %v", got)
		}
	})

	t.Run("bulk_fetch_returns_all_results", func(t *testing.T) {
		exec := func(ctx context.Context, command string) (string, error) {
			return "val", nil
		}
		p := NewAWSProvider("us-east-1", exec)
		got, err := p.BulkFetch(context.Background(), []string{"a", "b", "c"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 3 {
			t.Errorf("expected 3 results, got %d", len(got))
		}
	})

	t.Run("name_returns_aws_secrets_manager", func(t *testing.T) {
		p := NewAWSProvider("us-east-1", nil)
		if got := p.Name(); got != "aws-secrets-manager" {
			t.Errorf("got %q, want %q", got, "aws-secrets-manager")
		}
	})

	t.Run("validate_config_success", func(t *testing.T) {
		exec := mockExecutor(map[string]struct {
			output string
			err    error
		}{
			"get-caller-identity": {`{"Account":"123456"}`, nil},
		})
		p := NewAWSProvider("us-east-1", exec)
		if err := p.ValidateConfig(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("validate_config_missing_cli", func(t *testing.T) {
		exec := mockExecutor(map[string]struct {
			output string
			err    error
		}{
			"get-caller-identity": {"", fmt.Errorf("command not found: aws")},
		})
		p := NewAWSProvider("us-east-1", exec)
		err := p.ValidateConfig()
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("fetch_quotes_path_with_spaces", func(t *testing.T) {
		var captured string
		exec := func(ctx context.Context, command string) (string, error) {
			captured = command
			return "value", nil
		}
		p := NewAWSProvider("us-east-1", exec)
		_, err := p.Fetch(context.Background(), "my secret/path")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(captured, "'my secret/path'") {
			t.Errorf("expected quoted path in command, got: %s", captured)
		}
	})

	t.Run("fetch_quotes_path_with_single_quotes", func(t *testing.T) {
		var captured string
		exec := func(ctx context.Context, command string) (string, error) {
			captured = command
			return "value", nil
		}
		p := NewAWSProvider("us-east-1", exec)
		_, err := p.Fetch(context.Background(), "it's-a-secret")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(captured, "'it'\\''s-a-secret'") {
			t.Errorf("expected escaped single quote in command, got: %s", captured)
		}
	})

	t.Run("validate_config_quotes_region", func(t *testing.T) {
		var captured string
		exec := func(ctx context.Context, command string) (string, error) {
			captured = command
			return "{}", nil
		}
		p := NewAWSProvider("us-east-1", exec)
		_ = p.ValidateConfig()
		if !strings.Contains(captured, "'us-east-1'") {
			t.Errorf("expected quoted region in command, got: %s", captured)
		}
	})

	t.Run("validate_config_bad_credentials", func(t *testing.T) {
		exec := mockExecutor(map[string]struct {
			output string
			err    error
		}{
			"get-caller-identity": {"", fmt.Errorf("InvalidClientTokenId: bad creds")},
		})
		p := NewAWSProvider("us-east-1", exec)
		err := p.ValidateConfig()
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, ErrProviderAuth) {
			t.Fatalf("expected ErrProviderAuth, got %v", err)
		}
	})
}
