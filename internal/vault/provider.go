package vault

import (
	"context"
	"errors"
	"fmt"

	"github.com/weiqigod/curlew/internal/variable"
)

// ErrSecretNotFound is returned when a requested secret path does not exist.
var ErrSecretNotFound = errors.New("secret not found")

// ErrProviderAuth is returned when provider authentication fails.
var ErrProviderAuth = errors.New("vault provider authentication failed")

// Provider defines the interface for vault secret providers.
// Implementations shell out to CLI tools rather than using SDK libraries.
type Provider interface {
	// Name returns the provider identifier (e.g., "aws-secrets-manager").
	Name() string

	// Fetch retrieves a single secret value by path.
	Fetch(ctx context.Context, path string) (string, error)

	// BulkFetch retrieves multiple secrets by path.
	// Falls back to individual Fetch calls if bulk is not natively supported.
	BulkFetch(ctx context.Context, paths []string) (map[string]string, error)

	// ValidateConfig checks prerequisites (CLI tools, credentials).
	ValidateConfig() error
}

// CommandExecutor runs a shell command and returns stdout.
// Injected for testability (avoids real CLI calls in tests).
type CommandExecutor func(ctx context.Context, command string) (string, error)

func executeProvider(ctx context.Context, legacy CommandExecutor, command, program string, args, env []string) (string, error) {
	if legacy != nil {
		return legacy(ctx, command)
	}
	return variable.ExecuteProgram(ctx, program, args, env)
}

func providerDiagnostic(err error) string {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return ""
	}
	if diagnostic := variable.CommandDiagnostic(err); diagnostic != "" {
		return diagnostic
	}
	return err.Error()
}

func classifiedProviderError(err, kind error, legacyDetail, hint string) error {
	if errors.Is(err, variable.ErrCommandFailed) {
		return fmt.Errorf("%w: %w%s", kind, err, hint)
	}
	return fmt.Errorf("%w: %s%s", kind, legacyDetail, hint)
}
