package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/weiqigod/curlew/internal/auth"
	"github.com/weiqigod/curlew/internal/output"
	"github.com/weiqigod/curlew/internal/retry"
	"github.com/weiqigod/curlew/internal/vault"
	"gopkg.in/yaml.v3"
)

// ErrInvalidProjectConfig is returned when curlew.yaml cannot be parsed.
var ErrInvalidProjectConfig = errors.New("invalid project config")

// GraphQLErrorHandling holds configurable GraphQL error handling defaults.
// Mirrors spec: defaults.graphql.error_handling.partial_success: fail|warn|ignore
type GraphQLErrorHandling struct {
	PartialSuccess string `yaml:"partial_success,omitempty"`
}

// GraphQLDefaults holds project-wide GraphQL defaults.
type GraphQLDefaults struct {
	ErrorHandling GraphQLErrorHandling `yaml:"error_handling,omitempty"`
}

// DefaultsConfig holds project-wide default settings.
type DefaultsConfig struct {
	Retry   *retry.FullConfig `yaml:"retry,omitempty"`
	GraphQL *GraphQLDefaults  `yaml:"graphql,omitempty"`
}

// ConfigBlock is the shared config: block in curlew.yaml / collection files.
// It holds cross-cutting settings (locale, and future knobs). The block is
// optional and omitempty — absent means all fields are zero/default.
type ConfigBlock struct {
	Locale string `yaml:"locale,omitempty"`
}

// ProjectConfig holds the parsed contents of an curlew.yaml file.
type ProjectConfig struct {
	ProjectName  string
	Variables    map[string]string
	Secrets      *vault.SecretsConfig
	AuthProfiles []auth.Profile // parsed from auth_profiles: block
	Defaults     DefaultsConfig
	Output       *output.Config // M8-003: project-level output default
	UI           *UIConfig      // curlew ui server settings (UI_SPECIFICATION.md §11)
	Config       ConfigBlock    // M20-001: config: block (locale, etc.)
}

// UIConfig is the optional top-level ui: block in curlew.yaml.
// Precedence per field: CLI flag > ui: block > built-in default.
type UIConfig struct {
	Port        int              `yaml:"port,omitempty"`         // 1024–65535; 0 reserved for the --port flag
	Host        string           `yaml:"host,omitempty"`         // loopback literals only
	OpenBrowser *bool            `yaml:"open_browser,omitempty"` // default true
	Editor      string           `yaml:"editor,omitempty"`       // command template for /open
	History     *UIHistoryConfig `yaml:"history,omitempty"`
}

// UIHistoryConfig configures the curlew ui run-history store.
type UIHistoryConfig struct {
	Enabled *bool `yaml:"enabled,omitempty"`  // default true; false disables persistence even on Solo+
	MaxRuns int   `yaml:"max_runs,omitempty"` // 0 = default (50); cap 500
}

type projectFile struct {
	ProjectName  string         `yaml:"project_name"`
	Variables    map[string]any `yaml:"variables"`
	Secrets      yaml.Node      `yaml:"secrets,omitempty"`
	AuthProfiles yaml.Node      `yaml:"auth_profiles,omitempty"`
	Defaults     yaml.Node      `yaml:"defaults,omitempty"`
	Output       yaml.Node      `yaml:"output,omitempty"`
	UI           yaml.Node      `yaml:"ui,omitempty"`
	Config       yaml.Node      `yaml:"config,omitempty"`
}

type authProfileEntry struct {
	Type             string `yaml:"type"`
	Collection       string `yaml:"collection"`
	Extract          string `yaml:"extract,omitempty"`
	CacheTTL         int    `yaml:"cache_ttl,omitempty"`
	RefreshOnFailure bool   `yaml:"refresh_on_failure,omitempty"`
}

