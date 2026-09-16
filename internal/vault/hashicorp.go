package vault

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	hashicorpAuthHint    = "Check your Vault token or AppRole credentials. See https://developer.hashicorp.com/vault/docs/auth"
	hashicorpNetworkHint = "Check that the Vault server is reachable at the configured address. See https://developer.hashicorp.com/vault/docs/configuration/listener/tcp"
)

var hashicorpAuthErrorPatterns = []string{
	"permission denied",
	"missing client token",
	"invalid role",
	"invalid secret",
}

var hashicorpNetworkErrorPatterns = []string{
	"connection refused",
	"no such host",
	"dial tcp",
	"i/o timeout",
	"certificate",
	"tls:",
}

// HashiCorpProvider fetches secrets from HashiCorp Vault via the vault CLI.
type HashiCorpProvider struct {
	address string
	auth    AuthConfig
	token   string // resolved auth token (set on first use for approle)
	execute CommandExecutor
}

// NewHashiCorpProvider creates a provider that shells out to the vault CLI.
func NewHashiCorpProvider(address string, auth AuthConfig, exec CommandExecutor) *HashiCorpProvider {
	p := &HashiCorpProvider{
		address: address,
		auth:    auth,
		execute: exec,
	}
	if auth.Method == "token" {
		p.token = auth.Token
	}
	return p
}

// Name returns the provider identifier.
func (p *HashiCorpProvider) Name() string {
	return ProviderHashiCorp
}

// Fetch retrieves a single secret by path from HashiCorp Vault.
func (p *HashiCorpProvider) Fetch(ctx context.Context, path string) (string, error) {
	if err := p.ensureToken(ctx); err != nil {
		return "", err
	}

	cmd := fmt.Sprintf(
		"VAULT_ADDR=%s VAULT_TOKEN=%s vault kv get -format=json %s",
		shellQuote(p.address), shellQuote(p.token), shellQuote(path),
	)
	out, err := executeProvider(ctx, p.execute, cmd, "vault", []string{"kv", "get", "-format=json", path},
		[]string{"VAULT_ADDR=" + p.address, "VAULT_TOKEN=" + p.token})
	if err != nil {
		return "", p.classifyError(err, path)
	}
	return p.extractSecretData(out)
}

// BulkFetch retrieves multiple secrets by calling Fetch per path.
func (p *HashiCorpProvider) BulkFetch(ctx context.Context, paths []string) (map[string]string, error) {
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

// ValidateConfig checks that the vault CLI is available and credentials are valid.
func (p *HashiCorpProvider) ValidateConfig() error {
	if err := p.ensureToken(context.Background()); err != nil {
		return err
	}

	cmd := fmt.Sprintf(
		"VAULT_ADDR=%s VAULT_TOKEN=%s vault token lookup -format=json",
		shellQuote(p.address), shellQuote(p.token),
	)
	_, err := executeProvider(context.Background(), p.execute, cmd, "vault", []string{"token", "lookup", "-format=json"},
		[]string{"VAULT_ADDR=" + p.address, "VAULT_TOKEN=" + p.token})
	if err != nil {
		return p.classifyError(err, "")
	}
	return nil
}

// ensureToken resolves the auth token, performing AppRole login if needed.
func (p *HashiCorpProvider) ensureToken(ctx context.Context) error {
	if p.token != "" {
		return nil
	}
	if p.auth.Method == "approle" {
		tok, err := p.approleLogin(ctx)
		if err != nil {
			return err
		}
		p.token = tok
	}
	return nil
}

// approleLogin performs AppRole authentication and returns the client token.
func (p *HashiCorpProvider) approleLogin(ctx context.Context) (string, error) {
	cmd := fmt.Sprintf(
		"VAULT_ADDR=%s vault write -format=json auth/approle/login role_id=%s secret_id=%s",
		shellQuote(p.address), shellQuote(p.auth.RoleID), shellQuote(p.auth.SecretID),
	)
	out, err := executeProvider(ctx, p.execute, cmd, "vault", []string{
		"write", "-format=json", "auth/approle/login", "role_id=" + p.auth.RoleID, "secret_id=" + p.auth.SecretID,
	}, []string{"VAULT_ADDR=" + p.address})
	if err != nil {
		return "", p.classifyError(err, "")
	}

	var resp struct {
		Auth struct {
			ClientToken string `json:"client_token"`
		} `json:"auth"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		return "", fmt.Errorf("hashicorp vault: failed to parse approle login response: %w", err)
	}
	if resp.Auth.ClientToken == "" {
		return "", fmt.Errorf("%w: approle login returned empty token", ErrProviderAuth)
	}
	return resp.Auth.ClientToken, nil
}

// classifyError inspects the error message for known HashiCorp error patterns.
func (p *HashiCorpProvider) classifyError(err error, path string) error {
	msg := providerDiagnostic(err)

	for _, pattern := range hashicorpNetworkErrorPatterns {
		if strings.Contains(msg, pattern) {
			return fmt.Errorf("hashicorp vault: %w. %s", err, hashicorpNetworkHint)
		}
	}

	if strings.Contains(msg, "No value found") || strings.Contains(msg, "secret not found") {
		return classifiedProviderError(err, ErrSecretNotFound, path, "")
	}

	for _, pattern := range hashicorpAuthErrorPatterns {
		if strings.Contains(msg, pattern) {
			return classifiedProviderError(err, ErrProviderAuth, err.Error(), ". "+hashicorpAuthHint)
		}
	}

	return fmt.Errorf("hashicorp vault: %w", err)
}

// extractSecretData parses the vault kv get JSON output and returns
// the .data.data contents as compact JSON.
func (p *HashiCorpProvider) extractSecretData(jsonOutput string) (string, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(jsonOutput), &raw); err != nil {
		return "", fmt.Errorf("hashicorp vault: failed to parse response: %w", err)
	}

	dataField, ok := raw["data"]
	if !ok {
		return "", fmt.Errorf("hashicorp vault: response missing 'data' field")
	}

	var inner map[string]json.RawMessage
	if err := json.Unmarshal(dataField, &inner); err != nil {
		return "", fmt.Errorf("hashicorp vault: failed to parse data field: %w", err)
	}

	secretData, ok := inner["data"]
	if !ok {
		return "", fmt.Errorf("hashicorp vault: response missing 'data.data' field")
	}

	// Re-serialize as compact JSON.
	var parsed interface{}
	if err := json.Unmarshal(secretData, &parsed); err != nil {
		return "", fmt.Errorf("hashicorp vault: failed to parse secret data: %w", err)
	}
	compact, err := json.Marshal(parsed)
	if err != nil {
		return "", fmt.Errorf("hashicorp vault: failed to serialize secret data: %w", err)
	}
	return string(compact), nil
}
