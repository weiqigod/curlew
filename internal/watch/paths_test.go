package watch

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPaths_All(t *testing.T) {
	tests := []struct {
		name  string
		paths Paths
		want  []string
	}{
		{
			name: "excludes empty strings",
			paths: Paths{
				Collection: "/a/col.yaml",
				EnvFile:    "",
				DotEnv:     "",
			},
			want: []string{"/a/col.yaml"},
		},
		{
			name: "deduplicates same path",
			paths: Paths{
				Collection:    "/a/col.yaml",
				ExternalFiles: []string{"/a/col.yaml"},
			},
			want: []string{"/a/col.yaml"},
		},
		{
			name: "sorted output",
			paths: Paths{
				Collection:    "/z/col.yaml",
				ExternalFiles: []string{"/a/ext.yaml"},
				DotEnv:        "/m/.env",
			},
			want: []string{"/a/ext.yaml", "/m/.env", "/z/col.yaml"},
		},
		{
			name: "all fields populated",
			paths: Paths{
				Collection:    "/proj/col.yaml",
				ExternalFiles: []string{"/proj/req/a.yaml", "/proj/req/b.yaml"},
				EnvFile:       "/proj/env/dev.yaml",
				DotEnv:        "/proj/.env",
				ProjectConfig: "/proj/curlew.yaml",
			},
			want: []string{
				"/proj/.env",
				"/proj/col.yaml",
				"/proj/curlew.yaml",
				"/proj/env/dev.yaml",
				"/proj/req/a.yaml",
				"/proj/req/b.yaml",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.paths.All()
			if len(got) != len(tt.want) {
				t.Fatalf("All() = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("All()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestPaths_Dirs(t *testing.T) {
	tests := []struct {
		name  string
		paths Paths
		want  []string
	}{
		{
			name: "unique directories",
			paths: Paths{
				Collection:    "/proj/col.yaml",
				ExternalFiles: []string{"/proj/req/a.yaml"},
				EnvFile:       "/proj/env/dev.yaml",
			},
			want: []string{"/proj", "/proj/env", "/proj/req"},
		},
		{
			name: "single directory",
			paths: Paths{
				Collection:    "/proj/col.yaml",
				ExternalFiles: []string{"/proj/ext.yaml"},
			},
			want: []string{"/proj"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.paths.Dirs()
			if len(got) != len(tt.want) {
				t.Fatalf("Dirs() = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("Dirs()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestCollectPaths(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(t *testing.T) string // returns tmpDir
		envName   string
		wantErr   bool
		checkFunc func(t *testing.T, wp *Paths, tmpDir string)
	}{
		{
			name: "collection only - no extras",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				writeFile(t, filepath.Join(dir, "col.yaml"), `name: Test
requests:
  - name: R1
    request:
      method: GET
      url: https://example.com`)
				return dir
			},
			checkFunc: func(t *testing.T, wp *Paths, tmpDir string) {
				t.Helper()
				if wp.Collection != filepath.Join(tmpDir, "col.yaml") {
					t.Errorf("Collection = %q, want %q", wp.Collection, filepath.Join(tmpDir, "col.yaml"))
				}
				if len(wp.ExternalFiles) != 0 {
					t.Errorf("ExternalFiles = %v, want empty", wp.ExternalFiles)
				}
				if wp.EnvFile != "" {
					t.Errorf("EnvFile = %q, want empty", wp.EnvFile)
				}
				if wp.DotEnv != "" {
					t.Errorf("DotEnv = %q, want empty", wp.DotEnv)
				}
			},
		},
		{
			name: "with environment file",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				writeFile(t, filepath.Join(dir, "col.yaml"), `name: Test
requests:
  - name: R1
    request:
      method: GET
      url: https://example.com`)
				mkdirAll(t, filepath.Join(dir, "environments"))
				writeFile(t, filepath.Join(dir, "environments", "dev.yaml"), `variables:
  base_url: http://localhost`)
				return dir
			},
			envName: "dev",
			checkFunc: func(t *testing.T, wp *Paths, tmpDir string) {
				t.Helper()
				want := filepath.Join(tmpDir, "environments", "dev.yaml")
				if wp.EnvFile != want {
					t.Errorf("EnvFile = %q, want %q", wp.EnvFile, want)
				}
			},
		},
		{
			name: "with dotenv file",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				writeFile(t, filepath.Join(dir, "col.yaml"), `name: Test
requests:
  - name: R1
    request:
      method: GET
      url: https://example.com`)
				writeFile(t, filepath.Join(dir, ".env"), "API_KEY=secret")
				return dir
			},
			checkFunc: func(t *testing.T, wp *Paths, tmpDir string) {
				t.Helper()
				want := filepath.Join(tmpDir, ".env")
				if wp.DotEnv != want {
					t.Errorf("DotEnv = %q, want %q", wp.DotEnv, want)
				}
			},
		},
		{
			name: "with project config",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				writeFile(t, filepath.Join(dir, "col.yaml"), `name: Test
requests:
  - name: R1
    request:
      method: GET
      url: https://example.com`)
				writeFile(t, filepath.Join(dir, "curlew.yaml"), `project_name: myproj`)
				return dir
			},
			checkFunc: func(t *testing.T, wp *Paths, tmpDir string) {
				t.Helper()
				want := filepath.Join(tmpDir, "curlew.yaml")
				if wp.ProjectConfig != want {
					t.Errorf("ProjectConfig = %q, want %q", wp.ProjectConfig, want)
				}
			},
		},
		{
			name: "with external request refs",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				mkdirAll(t, filepath.Join(dir, "requests"))
				writeFile(t, filepath.Join(dir, "requests", "get.yaml"), `name: Get
request:
  method: GET
  url: https://example.com`)
				writeFile(t, filepath.Join(dir, "col.yaml"), `name: Test
requests:
  - path: requests/get.yaml`)
				return dir
			},
			checkFunc: func(t *testing.T, wp *Paths, tmpDir string) {
				t.Helper()
				if len(wp.ExternalFiles) != 1 {
					t.Fatalf("ExternalFiles count = %d, want 1", len(wp.ExternalFiles))
				}
				want := filepath.Join(tmpDir, "requests", "get.yaml")
				if wp.ExternalFiles[0] != want {
					t.Errorf("ExternalFiles[0] = %q, want %q", wp.ExternalFiles[0], want)
				}
			},
		},
		{
			name: "all sources present",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				mkdirAll(t, filepath.Join(dir, "requests"))
				mkdirAll(t, filepath.Join(dir, "environments"))
				writeFile(t, filepath.Join(dir, "requests", "get.yaml"), `name: Get
request:
  method: GET
  url: https://example.com`)
				writeFile(t, filepath.Join(dir, "col.yaml"), `name: Test
requests:
  - path: requests/get.yaml`)
				writeFile(t, filepath.Join(dir, "environments", "dev.yaml"), `variables:
  base_url: http://localhost`)
				writeFile(t, filepath.Join(dir, ".env"), "KEY=val")
				writeFile(t, filepath.Join(dir, "curlew.yaml"), `project_name: proj`)
				return dir
			},
			envName: "dev",
			checkFunc: func(t *testing.T, wp *Paths, tmpDir string) {
				t.Helper()
				all := wp.All()
				if len(all) < 4 {
					t.Errorf("All() has %d entries, want >= 4", len(all))
				}
			},
		},
		{
			name: "missing collection file - error",
			setup: func(t *testing.T) string {
				t.Helper()
				return t.TempDir()
			},
			wantErr: true,
		},
		{
			name: "env file specified but not found - error",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				writeFile(t, filepath.Join(dir, "col.yaml"), `name: Test
requests:
  - name: R1
    request:
      method: GET
      url: https://example.com`)
				return dir
			},
			envName: "nonexistent",
			wantErr: true,
		},
		{
			name: "dotenv missing is not an error",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				writeFile(t, filepath.Join(dir, "col.yaml"), `name: Test
requests:
  - name: R1
    request:
      method: GET
      url: https://example.com`)
				return dir
			},
			checkFunc: func(t *testing.T, wp *Paths, tmpDir string) {
				t.Helper()
				if wp.DotEnv != "" {
					t.Errorf("DotEnv = %q, want empty", wp.DotEnv)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := tt.setup(t)
			colPath := filepath.Join(tmpDir, "col.yaml")
			wp, err := CollectPaths(colPath, tt.envName)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.checkFunc != nil {
				tt.checkFunc(t, wp, tmpDir)
			}
		})
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writeFile(%s): %v", path, err)
	}
}

func mkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdirAll(%s): %v", path, err)
	}
}
