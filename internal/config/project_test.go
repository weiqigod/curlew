package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/peterlindqvist/apitest/internal/auth"
	"github.com/peterlindqvist/apitest/internal/output"
)

func TestParseProjectConfig(t *testing.T) {
	tests := []struct {
		name        string
		yaml        string
		wantProject string
		wantVars    map[string]string
		wantErr     error
	}{
		{
			name:        "valid with project_name and variables",
			yaml:        "project_name: MyApp\nvariables:\n  base_url: https://api.example.com\n  api_version: v1\n",
			wantProject: "MyApp",
			wantVars:    map[string]string{"base_url": "https://api.example.com", "api_version": "v1"},
		},
		{
			name:        "valid with project_name only",
			yaml:        "project_name: MyApp\n",
			wantProject: "MyApp",
			wantVars:    map[string]string{},
		},
		{
			name:     "valid with variables only",
			yaml:     "variables:\n  key: value\n",
			wantVars: map[string]string{"key": "value"},
		},
		{
			name:     "empty file returns zero struct",
			yaml:     "",
			wantVars: map[string]string{},
		},
		{
			name:    "invalid yaml returns ErrInvalidProjectConfig",
			yaml:    "variables: [not: a: map]",
			wantErr: ErrInvalidProjectConfig,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f, err := os.CreateTemp(t.TempDir(), "apitest-*.yaml")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := f.WriteString(tc.yaml); err != nil {
				t.Fatal(err)
			}
			if err := f.Close(); err != nil {
				t.Fatal(err)
			}

			cfg, err := ParseProjectConfig(f.Name())

			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("expected error %v, got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.ProjectName != tc.wantProject {
				t.Errorf("ProjectName = %q, want %q", cfg.ProjectName, tc.wantProject)
			}
			if len(cfg.Variables) != len(tc.wantVars) {
				t.Errorf("Variables = %v, want %v", cfg.Variables, tc.wantVars)
			}
			for k, v := range tc.wantVars {
				if cfg.Variables[k] != v {
					t.Errorf("Variables[%q] = %q, want %q", k, cfg.Variables[k], v)
				}
			}
		})
	}
}

func TestParseProjectConfig_Secrets(t *testing.T) {
	tests := []struct {
		name         string
		yaml         string
		wantNil      bool // true if cfg.Secrets should be nil
		wantProvider string
		wantErr      bool
	}{
		{
			name:         "valid_with_secrets_aws",
			yaml:         "project_name: test\nsecrets:\n  provider: aws-secrets-manager\n  region: us-east-1\n  keys:\n    api_key: prod/key",
			wantProvider: "aws-secrets-manager",
		},
		{
			name:         "valid_with_secrets_and_variables",
			yaml:         "project_name: test\nvariables:\n  base_url: http://localhost\nsecrets:\n  provider: 1password\n  keys:\n    k: v",
			wantProvider: "1password",
		},
		{
			name:    "unknown_secrets_provider_returns_error",
			yaml:    "project_name: test\nsecrets:\n  provider: unknown\n  keys:\n    k: v",
			wantErr: true,
		},
		{
			name:    "no_secrets_block_returns_nil",
			yaml:    "project_name: test",
			wantNil: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f, err := os.CreateTemp(t.TempDir(), "apitest-*.yaml")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := f.WriteString(tc.yaml); err != nil {
				t.Fatal(err)
			}
			if err := f.Close(); err != nil {
				t.Fatal(err)
			}

			cfg, err := ParseProjectConfig(f.Name())
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tc.wantNil {
				if cfg.Secrets != nil {
					t.Errorf("expected Secrets to be nil, got %+v", cfg.Secrets)
				}
				return
			}

			if cfg.Secrets == nil {
				t.Fatal("expected Secrets to be non-nil")
			}
			if cfg.Secrets.Provider != tc.wantProvider {
				t.Errorf("Secrets.Provider = %q, want %q", cfg.Secrets.Provider, tc.wantProvider)
			}
		})
	}
}

