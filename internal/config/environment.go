package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

var (
	// ErrEnvironmentNotFound indicates the named environment file does not exist.
	ErrEnvironmentNotFound = errors.New("environment not found")
	// ErrInvalidEnvironment indicates the environment file could not be parsed.
	ErrInvalidEnvironment = errors.New("invalid environment file")
)

// envFile is the internal representation of an environment YAML file.
type envFile struct {
	Variables map[string]any `yaml:"variables"`
	Config    ConfigBlock    `yaml:"config,omitempty"`
}

// EnvironmentConfig is the parsed environment file, including both its
// flattened variables and cross-cutting config values such as locale.
type EnvironmentConfig struct {
	Variables map[string]string
	Config    ConfigBlock
}

// ParseEnvironmentFile reads and parses an environment YAML file,
// returning a flat map of variable names to string values.
// Nested maps are flattened using underscore-separated keys.
func ParseEnvironmentFile(path string) (map[string]string, error) {
	env, err := ParseEnvironmentConfigFile(path)
	if err != nil {
		return nil, err
	}
	return env.Variables, nil
}

// ParseEnvironmentConfigFile reads variables and the optional config: block
// from an environment YAML file.
func ParseEnvironmentConfigFile(path string) (*EnvironmentConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading environment file %s: %w", path, err)
	}

	var ef envFile
	if err := yaml.Unmarshal(data, &ef); err != nil {
		return nil, fmt.Errorf("%w: %s: %w", ErrInvalidEnvironment, path, err)
	}

	result := make(map[string]string)
	flatten("", ef.Variables, result)
	return &EnvironmentConfig{Variables: result, Config: ef.Config}, nil
}

// flatten recursively flattens a nested map into underscore-separated keys.
func flatten(prefix string, m map[string]any, out map[string]string) {
	for k, v := range m {
		key := k
		if prefix != "" {
			key = prefix + "_" + k
		}
		switch val := v.(type) {
		case map[string]any:
			flatten(key, val, out)
		default:
			out[key] = fmt.Sprint(val)
		}
	}
}

// FindEnvironmentFile locates the file path for the named environment.
// Checks .yaml first, then .yml.
func FindEnvironmentFile(envName, baseDir string) (string, error) {
	envDir := filepath.Join(baseDir, "environments")

	yamlPath := filepath.Join(envDir, envName+".yaml")
	if _, err := os.Stat(yamlPath); err == nil {
		return yamlPath, nil
	}

	ymlPath := filepath.Join(envDir, envName+".yml")
	if _, err := os.Stat(ymlPath); err == nil {
		return ymlPath, nil
	}

	return "", fmt.Errorf("%w: %s", ErrEnvironmentNotFound, envName)
}

// ListAvailableEnvironments returns sorted names of environment files
// found in the environments/ subdirectory of baseDir.
func ListAvailableEnvironments(baseDir string) []string {
	envDir := filepath.Join(baseDir, "environments")
	entries, err := os.ReadDir(envDir)
	if err != nil {
		return []string{}
	}

	seen := make(map[string]struct{})
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		ext := filepath.Ext(name)
		if ext == ".yaml" || ext == ".yml" {
			seen[strings.TrimSuffix(name, ext)] = struct{}{}
		}
	}

	names := make([]string, 0, len(seen))
	for n := range seen {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// LoadEnvironment finds and parses the named environment file,
// searching in the environments/ subdirectory of baseDir.
func LoadEnvironment(envName, baseDir string) (map[string]string, error) {
	env, err := LoadEnvironmentConfig(envName, baseDir)
	if err != nil {
		return nil, err
	}
	return env.Variables, nil
}

// LoadEnvironmentConfig finds and parses a named environment, retaining its
// optional config: block in addition to the flattened variable map.
func LoadEnvironmentConfig(envName, baseDir string) (*EnvironmentConfig, error) {
	path, err := FindEnvironmentFile(envName, baseDir)
	if err != nil {
		if errors.Is(err, ErrEnvironmentNotFound) {
			available := ListAvailableEnvironments(baseDir)
			if len(available) > 0 {
				return nil, fmt.Errorf("%w: %q (available: %s)", ErrEnvironmentNotFound, envName, strings.Join(available, ", "))
			}
			return nil, fmt.Errorf("%w: %q (no environment files found in environments/)", ErrEnvironmentNotFound, envName)
		}
		return nil, err
	}
	return ParseEnvironmentConfigFile(path)
}
