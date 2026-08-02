package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testValidTeamTemplate = `team_secrets:
  vault_configs:
    production:
      provider: aws-secrets-manager
      region: eu-west-1
      keys:
        api_key: prod/api-key
        db_password: prod/db-credentials#password
    staging:
      provider: azure-key-vault
      vault_name: staging-vault
      keys:
        api_key: staging-api-key
        db_password: staging-db-password
`

const testInvalidTeamTemplate = `team_secrets:
  vault_configs:
    production:
      provider: foo
      region: eu-west-1
      keys:
        api_key: prod/api-key
`

func TestValidateCmd_TeamTemplate(t *testing.T) {
	t.Run("valid_template_prints_summary_exit_0", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "shared-vault-template.yaml")
		if err := os.WriteFile(path, []byte(testValidTeamTemplate), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
		stdout, _, code := captureRun(t, "validate", path)
		if code != 0 {
			t.Errorf("exit code = %d, want 0; stdout=%q", code, stdout)
		}
		if !strings.Contains(stdout, "shared vault template valid") {
			t.Errorf("stdout does not contain 'shared vault template valid': %q", stdout)
		}
		if !strings.Contains(stdout, "2 environments, 4 secrets") {
			t.Errorf("stdout does not contain '2 environments, 4 secrets': %q", stdout)
		}
	})

	t.Run("invalid_template_prints_key_path_exit_2", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "shared-vault-template.invalid.yaml")
		if err := os.WriteFile(path, []byte(testInvalidTeamTemplate), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
		stdout, stderr, code := captureRun(t, "validate", path)
		if code != 2 {
			t.Errorf("exit code = %d, want 2; stdout=%q stderr=%q", code, stdout, stderr)
		}
		combined := stdout + stderr
		if !strings.Contains(combined, "team_secrets.vault_configs.production.provider") {
			t.Errorf("output does not contain key path; combined output: %q", combined)
		}
		if !strings.Contains(combined, "unknown provider 'foo'") {
			t.Errorf("output does not contain 'unknown provider 'foo''; combined output: %q", combined)
		}
	})

	t.Run("help_mentions_shared_vault_template", func(t *testing.T) {
		stdout, _, _ := captureRun(t, "--help")
		if !strings.Contains(stdout, "shared vault configuration templates") {
			t.Errorf("help output does not mention 'shared vault configuration templates': %q", stdout)
		}
	})
}

// TestValidateCmd_DuplicateRequestNames verifies that curlew validate surfaces
// duplicate main request names as exit code 3 (M8-004 DoD item #9).
// The rejection is inherited from parser.ParseFile and requires no validate-
// specific code; this test confirms the end-to-end plumbing works correctly.
func TestValidateCmd_DuplicateRequestNames(t *testing.T) {
	t.Run("duplicate_request_names_exit_3", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "dupes.yaml")
		body := `name: Dupes
requests:
  - name: Get user
    request: {method: GET, url: "https://example.com/a"}
  - name: Get user
    request: {method: GET, url: "https://example.com/b"}
`
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
		stdout, stderr, code := captureRun(t, "validate", path)
		if code != 3 {
			t.Errorf("exit code = %d, want 3; stdout=%q stderr=%q", code, stdout, stderr)
		}
		combined := stdout + stderr
		if !strings.Contains(combined, "duplicate request name") {
			t.Errorf("output does not contain 'duplicate request name'; combined output: %q", combined)
		}
		if !strings.Contains(combined, `"Get user"`) {
			t.Errorf("output does not name the duplicate request; combined output: %q", combined)
		}
	})
}