func TestParseProjectConfig_AuthProfiles(t *testing.T) {
	tests := []struct {
		name        string
		yaml        string
		wantLen     int
		wantProfile auth.Profile // checked only when wantLen == 1
		wantErr     bool
	}{
		{
			name:    "no auth_profiles block",
			yaml:    "project_name: test\n",
			wantLen: 0,
		},
		{
			name:    "dynamic profile with extract",
			yaml:    "project_name: test\nauth_profiles:\n  admin_token:\n    type: dynamic\n    collection: auth/login.yaml\n    extract: admin_token\n",
			wantLen: 1,
			wantProfile: auth.Profile{
				Name:       "admin_token",
				Type:       auth.ProfileDynamic,
				Collection: "auth/login.yaml",
				Extract:    "admin_token",
			},
		},
		{
			name:    "dynamic profile without extract (all vars)",
			yaml:    "project_name: test\nauth_profiles:\n  login:\n    type: dynamic\n    collection: auth/login.yaml\n",
			wantLen: 1,
			wantProfile: auth.Profile{
				Name:       "login",
				Type:       auth.ProfileDynamic,
				Collection: "auth/login.yaml",
			},
		},
		{
			name:    "unsupported type returns error",
			yaml:    "project_name: test\nauth_profiles:\n  login:\n    type: static\n    collection: auth/login.yaml\n",
			wantErr: true,
		},
		{
			name:    "missing collection returns error",
			yaml:    "project_name: test\nauth_profiles:\n  login:\n    type: dynamic\n",
			wantErr: true,
		},
		{
			name:    "multiple profiles sorted by name",
			yaml:    "project_name: test\nauth_profiles:\n  zebra:\n    type: dynamic\n    collection: auth/zebra.yaml\n  alpha:\n    type: dynamic\n    collection: auth/alpha.yaml\n",
			wantLen: 2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f, err := os.CreateTemp(t.TempDir(), "apitest-*.yaml")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := f.WriteString(tc.yaml); err != nil {
				t.Fatal(err)
			}
			if err := f.Close(); err != nil {
				t.Fatal(err)
			}

			cfg, parseErr := ParseProjectConfig(f.Name())
			if tc.wantErr {
				if parseErr == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if parseErr != nil {
				t.Fatalf("unexpected error: %v", parseErr)
			}
			if len(cfg.AuthProfiles) != tc.wantLen {
				t.Errorf("AuthProfiles len = %d, want %d", len(cfg.AuthProfiles), tc.wantLen)
			}
			if tc.wantLen == 1 && len(cfg.AuthProfiles) == 1 {
				got := cfg.AuthProfiles[0]
				if got.Name != tc.wantProfile.Name {
					t.Errorf("Profile.Name = %q, want %q", got.Name, tc.wantProfile.Name)
				}
				if got.Type != tc.wantProfile.Type {
					t.Errorf("Profile.Type = %q, want %q", got.Type, tc.wantProfile.Type)
				}
				if got.Collection != tc.wantProfile.Collection {
					t.Errorf("Profile.Collection = %q, want %q", got.Collection, tc.wantProfile.Collection)
				}
				if got.Extract != tc.wantProfile.Extract {
					t.Errorf("Profile.Extract = %q, want %q", got.Extract, tc.wantProfile.Extract)
				}
			}
			if tc.wantLen == 2 && len(cfg.AuthProfiles) == 2 {
				if cfg.AuthProfiles[0].Name != "alpha" {
					t.Errorf("profiles not sorted: first = %q, want %q", cfg.AuthProfiles[0].Name, "alpha")
				}
			}
		})
	}
}

func TestParseProjectConfig_AuthProfiles_CacheFields(t *testing.T) {
	tests := []struct {
		name              string
		yaml              string
		wantCacheTTL      int
		wantRefreshOnFail bool
	}{
		{
			name:         "dynamic profile with cache_ttl",
			yaml:         "auth_profiles:\n  tok:\n    type: dynamic\n    collection: auth.yaml\n    cache_ttl: 3600\n",
			wantCacheTTL: 3600, wantRefreshOnFail: false,
		},
		{
			name:              "dynamic profile with refresh_on_failure",
			yaml:              "auth_profiles:\n  tok:\n    type: dynamic\n    collection: auth.yaml\n    refresh_on_failure: true\n",
			wantCacheTTL:      0,
			wantRefreshOnFail: true,
		},
		{
			name:              "profile without cache fields defaults to zero",
			yaml:              "auth_profiles:\n  tok:\n    type: dynamic\n    collection: auth.yaml\n",
			wantCacheTTL:      0,
			wantRefreshOnFail: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f, err := os.CreateTemp(t.TempDir(), "apitest-*.yaml")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := f.WriteString(tc.yaml); err != nil {
				t.Fatal(err)
			}
			if err := f.Close(); err != nil {
				t.Fatal(err)
			}

			cfg, parseErr := ParseProjectConfig(f.Name())
			if parseErr != nil {
				t.Fatalf("unexpected error: %v", parseErr)
			}
			if len(cfg.AuthProfiles) != 1 {
				t.Fatalf("expected 1 profile, got %d", len(cfg.AuthProfiles))
			}
			p := cfg.AuthProfiles[0]
			if p.CacheTTL != tc.wantCacheTTL {
				t.Errorf("CacheTTL = %d, want %d", p.CacheTTL, tc.wantCacheTTL)
			}
			if p.RefreshOnFailure != tc.wantRefreshOnFail {
				t.Errorf("RefreshOnFailure = %v, want %v", p.RefreshOnFailure, tc.wantRefreshOnFail)
			}
		})
	}
}

