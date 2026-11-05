package vault

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

const (
	gcpAuthHint    = "Run 'gcloud auth login' or check GCP credentials. See https://cloud.google.com/sdk/gcloud/reference/auth/login"
	gcpInstallHint = "Install the Google Cloud SDK: https://cloud.google.com/sdk/docs/install"
)

var gcpAuthErrorPatterns = []string{
	"UNAUTHENTICATED",
	"PERMISSION_DENIED",
	"gcloud auth login",
	"not authorized",
}

// GCPProvider fetches secrets from GCP Secret Manager via the gcloud CLI.
type GCPProvider struct {
	project string
	execute CommandExecutor
}

// NewGCPProvider creates a provider that shells out to the gcloud CLI.
func NewGCPProvider(project string, exec CommandExecutor) *GCPProvider {
	return &GCPProvider{project: project, execute: exec}
}

// Name returns the provider identifier.
func (p *GCPProvider) Name() string { return ProviderGCP }

// Fetch retrieves a single secret by path from GCP Secret Manager.
func (p *GCPProvider) Fetch(ctx context.Context, path string) (string, error) {
	cmd := fmt.Sprintf(
		"gcloud secrets versions access latest --secret=%s --project=%s",
		shellQuote(path), shellQuote(p.project),
	)
	out, err := executeProvider(ctx, p.execute, cmd, "gcloud", []string{
		"secrets", "versions", "access", "latest", "--secret=" + path, "--project=" + p.project,
	}, nil)
	if err != nil {
		return "", p.classifyError(err, path)
	}
	return out, nil
}

// BulkFetch retrieves multiple secrets by calling Fetch per path.
func (p *GCPProvider) BulkFetch(ctx context.Context, paths []string) (map[string]string, error) {
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

// ValidateConfig checks that the gcloud CLI is available and credentials are valid.
func (p *GCPProvider) ValidateConfig() error {
	_, err := executeProvider(context.Background(), p.execute, "gcloud config get-value project", "gcloud", []string{"config", "get-value", "project"}, nil)
	if err != nil {
		return p.classifyError(err, "")
	}
	return nil
}

func (p *GCPProvider) classifyError(err error, path string) error {
	msg := providerDiagnostic(err)
	if errors.Is(err, exec.ErrNotFound) {
		return fmt.Errorf("%w: gcloud CLI not installed: %w. %s", ErrProviderAuth, err, gcpInstallHint)
	}
	if strings.Contains(msg, "command not found") || strings.Contains(msg, "gcloud: not found") {
		return classifiedProviderError(err, ErrProviderAuth, "gcloud CLI not installed", ". "+gcpInstallHint)
	}
	for _, pattern := range gcpAuthErrorPatterns {
		if strings.Contains(msg, pattern) {
			return classifiedProviderError(err, ErrProviderAuth, err.Error(), ". "+gcpAuthHint)
		}
	}
	if strings.Contains(msg, "NOT_FOUND") || strings.Contains(msg, "not found") {
		return classifiedProviderError(err, ErrSecretNotFound, path, "")
	}
	return fmt.Errorf("gcp secret manager: %w", err)
}
