package parser

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// parseExternalFile reads and parses a standalone request YAML file.
func parseExternalFile(path string) (*externalRequest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("external request file %s: %w", path, ErrExternalFileNotFound)
		}
		return nil, fmt.Errorf("reading external request file %s: %w", path, err)
	}

	var ext externalRequest
	if err := yaml.Unmarshal(data, &ext); err != nil {
		return nil, fmt.Errorf("parsing external request file %s: %w: %w", path, ErrInvalidYAML, err)
	}

	if ext.Request.URL == "" {
		return nil, fmt.Errorf("external request %q in %s: %w", ext.Name, path, ErrMissingRequiredField)
	}

	return &ext, nil
}

// resolveExternalReferences replaces path:-based items with fully resolved RequestItems.
// Returns the resolved items and the absolute paths of all external files referenced.
func resolveExternalReferences(collectionPath string, items []RequestItem, visited map[string]bool) ([]RequestItem, []string, error) {
	collectionDir := filepath.Dir(collectionPath)
	resolved := make([]RequestItem, 0, len(items))
	var extFiles []string

	for _, item := range items {
		if item.Path == "" {
			resolved = append(resolved, item)
			continue
		}

		// Validate mutual exclusivity: path and request.url cannot both be set
		if item.Request.URL != "" {
			return nil, nil, fmt.Errorf("request %q has both path and request.url: %w", item.Name, ErrMutuallyExclusive)
		}

		extPath := item.Path
		if !filepath.IsAbs(extPath) {
			extPath = filepath.Join(collectionDir, extPath)
		}

		absPath, err := filepath.Abs(extPath)
		if err != nil {
			return nil, nil, fmt.Errorf("resolving path %q: %w", item.Path, err)
		}

		if visited[absPath] {
			return nil, nil, fmt.Errorf("circular file reference: %s references %s: %w", collectionPath, item.Path, ErrCircularFileReference)
		}

		ext, err := parseExternalFile(extPath)
		if err != nil {
			return nil, nil, err
		}

		extFiles = append(extFiles, absPath)

		ri := RequestItem{
			Name:             ext.Name,
			Request:          ext.Request,
			Assertions:       ext.Assertions,
			Extract:          ext.Extract,
			ExtractSensitive: ext.ExtractSensitive,
			SourceFile:       absPath,
			SourceLine:       1,
		}

		// Reference site variable overrides
		if len(item.Variables.Values) > 0 {
			ri.Variables = item.Variables
		}

		// Reference site name override
		if item.Name != "" {
			ri.Name = item.Name
		}

		// Reference site auth override
		if item.Auth != "" {
			ri.Auth = item.Auth
		}

		resolved = append(resolved, ri)
	}

	return resolved, extFiles, nil
}