func TestParseProjectConfig_Defaults(t *testing.T) {
	tests := []struct {
		name         string
		yaml         string
		wantRetryNil bool
		wantEnabled  *bool
		wantMax      *int
		wantErr      bool
	}{
		{
			name:         "no defaults block",
			yaml:         "project_name: test\n",
			wantRetryNil: true,
		},
		{
			name:         "defaults with retry enabled",
			yaml:         "project_name: test\ndefaults:\n  retry:\n    enabled: true\n    max_attempts: 5\n",
			wantRetryNil: false,
			wantEnabled:  boolPtr(true),
			wantMax:      intPtr(5),
		},
		{
			name:         "defaults with retry disabled",
			yaml:         "project_name: test\ndefaults:\n  retry:\n    enabled: false\n",
			wantRetryNil: false,
			wantEnabled:  boolPtr(false),
		},
		{
			name:    "invalid defaults",
			yaml:    "project_name: test\ndefaults: not_a_map\n",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f, err := os.CreateTemp(t.TempDir(), "apitest-*.yaml")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := f.WriteString(tc.yaml); err != nil {
				t.Fatal(err)
			}
			if err := f.Close(); err != nil {
				t.Fatal(err)
			}

			cfg, err := ParseProjectConfig(f.Name())
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tc.wantRetryNil {
				if cfg.Defaults.Retry != nil {
					t.Errorf("expected Defaults.Retry to be nil, got %+v", cfg.Defaults.Retry)
				}
				return
			}

			if cfg.Defaults.Retry == nil {
				t.Fatal("expected Defaults.Retry to be non-nil")
			}
			if tc.wantEnabled != nil {
				if cfg.Defaults.Retry.Enabled == nil || *cfg.Defaults.Retry.Enabled != *tc.wantEnabled {
					t.Errorf("Defaults.Retry.Enabled = %v, want %v", cfg.Defaults.Retry.Enabled, *tc.wantEnabled)
				}
			}
			if tc.wantMax != nil {
				if cfg.Defaults.Retry.MaxAttempts == nil || *cfg.Defaults.Retry.MaxAttempts != *tc.wantMax {
					t.Errorf("Defaults.Retry.MaxAttempts = %v, want %v", cfg.Defaults.Retry.MaxAttempts, *tc.wantMax)
				}
			}
		})
	}
}

func boolPtr(v bool) *bool    { return &v }
func intPtr(v int) *int       { return &v }
func strPtr(v string) *string { return &v }

