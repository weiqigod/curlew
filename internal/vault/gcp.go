package vault

import (
	"context"
	"fmt"
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
	out, err := p.execute(ctx, cmd)
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
	_, err := p.execute(context.Background(), "gcloud config get-value project")
	if err != nil {
		return p.classifyError(err, "")
	}
	return nil
}

func (p *GCPProvider) classifyError(err error, path string) error {
	msg := err.Error()
	if strings.Contains(msg, "command not found") || strings.Contains(msg, "gcloud: not found") {
		return fmt.Errorf("%w: gcloud CLI not installed. %s", ErrProviderAuth, gcpInstallHint)
	}
	for _, pattern := range gcpAuthErrorPatterns {
		if strings.Contains(msg, pattern) {
			return fmt.Errorf("%w: %s. %s", ErrProviderAuth, msg, gcpAuthHint)
		}
	}
	if strings.Contains(msg, "NOT_FOUND") || strings.Contains(msg, "not found") {
		return fmt.Errorf("%w: %s", ErrSecretNotFound, path)
	}
	return fmt.Errorf("gcp secret manager: %w", err)
}
