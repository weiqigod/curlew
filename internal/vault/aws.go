package vault

import (
	"context"
	"fmt"
	"strings"
)

// awsAuthHint is appended to authentication errors to guide the user.
const awsAuthHint = "Run 'aws configure' or check AWS credentials. See https://docs.aws.amazon.com/cli/latest/userguide/cli-configure-files.html"

// awsAuthErrorPatterns are substrings in AWS CLI stderr that indicate auth failure.
var awsAuthErrorPatterns = []string{
	"InvalidClientTokenId",
	"ExpiredToken",
	"credentials",
}

// shellQuote wraps s in single quotes, escaping any embedded single quotes
// using the standard shell idiom: replace ' with '\” (end quote, escaped
// literal quote, restart quote).
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

// AWSProvider fetches secrets from AWS Secrets Manager via the AWS CLI.
type AWSProvider struct {
	region  string
	execute CommandExecutor
}

// NewAWSProvider creates a provider that shells out to the AWS CLI.
func NewAWSProvider(region string, exec CommandExecutor) *AWSProvider {
	return &AWSProvider{region: region, execute: exec}
}

// Name returns the provider identifier.
func (p *AWSProvider) Name() string {
	return ProviderAWS
}

// Fetch retrieves a single secret by path from AWS Secrets Manager.
func (p *AWSProvider) Fetch(ctx context.Context, path string) (string, error) {
	cmd := fmt.Sprintf(
		"aws secretsmanager get-secret-value --secret-id %s --region %s --query SecretString --output text",
		shellQuote(path), shellQuote(p.region),
	)
	out, err := p.execute(ctx, cmd)
	if err != nil {
		return "", p.classifyError(err, path)
	}
	return out, nil
}

// BulkFetch retrieves multiple secrets by calling Fetch per path.
func (p *AWSProvider) BulkFetch(ctx context.Context, paths []string) (map[string]string, error) {
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

// ValidateConfig checks that the AWS CLI is available and credentials are valid.
func (p *AWSProvider) ValidateConfig() error {
	cmd := fmt.Sprintf("aws sts get-caller-identity --region %s", shellQuote(p.region))
	_, err := p.execute(context.Background(), cmd)
	if err != nil {
		return p.classifyError(err, "")
	}
	return nil
}

// classifyError inspects the error message for known AWS error patterns.
func (p *AWSProvider) classifyError(err error, path string) error {
	msg := err.Error()

	if strings.Contains(msg, "ResourceNotFoundException") {
		return fmt.Errorf("%w: %s", ErrSecretNotFound, path)
	}

	for _, pattern := range awsAuthErrorPatterns {
		if strings.Contains(msg, pattern) {
			return fmt.Errorf("%w: %s. %s", ErrProviderAuth, msg, awsAuthHint)
		}
	}

	return fmt.Errorf("aws secrets manager: %w", err)
}