func TestParseProjectConfig_FileNotFound(t *testing.T) {
	_, err := ParseProjectConfig("/nonexistent/path/apitest.yaml")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestFindProjectRoot(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(t *testing.T) string
		wantFound  bool
		wantRootFn func(startDir string) string // nil means do not assert root path
	}{
		{
			name: "found in same directory",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				if err := os.WriteFile(filepath.Join(dir, "apitest.yaml"), []byte("project_name: Test\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				return dir
			},
			wantFound: true,
		},
		{
			name: "found in parent directory",
			setup: func(t *testing.T) string {
				parent := t.TempDir()
				child := filepath.Join(parent, "sub")
				if err := os.Mkdir(child, 0o750); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(parent, "apitest.yaml"), []byte("project_name: Test\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				return child
			},
			wantFound: true,
		},
		{
			name: "found in grandparent directory",
			setup: func(t *testing.T) string {
				root := t.TempDir()
				mid := filepath.Join(root, "mid")
				leaf := filepath.Join(mid, "leaf")
				if err := os.MkdirAll(leaf, 0o750); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(root, "apitest.yaml"), []byte("project_name: Test\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				return leaf
			},
			wantFound:  true,
			wantRootFn: func(s string) string { return filepath.Dir(filepath.Dir(s)) },
		},
		{
			name: "not found returns false",
			setup: func(t *testing.T) string {
				return t.TempDir()
			},
			wantFound: false,
		},
		{
			name: "apitest.yml extension supported",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				if err := os.WriteFile(filepath.Join(dir, "apitest.yml"), []byte("project_name: Test\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				return dir
			},
			wantFound: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			startDir := tc.setup(t)
			root, found := FindProjectRoot(startDir)
			if found != tc.wantFound {
				t.Errorf("FindProjectRoot found = %v, want %v", found, tc.wantFound)
			}
			if tc.wantRootFn != nil {
				if wantRoot := tc.wantRootFn(startDir); root != wantRoot {
					t.Errorf("FindProjectRoot root = %q, want %q", root, wantRoot)
				}
			}
		})
	}
}

func TestFindProjectRoot_ReturnsCorrectDir(t *testing.T) {
	parent := t.TempDir()
	child := filepath.Join(parent, "sub")
	if err := os.Mkdir(child, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(parent, "apitest.yaml"), []byte("project_name: Test\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	root, found := FindProjectRoot(child)
	if !found {
		t.Fatal("expected to find project root")
	}
	if root != parent {
		t.Errorf("root = %q, want %q", root, parent)
	}
}

func TestLoadProjectConfig(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(t *testing.T) string
		wantVars    map[string]string
		wantRootSet bool
		wantRootFn  func(startDir string) string // nil means do not assert root path value
		wantErr     error
	}{
		{
			name: "loads from project root",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				content := "project_name: TestProject\nvariables:\n  url: https://example.com\n"
				if err := os.WriteFile(filepath.Join(dir, "apitest.yaml"), []byte(content), 0o600); err != nil {
					t.Fatal(err)
				}
				return dir
			},
			wantVars:    map[string]string{"url": "https://example.com"},
			wantRootSet: true,
			wantRootFn:  func(s string) string { return s },
		},
		{
			name: "no project root returns empty config no error",
			setup: func(t *testing.T) string {
				return t.TempDir()
			},
			wantVars:    map[string]string{},
			wantRootSet: false,
		},
		{
			name: "invalid yaml returns error",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				if err := os.WriteFile(filepath.Join(dir, "apitest.yaml"), []byte("variables: [not: a: map]"), 0o600); err != nil {
					t.Fatal(err)
				}
				return dir
			},
			wantErr: ErrInvalidProjectConfig,
		},
		{
			name: "yml extension supported",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				if err := os.WriteFile(filepath.Join(dir, "apitest.yml"), []byte("project_name: YML\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				return dir
			},
			wantVars:    map[string]string{},
			wantRootSet: true,
			wantRootFn:  func(s string) string { return s },
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			startDir := tc.setup(t)
			cfg, root, err := LoadProjectConfig(startDir)

			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("expected error %v, got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantRootSet && root == "" {
				t.Error("expected root to be set, got empty string")
			}
			if !tc.wantRootSet && root != "" {
				t.Errorf("expected root to be empty, got %q", root)
			}
			if tc.wantRootFn != nil {
				if wantRoot := tc.wantRootFn(startDir); root != wantRoot {
					t.Errorf("root = %q, want %q", root, wantRoot)
				}
			}
			for k, v := range tc.wantVars {
				if cfg.Variables[k] != v {
					t.Errorf("Variables[%q] = %q, want %q", k, cfg.Variables[k], v)
				}
			}
		})
	}
}

func TestParseProjectConfig_GraphQLDefaults(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantPS  *string // nil = expect GraphQL nil or empty partial_success
		wantErr bool
	}{
		{
			name:   "defaults.graphql.error_handling.partial_success warn",
			yaml:   "defaults:\n  graphql:\n    error_handling:\n      partial_success: warn\n",
			wantPS: strPtr("warn"),
		},
		{
			name:   "fail value",
			yaml:   "defaults:\n  graphql:\n    error_handling:\n      partial_success: fail\n",
			wantPS: strPtr("fail"),
		},
		{
			name:   "ignore value",
			yaml:   "defaults:\n  graphql:\n    error_handling:\n      partial_success: ignore\n",
			wantPS: strPtr("ignore"),
		},
		{
			name:    "invalid value returns ErrInvalidProjectConfig",
			yaml:    "defaults:\n  graphql:\n    error_handling:\n      partial_success: maybe\n",
			wantErr: true,
		},
		{
			name:   "defaults without graphql block",
			yaml:   "defaults:\n  retry:\n    max_attempts: 3\n",
			wantPS: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f, err := os.CreateTemp(t.TempDir(), "apitest-*.yaml")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := f.WriteString(tc.yaml); err != nil {
				t.Fatal(err)
			}
			if err := f.Close(); err != nil {
				t.Fatal(err)
			}

			cfg, parseErr := ParseProjectConfig(f.Name())
			if tc.wantErr {
				if parseErr == nil {
					t.Fatal("expected error, got nil")
				}
				if !errors.Is(parseErr, ErrInvalidProjectConfig) {
					t.Errorf("expected ErrInvalidProjectConfig, got %v", parseErr)
				}
				return
			}
			if parseErr != nil {
				t.Fatalf("unexpected error: %v", parseErr)
			}

			if tc.wantPS == nil {
				if cfg.Defaults.GraphQL != nil && cfg.Defaults.GraphQL.ErrorHandling.PartialSuccess != "" {
					t.Errorf("expected no GraphQL partial_success, got %q", cfg.Defaults.GraphQL.ErrorHandling.PartialSuccess)
				}
				return
			}

			if cfg.Defaults.GraphQL == nil {
				t.Fatal("expected Defaults.GraphQL to be non-nil")
			}
			if cfg.Defaults.GraphQL.ErrorHandling.PartialSuccess != *tc.wantPS {
				t.Errorf("PartialSuccess = %q, want %q", cfg.Defaults.GraphQL.ErrorHandling.PartialSuccess, *tc.wantPS)
			}
		})
	}
}

