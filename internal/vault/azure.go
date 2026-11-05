package vault

import (
	"context"
	"fmt"
	"strings"
)

const azureAuthHint = "Run 'az login' or check Azure CLI credentials. See https://learn.microsoft.com/en-us/cli/azure/authenticate-azure-cli"

var azureAuthErrorPatterns = []string{
	"AADSTS",
	"Please run 'az login'",
	"az login",
	"not logged in",
	"InvalidAuthenticationToken",
}

// AzureProvider fetches secrets from Azure Key Vault via the Azure CLI.
type AzureProvider struct {
	vaultName string
	execute   CommandExecutor
}

// NewAzureProvider creates a provider that shells out to the Azure CLI.
func NewAzureProvider(vaultName string, exec CommandExecutor) *AzureProvider {
	return &AzureProvider{vaultName: vaultName, execute: exec}
}

// Name returns the provider identifier.
func (p *AzureProvider) Name() string {
	return ProviderAzure
}

// Fetch retrieves a single secret by name from Azure Key Vault.
func (p *AzureProvider) Fetch(ctx context.Context, path string) (string, error) {
	cmd := fmt.Sprintf(
		"az keyvault secret show --name %s --vault-name %s --query value -o tsv",
		shellQuote(path), shellQuote(p.vaultName),
	)
	out, err := executeProvider(ctx, p.execute, cmd, "az", []string{
		"keyvault", "secret", "show", "--name", path, "--vault-name", p.vaultName,
		"--query", "value", "-o", "tsv",
	}, nil)
	if err != nil {
		return "", p.classifyError(err, path)
	}
	return out, nil
}

// BulkFetch retrieves multiple secrets by calling Fetch per path.
func (p *AzureProvider) BulkFetch(ctx context.Context, paths []string) (map[string]string, error) {
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

// ValidateConfig checks that the Azure CLI is available and credentials are valid.
func (p *AzureProvider) ValidateConfig() error {
	_, err := executeProvider(context.Background(), p.execute, "az account show", "az", []string{"account", "show"}, nil)
	if err != nil {
		return p.classifyError(err, "")
	}
	return nil
}

// classifyError inspects the error message for known Azure error patterns.
func (p *AzureProvider) classifyError(err error, path string) error {
	msg := providerDiagnostic(err)

	if strings.Contains(msg, "SecretNotFound") || strings.Contains(msg, "ResourceNotFound") {
		return classifiedProviderError(err, ErrSecretNotFound, path, "")
	}

	for _, pattern := range azureAuthErrorPatterns {
		if strings.Contains(msg, pattern) {
			return classifiedProviderError(err, ErrProviderAuth, err.Error(), ". "+azureAuthHint)
		}
	}

	return fmt.Errorf("azure key vault: %w", err)
}
