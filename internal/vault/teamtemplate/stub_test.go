package teamtemplate

import (
	"context"
	"fmt"
	"testing"
)

func TestStubProvider_Fetch(t *testing.T) {
	tests := []struct {
		name     string
		env      string
		provider string
		path     string
		want     string
	}{
		{
			name:     "production AWS path",
			env:      "production",
			provider: "aws-secrets-manager",
			path:     "prod/api-key",
			want:     fmt.Sprintf(stubValueFormat, "production", "prod/api-key"),
		},
		{
			name:     "staging Azure path",
			env:      "staging",
			provider: "azure-key-vault",
			path:     "staging-api-key",
			want:     fmt.Sprintf(stubValueFormat, "staging", "staging-api-key"),
		},
		{
			name:     "path with field suffix preserved",
			env:      "production",
			provider: "aws-secrets-manager",
			path:     "prod/db#password",
			want:     fmt.Sprintf(stubValueFormat, "production", "prod/db#password"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewStubProvider(tt.env, tt.provider)
			got, err := p.Fetch(context.Background(), tt.path)
			if err != nil {
				t.Fatalf("Fetch() unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("Fetch() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStubProvider_BulkFetch_ReturnsMap(t *testing.T) {
	p := NewStubProvider("production", "aws-secrets-manager")
	paths := []string{"prod/api-key", "prod/db-password", "prod/api-key"} // duplicate
	got, err := p.BulkFetch(context.Background(), paths)
	if err != nil {
		t.Fatalf("BulkFetch() unexpected error: %v", err)
	}
	// Both unique paths should be present
	if len(got) != 2 {
		t.Fatalf("BulkFetch() returned %d entries, want 2; got: %v", len(got), got)
	}
	for _, p := range []string{"prod/api-key", "prod/db-password"} {
		if _, ok := got[p]; !ok {
			t.Errorf("BulkFetch() missing path %q", p)
		}
	}
}

func TestStubProvider_ValidateConfig_NeverErrors(t *testing.T) {
	p := NewStubProvider("env", "aws-secrets-manager")
	if err := p.ValidateConfig(); err != nil {
		t.Errorf("ValidateConfig() = %v, want nil", err)
	}
}

func TestStubProvider_Name_IncludesProvider(t *testing.T) {
	tests := []struct {
		provider string
		wantSub  string
	}{
		{"aws-secrets-manager", "aws-secrets-manager"},
		{"azure-key-vault", "azure-key-vault"},
	}
	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			p := NewStubProvider("env", tt.provider)
			name := p.Name()
			if name == "" {
				t.Fatal("Name() returned empty string")
			}
			// Name should include the provider identifier
			found := false
			if len(name) >= len(tt.wantSub) {
				for i := 0; i <= len(name)-len(tt.wantSub); i++ {
					if name[i:i+len(tt.wantSub)] == tt.wantSub {
						found = true
						break
					}
				}
			}
			if !found {
				t.Errorf("Name() = %q, want it to contain %q", name, tt.wantSub)
			}
		})
	}
}