// TestParseProjectConfig_Output verifies that the optional output: block is
// correctly parsed and validated in ProjectConfig.
func TestParseProjectConfig_Output(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		want    *output.Config
		wantErr bool
	}{
		{
			name: "no_output",
			yaml: "project_name: demo\n",
			want: nil, wantErr: false,
		},
		{
			name: "full_output",
			yaml: "project_name: demo\noutput:\n  format: json\n  report: r.json\n  verbosity: normal\n",
			want: &output.Config{Format: "json", Report: "r.json", Verbosity: "normal"}, wantErr: false,
		},
		{
			name: "invalid_format",
			yaml: "project_name: demo\noutput:\n  format: yaml\n",
			want: nil, wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f, err := os.CreateTemp(t.TempDir(), "apitest-*.yaml")
			if err != nil {
				t.Fatalf("create temp: %v", err)
			}
			_, _ = f.WriteString(tc.yaml)
			_ = f.Close()
			cfg, parseErr := ParseProjectConfig(f.Name())
			if (parseErr != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", parseErr, tc.wantErr)
			}
			if tc.wantErr {
				return
			}
			if (cfg.Output == nil) != (tc.want == nil) {
				t.Fatalf("Output nil mismatch: got nil=%v, want nil=%v", cfg.Output == nil, tc.want == nil)
			}
			if tc.want != nil && *cfg.Output != *tc.want {
				t.Errorf("Output = %+v, want %+v", *cfg.Output, *tc.want)
			}
		})
	}
}

// ---- Step 5: Config locale plumbing ----

func TestParseProjectConfig_Locale(t *testing.T) {
	tests := []struct {
		name       string
		yaml       string
		wantLocale string
		wantErr    bool
	}{
		{
			name:       "config block with locale",
			yaml:       "project_name: Test\nconfig:\n  locale: de-DE\n",
			wantLocale: "de-DE",
		},
		{
			name:       "no config block returns empty locale",
			yaml:       "project_name: Test\n",
			wantLocale: "",
		},
		{
			name:       "empty config block returns empty locale",
			yaml:       "project_name: Test\nconfig:\n",
			wantLocale: "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tmp := t.TempDir()
			path := filepath.Join(tmp, "apitest.yaml")
			if err := os.WriteFile(path, []byte(tc.yaml), 0o644); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}
			cfg, err := ParseProjectConfig(path)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.Config.Locale != tc.wantLocale {
				t.Errorf("Config.Locale = %q, want %q", cfg.Config.Locale, tc.wantLocale)
			}
		})
	}
}
