package vault

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestOnePasswordProvider(t *testing.T) {
	t.Run("name_returns_1password", func(t *testing.T) {
		p := NewOnePasswordProvider(nil)
		if got := p.Name(); got != Provider1Password {
			t.Errorf("got %q, want %q", got, Provider1Password)
		}
	})

	t.Run("fetch_op_uri_uses_op_read", func(t *testing.T) {
		var captured string
		exec := func(ctx context.Context, command string) (string, error) {
			captured = command
			return "secret-value", nil
		}
		p := NewOnePasswordProvider(exec)
		got, err := p.Fetch(context.Background(), "op://vault/item/field")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "secret-value" {
			t.Errorf("got %q, want %q", got, "secret-value")
		}
		if !strings.HasPrefix(captured, "op read ") {
			t.Errorf("expected 'op read' command, got: %s", captured)
		}
	})

	t.Run("fetch_plain_name_uses_op_item_get", func(t *testing.T) {
		var captured string
		exec := func(ctx context.Context, command string) (string, error) {
			captured = command
			return `{"id":"abc"}`, nil
		}
		p := NewOnePasswordProvider(exec)
		_, err := p.Fetch(context.Background(), "my-item")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.HasPrefix(captured, "op item get ") {
			t.Errorf("expected 'op item get' command, got: %s", captured)
		}
		if !strings.Contains(captured, "--format json") {
			t.Errorf("expected '--format json' in command, got: %s", captured)
		}
	})

	t.Run("fetch_op_uri_command_format_is_correct", func(t *testing.T) {
		var captured string
		exec := func(ctx context.Context, command string) (string, error) {
			captured = command
			return "value", nil
		}
		p := NewOnePasswordProvider(exec)
		_, err := p.Fetch(context.Background(), "op://My Vault/item/field")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := "op read 'op://My Vault/item/field'"
		if captured != want {
			t.Errorf("got command %q, want %q", captured, want)
		}
	})

	t.Run("fetch_plain_name_command_format_is_correct", func(t *testing.T) {
		var captured string
		exec := func(ctx context.Context, command string) (string, error) {
			captured = command
			return `{}`, nil
		}
		p := NewOnePasswordProvider(exec)
		_, err := p.Fetch(context.Background(), "my-item")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := "op item get 'my-item' --format json"
		if captured != want {
			t.Errorf("got command %q, want %q", captured, want)
		}
	})

	t.Run("fetch_quotes_path_with_spaces", func(t *testing.T) {
		var captured string
		exec := func(ctx context.Context, command string) (string, error) {
			captured = command
			return "value", nil
		}
		p := NewOnePasswordProvider(exec)
		_, err := p.Fetch(context.Background(), "my item name")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(captured, "'my item name'") {
			t.Errorf("expected quoted path in command, got: %s", captured)
		}
	})

	t.Run("fetch_not_found_returns_sentinel", func(t *testing.T) {
		exec := mockExecutor(map[string]struct {
			output string
			err    error
		}{
			"op item get": {"", fmt.Errorf("[ERROR] 2024/01/01 no item named \"missing\" in \"Personal\"")},
		})
		p := NewOnePasswordProvider(exec)
		_, err := p.Fetch(context.Background(), "missing")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, ErrSecretNotFound) {
			t.Fatalf("expected ErrSecretNotFound, got %v", err)
		}
	})

	t.Run("fetch_not_signed_in_returns_auth_error", func(t *testing.T) {
		exec := mockExecutor(map[string]struct {
			output string
			err    error
		}{
			"op item get": {"", fmt.Errorf("[ERROR] 2024/01/01 not currently signed in to any account")},
		})
		p := NewOnePasswordProvider(exec)
		_, err := p.Fetch(context.Background(), "item")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, ErrProviderAuth) {
			t.Fatalf("expected ErrProviderAuth, got %v", err)
		}
	})

	t.Run("fetch_session_expired_returns_auth_error", func(t *testing.T) {
		exec := mockExecutor(map[string]struct {
			output string
			err    error
		}{
			"op item get": {"", fmt.Errorf("[ERROR] 2024/01/01 session expired, please sign in again")},
		})
		p := NewOnePasswordProvider(exec)
		_, err := p.Fetch(context.Background(), "item")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, ErrProviderAuth) {
			t.Fatalf("expected ErrProviderAuth, got %v", err)
		}
	})

	t.Run("fetch_cli_not_installed_returns_auth_error_with_install_url", func(t *testing.T) {
		exec := mockExecutor(map[string]struct {
			output string
			err    error
		}{
			"op item get": {"", fmt.Errorf("command not found: op")},
		})
		p := NewOnePasswordProvider(exec)
		_, err := p.Fetch(context.Background(), "item")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, ErrProviderAuth) {
			t.Fatalf("expected ErrProviderAuth, got %v", err)
		}
		if !strings.Contains(err.Error(), "https://developer.1password.com") {
			t.Errorf("expected install URL in error, got: %v", err)
		}
	})

	t.Run("fetch_generic_error_wraps_original", func(t *testing.T) {
		exec := mockExecutor(map[string]struct {
			output string
			err    error
		}{
			"op item get": {"", fmt.Errorf("some unexpected error")},
		})
		p := NewOnePasswordProvider(exec)
		_, err := p.Fetch(context.Background(), "item")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if errors.Is(err, ErrSecretNotFound) || errors.Is(err, ErrProviderAuth) {
			t.Errorf("expected wrapped generic error, got sentinel: %v", err)
		}
		if !strings.Contains(err.Error(), "1password") {
			t.Errorf("expected '1password' prefix in error, got: %v", err)
		}
	})

	t.Run("bulk_fetch_calls_fetch_per_path", func(t *testing.T) {
		callCount := 0
		exec := func(ctx context.Context, command string) (string, error) {
			callCount++
			if strings.Contains(command, "item-a") {
				return "value-a", nil
			}
			if strings.Contains(command, "item-b") {
				return "value-b", nil
			}
			return "", fmt.Errorf("unexpected command: %s", command)
		}
		p := NewOnePasswordProvider(exec)
		got, err := p.BulkFetch(context.Background(), []string{"item-a", "item-b"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if callCount != 2 {
			t.Errorf("expected 2 calls, got %d", callCount)
		}
		if got["item-a"] != "value-a" || got["item-b"] != "value-b" {
			t.Errorf("unexpected results: %v", got)
		}
	})

	t.Run("bulk_fetch_returns_all_results", func(t *testing.T) {
		exec := func(_ context.Context, _ string) (string, error) {
			return "val", nil
		}
		p := NewOnePasswordProvider(exec)
		got, err := p.BulkFetch(context.Background(), []string{"a", "b", "c"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 3 {
			t.Errorf("expected 3 results, got %d", len(got))
		}
	})

	t.Run("bulk_fetch_stops_on_first_error", func(t *testing.T) {
		exec := func(_ context.Context, command string) (string, error) {
			if strings.Contains(command, "'b'") {
				return "", fmt.Errorf("no item named \"b\"")
			}
			return "val", nil
		}
		p := NewOnePasswordProvider(exec)
		_, err := p.BulkFetch(context.Background(), []string{"a", "b", "c"})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("validate_config_success", func(t *testing.T) {
		exec := mockExecutor(map[string]struct {
			output string
			err    error
		}{
			"op whoami": {`{"url":"my.1password.com"}`, nil},
		})
		p := NewOnePasswordProvider(exec)
		if err := p.ValidateConfig(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("validate_config_cli_not_installed_suggests_install_url", func(t *testing.T) {
		exec := mockExecutor(map[string]struct {
			output string
			err    error
		}{
			"op whoami": {"", fmt.Errorf("op: not found")},
		})
		p := NewOnePasswordProvider(exec)
		err := p.ValidateConfig()
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, ErrProviderAuth) {
			t.Fatalf("expected ErrProviderAuth, got %v", err)
		}
		if !strings.Contains(err.Error(), "https://developer.1password.com") {
			t.Errorf("expected install URL in error, got: %v", err)
		}
	})

	t.Run("validate_config_not_signed_in_returns_auth_error", func(t *testing.T) {
		exec := mockExecutor(map[string]struct {
			output string
			err    error
		}{
			"op whoami": {"", fmt.Errorf("not currently signed in to any account")},
		})
		p := NewOnePasswordProvider(exec)
		err := p.ValidateConfig()
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, ErrProviderAuth) {
			t.Fatalf("expected ErrProviderAuth, got %v", err)
		}
	})
}
