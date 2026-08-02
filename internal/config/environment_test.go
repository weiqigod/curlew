package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseEnvironmentFile(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		wantVars map[string]string
		wantErr  bool
	}{
		{
			"valid flat variables",
			"variables:\n  base_url: http://localhost\n",
			map[string]string{"base_url": "http://localhost"},
			false,
		},
		{
			"nested variables flattened",
			"variables:\n  db:\n    host: localhost\n    port: \"5432\"\n",
			map[string]string{"db_host": "localhost", "db_port": "5432"},
			false,
		},
		{
			"empty variables section",
			"variables:\n",
			map[string]string{},
			false,
		},
		{
			"no variables key",
			"other: value\n",
			map[string]string{},
			false,
		},
		{
			"invalid YAML",
			"{{invalid",
			nil,
			true,
		},
		{
			"deeply nested",
			"variables:\n  a:\n    b:\n      c: deep\n",
			map[string]string{"a_b_c": "deep"},
			false,
		},
		{
			"mixed flat and nested",
			"variables:\n  simple: value\n  nested:\n    key: val\n",
			map[string]string{"simple": "value", "nested_key": "val"},
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			path := filepath.Join(tmpDir, "env.yaml")
			if err := os.WriteFile(path, []byte(tt.content), 0o644); err != nil {
				t.Fatal(err)
			}

			got, err := ParseEnvironmentFile(path)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tt.wantVars) {
				t.Fatalf("got %d vars, want %d: %v", len(got), len(tt.wantVars), got)
			}
			for k, want := range tt.wantVars {
				if got[k] != want {
					t.Errorf("vars[%q] = %q, want %q", k, got[k], want)
				}
			}
		})
	}
}

func TestParseEnvironmentConfigFile_ReturnsVariablesAndLocale(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dev.yaml")
	if err := os.WriteFile(path, []byte(`variables:
  base_url: http://localhost
config:
  locale: de-DE
`), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := ParseEnvironmentConfigFile(path)
	if err != nil {
		t.Fatalf("ParseEnvironmentConfigFile: %v", err)
	}
	if got.Variables["base_url"] != "http://localhost" {
		t.Errorf("base_url = %q, want http://localhost", got.Variables["base_url"])
	}
	if got.Config.Locale != "de-DE" {
		t.Errorf("Config.Locale = %q, want de-DE", got.Config.Locale)
	}
}

func TestFindEnvironmentFile(t *testing.T) {
	tests := []struct {
		name     string
		files    []string // files to create in environments/
		envName  string
		wantFile string
		wantErr  bool
	}{
		{"yaml extension found", []string{"dev.yaml"}, "dev", "dev.yaml", false},
		{"yml extension found", []string{"dev.yml"}, "dev", "dev.yml", false},
		{"yaml preferred over yml", []string{"dev.yaml", "dev.yml"}, "dev", "dev.yaml", false},
		{"not found", []string{"dev.yaml"}, "staging", "", true},
		{"no environments directory", nil, "dev", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			if tt.files != nil {
				envDir := filepath.Join(tmpDir, "environments")
				if err := os.MkdirAll(envDir, 0o755); err != nil {
					t.Fatal(err)
				}
				for _, f := range tt.files {
					if err := os.WriteFile(filepath.Join(envDir, f), []byte("variables:\n"), 0o644); err != nil {
						t.Fatal(err)
					}
				}
			}

			got, err := FindEnvironmentFile(tt.envName, tmpDir)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			wantPath := filepath.Join(tmpDir, "environments", tt.wantFile)
			if got != wantPath {
				t.Errorf("path = %q, want %q", got, wantPath)
			}
		})
	}
}

func TestListAvailableEnvironments(t *testing.T) {
	tests := []struct {
		name  string
		files []string
		want  []string
	}{
		{"multiple environments", []string{"dev.yaml", "staging.yml", "prod.yaml"}, []string{"dev", "prod", "staging"}},
		{"deduplicates yaml and yml", []string{"dev.yaml", "dev.yml"}, []string{"dev"}},
		{"empty directory", []string{}, []string{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			envDir := filepath.Join(tmpDir, "environments")
			if err := os.MkdirAll(envDir, 0o755); err != nil {
				t.Fatal(err)
			}
			for _, f := range tt.files {
				if err := os.WriteFile(filepath.Join(envDir, f), []byte("variables:\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}

			got := ListAvailableEnvironments(tmpDir)
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i, want := range tt.want {
				if got[i] != want {
					t.Errorf("got[%d] = %q, want %q", i, got[i], want)
				}
			}
		})
	}

	t.Run("no environments directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		got := ListAvailableEnvironments(tmpDir)
		if len(got) != 0 {
			t.Errorf("got %v, want empty", got)
		}
	})
}

func TestLoadEnvironment(t *testing.T) {
	t.Run("loads and parses environment", func(t *testing.T) {
		tmpDir := t.TempDir()
		envDir := filepath.Join(tmpDir, "environments")
		if err := os.MkdirAll(envDir, 0o755); err != nil {
			t.Fatal(err)
		}
		content := "variables:\n  base_url: http://localhost\n"
		if err := os.WriteFile(filepath.Join(envDir, "dev.yaml"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}

		got, err := LoadEnvironment("dev", tmpDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got["base_url"] != "http://localhost" {
			t.Errorf("base_url = %q, want %q", got["base_url"], "http://localhost")
		}
	})

	t.Run("environment not found lists available", func(t *testing.T) {
		tmpDir := t.TempDir()
		envDir := filepath.Join(tmpDir, "environments")
		if err := os.MkdirAll(envDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(envDir, "dev.yaml"), []byte("variables:\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		_, err := LoadEnvironment("missing", tmpDir)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		errMsg := err.Error()
		if !strings.Contains(errMsg, "missing") {
			t.Errorf("error %q does not mention env name", errMsg)
		}
		if !strings.Contains(errMsg, "dev") {
			t.Errorf("error %q does not list available environment 'dev'", errMsg)
		}
	})

	t.Run("invalid yaml reports file path", func(t *testing.T) {
		tmpDir := t.TempDir()
		envDir := filepath.Join(tmpDir, "environments")
		if err := os.MkdirAll(envDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(envDir, "bad.yaml"), []byte("{{invalid"), 0o644); err != nil {
			t.Fatal(err)
		}

		_, err := LoadEnvironment("bad", tmpDir)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		errMsg := err.Error()
		if !strings.Contains(errMsg, "bad.yaml") {
			t.Errorf("error %q does not contain file path", errMsg)
		}
	})
}
