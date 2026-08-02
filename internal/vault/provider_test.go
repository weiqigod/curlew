package vault

import (
	"testing"

	"github.com/weiqigod/curlew/internal/variable"
)

// TestCommandExecutorType verifies variable.ExecuteCommand has compatible signature.
func TestCommandExecutorType(t *testing.T) {
	var exec CommandExecutor = variable.ExecuteCommand
	_ = exec
}

// Compile-time check that AWSProvider satisfies Provider.
var _ Provider = (*AWSProvider)(nil)

// Compile-time check that AzureProvider satisfies Provider.
var _ Provider = (*AzureProvider)(nil)

// Compile-time check that HashiCorpProvider satisfies Provider.
var _ Provider = (*HashiCorpProvider)(nil)

// Compile-time check that GCPProvider satisfies Provider.
var _ Provider = (*GCPProvider)(nil)

// Compile-time check that OnePasswordProvider satisfies Provider.
var _ Provider = (*OnePasswordProvider)(nil)