// ParseProjectConfig reads and parses an curlew.yaml file at the given path.
// Returns ErrInvalidProjectConfig for parse failures.
func ParseProjectConfig(path string) (*ProjectConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read project config: %w", err)
	}
	var pf projectFile
	if err := yaml.Unmarshal(data, &pf); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidProjectConfig, err)
	}
	vars := make(map[string]string)
	flatten("", pf.Variables, vars)

	cfg := &ProjectConfig{
		ProjectName: pf.ProjectName,
		Variables:   vars,
	}

	// Parse secrets block if present (Kind 0 means the node was not populated).
	if pf.Secrets.Kind != 0 {
		secrets, secretsErr := vault.ParseSecretsYAML(&pf.Secrets)
		if secretsErr != nil {
			return nil, fmt.Errorf("parsing secrets config: %w", secretsErr)
		}
		cfg.Secrets = secrets
	}

	// Parse auth_profiles block if present.
	if pf.AuthProfiles.Kind != 0 {
		var raw map[string]authProfileEntry
		if err := pf.AuthProfiles.Decode(&raw); err != nil {
			return nil, fmt.Errorf("%w: auth_profiles: %w", ErrInvalidProjectConfig, err)
		}
		for name, entry := range raw {
			if entry.Type != string(auth.ProfileDynamic) {
				return nil, fmt.Errorf("%w: auth_profiles: unsupported type %q for profile %q",
					ErrInvalidProjectConfig, entry.Type, name)
			}
			if entry.Collection == "" {
				return nil, fmt.Errorf("%w: auth_profiles: profile %q missing collection path",
					ErrInvalidProjectConfig, name)
			}
			cfg.AuthProfiles = append(cfg.AuthProfiles, auth.Profile{
				Name:             name,
				Type:             auth.ProfileDynamic,
				Collection:       entry.Collection,
				Extract:          entry.Extract,
				CacheTTL:         entry.CacheTTL,
				RefreshOnFailure: entry.RefreshOnFailure,
			})
		}
		sort.Slice(cfg.AuthProfiles, func(i, j int) bool {
			return cfg.AuthProfiles[i].Name < cfg.AuthProfiles[j].Name
		})
	}

	// Parse defaults block if present.
	if pf.Defaults.Kind != 0 {
		var defaults DefaultsConfig
		if err := pf.Defaults.Decode(&defaults); err != nil {
			return nil, fmt.Errorf("%w: defaults: %w", ErrInvalidProjectConfig, err)
		}
		if defaults.GraphQL != nil {
			ps := defaults.GraphQL.ErrorHandling.PartialSuccess
			if ps != "" && ps != "fail" && ps != "warn" && ps != "ignore" {
				return nil, fmt.Errorf("%w: defaults.graphql.error_handling.partial_success: invalid value %q (allowed: fail, warn, ignore)",
					ErrInvalidProjectConfig, ps)
			}
		}
		cfg.Defaults = defaults
	}

	// Parse output block if present (M8-003: project-level output default).
	if pf.Output.Kind != 0 {
		var oc output.Config
		if err := pf.Output.Decode(&oc); err != nil {
			return nil, fmt.Errorf("%w: output: %w", ErrInvalidProjectConfig, err)
		}
		if verr := oc.Validate(); verr != nil {
			return nil, fmt.Errorf("%w: output: %w", ErrInvalidProjectConfig, verr)
		}
		cfg.Output = &oc
	}

	// Parse ui block if present (curlew ui server settings).
	if pf.UI.Kind != 0 {
		var ui UIConfig
		if err := pf.UI.Decode(&ui); err != nil {
			return nil, fmt.Errorf("%w: ui: %w", ErrInvalidProjectConfig, err)
		}
		if ui.Port != 0 && (ui.Port < 1024 || ui.Port > 65535) {
			return nil, fmt.Errorf("%w: ui.port: must be 1024-65535, got %d", ErrInvalidProjectConfig, ui.Port)
		}
		switch ui.Host {
		case "", "127.0.0.1", "localhost", "::1":
		default:
			return nil, fmt.Errorf("%w: ui.host: only loopback is allowed (127.0.0.1 | localhost | ::1), got %q", ErrInvalidProjectConfig, ui.Host)
		}
		if ui.History != nil && ui.History.MaxRuns > 500 {
			return nil, fmt.Errorf("%w: ui.history.max_runs: cap is 500, got %d", ErrInvalidProjectConfig, ui.History.MaxRuns)
		}
		cfg.UI = &ui
	}

	// Parse config block if present (M20-001: locale and future knobs).
	if pf.Config.Kind != 0 {
		var cb ConfigBlock
		if err := pf.Config.Decode(&cb); err != nil {
			return nil, fmt.Errorf("%w: config: %w", ErrInvalidProjectConfig, err)
		}
		cfg.Config = cb
	}

	return cfg, nil
}

// FindProjectRoot walks up from startDir looking for curlew.yaml (or curlew.yml).
// Returns the directory containing the project config file and true,
// or ("", false) if no project config is found up to the filesystem root.
func FindProjectRoot(startDir string) (string, bool) {
	dir := startDir
	for {
		for _, name := range []string{"curlew.yaml", "curlew.yml"} {
			if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
				return dir, true
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir { // reached filesystem root
			return "", false
		}
		dir = parent
	}
}

// LoadProjectConfig finds the project root by walking up from startDir,
// then parses curlew.yaml (or curlew.yml). Returns an empty ProjectConfig and
// empty root string if no project config is found (not an error). Returns error
// if the file exists but cannot be parsed.
func LoadProjectConfig(startDir string) (*ProjectConfig, string, error) {
	root, found := FindProjectRoot(startDir)
	if !found {
		return &ProjectConfig{Variables: map[string]string{}}, "", nil
	}
	name := "curlew.yaml"
	if _, err := os.Stat(filepath.Join(root, name)); errors.Is(err, os.ErrNotExist) {
		name = "curlew.yml"
	}
	cfg, err := ParseProjectConfig(filepath.Join(root, name))
	if err != nil {
		return nil, "", err
	}
	return cfg, root, nil
}
