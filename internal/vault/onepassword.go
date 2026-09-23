package vault

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

const (
	onePasswordAuthHint    = "Sign in to 1Password CLI: run 'op signin'. See https://developer.1password.com/docs/cli/sign-in-manually/"
	onePasswordInstallHint = "Install the 1Password CLI: https://developer.1password.com/docs/cli/get-started/"
)

var onePasswordAuthErrorPatterns = []string{
	"not currently signed in",
	"not signed in",
	"sign in",
	"session expired",
	"authorization",
}

var onePasswordNotFoundPatterns = []string{
	"no item named",
	"isn't an item",
}

// OnePasswordProvider fetches secrets from 1Password via the op CLI.
type OnePasswordProvider struct {
	execute CommandExecutor
}

// NewOnePasswordProvider creates a provider that shells out to the op CLI.
func NewOnePasswordProvider(exec CommandExecutor) *OnePasswordProvider {
	return &OnePasswordProvider{execute: exec}
}

// Name returns the provider identifier.
func (p *OnePasswordProvider) Name() string { return Provider1Password }

// Fetch retrieves a single secret by path from 1Password.
// Paths prefixed with "op://" use `op read`; plain item names use `op item get`.
func (p *OnePasswordProvider) Fetch(ctx context.Context, path string) (string, error) {
	var cmd string
	var args []string
	if strings.HasPrefix(path, "op://") {
		cmd = fmt.Sprintf("op read %s", shellQuote(path))
		args = []string{"read", path}
	} else {
		cmd = fmt.Sprintf("op item get %s --format json", shellQuote(path))
		args = []string{"item", "get", path, "--format", "json"}
	}
	out, err := executeProvider(ctx, p.execute, cmd, "op", args, nil)
	if err != nil {
		return "", p.classifyError(err, path)
	}
	return out, nil
}

// BulkFetch retrieves multiple secrets by calling Fetch per path.
func (p *OnePasswordProvider) BulkFetch(ctx context.Context, paths []string) (map[string]string, error) {
	results := make(map[string]string, len(paths))
	for _, path := range paths {
		val, err := p.Fetch(ctx, path)
		if err != nil {
			return nil, err
		}
		results[path] = val
	}
	return results, nil
}

// ValidateConfig checks that the op CLI is installed and the user is signed in.
func (p *OnePasswordProvider) ValidateConfig() error {
	_, err := executeProvider(context.Background(), p.execute, "op whoami", "op", []string{"whoami"}, nil)
	if err != nil {
		return p.classifyError(err, "")
	}
	return nil
}

func (p *OnePasswordProvider) classifyError(err error, path string) error {
	msg := providerDiagnostic(err)
	if errors.Is(err, exec.ErrNotFound) {
		return fmt.Errorf("%w: op CLI not installed: %w. %s", ErrProviderAuth, err, onePasswordInstallHint)
	}
	// CLI not installed — surface install URL (behavior 4).
	if strings.Contains(msg, "command not found") || strings.Contains(msg, "op: not found") {
		return classifiedProviderError(err, ErrProviderAuth, "op CLI not installed", ". "+onePasswordInstallHint)
	}
	for _, pattern := range onePasswordNotFoundPatterns {
		if strings.Contains(msg, pattern) {
			return classifiedProviderError(err, ErrSecretNotFound, path, "")
		}
	}
	for _, pattern := range onePasswordAuthErrorPatterns {
		if strings.Contains(msg, pattern) {
			return classifiedProviderError(err, ErrProviderAuth, err.Error(), ". "+onePasswordAuthHint)
		}
	}
	return fmt.Errorf("1password: %w", err)
}
