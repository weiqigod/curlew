// Package datadriven provides data-driven testing support, enabling a single
// request definition to execute multiple times with different data values
// loaded from CSV or JSON data sources.
package datadriven

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Sentinel errors for data-driven testing failures.
var (
	ErrFileNotFound      = errors.New("data file not found")
	ErrEmptyDataFile     = errors.New("data file is empty")
	ErrUnsupportedFormat = errors.New("unsupported data format")
	ErrMalformedData     = errors.New("malformed data file")
)

// Config holds the data-driven configuration parsed from a collection YAML.
type Config struct {
	Source       string `yaml:"source"`
	Format       string `yaml:"format,omitempty"`         // csv, json, yaml; auto-detected from extension if empty
	Filter       string `yaml:"filter,omitempty"`         // filter expression (e.g., "{{age}} >= 18")
	Limit        *int   `yaml:"limit,omitempty"`          // max rows to execute
	StartRow     *int   `yaml:"start_row,omitempty"`      // first row index (0-based, inclusive)
	EndRow       *int   `yaml:"end_row,omitempty"`        // last row index (0-based, inclusive)
	FailFast     bool   `yaml:"fail_fast,omitempty"`      // stop on first iteration failure
	Parallel     bool   `yaml:"parallel,omitempty"`       // run iterations concurrently
	RateLimitRPS *int   `yaml:"rate_limit_rps,omitempty"` // max requests per second (nil = unlimited)
	StoreResults string `yaml:"store_results,omitempty"`  // all | summary | failed_only (default: all)
}

// DefaultMaxWorkers is the maximum number of concurrent workers for parallel data-driven execution.
const DefaultMaxWorkers = 20

// DefaultChunkSize is the number of rows processed per chunk for large datasets.
const DefaultChunkSize = 1000

// LargeDatasetThreshold is the row count above which a confirmation/warning is triggered.
const LargeDatasetThreshold = 10000

// StoreResults constants.
const (
	StoreAll        = "all"
	StoreSummary    = "summary"
	StoreFailedOnly = "failed_only"
)

// EffectiveStoreResults returns the store_results value, defaulting to "all".
func (c Config) EffectiveStoreResults() string {
	if c.StoreResults == "" {
		return StoreAll
	}
	return c.StoreResults
}

// Row represents a single iteration's data -- column name to string value.
type Row map[string]string

// DataSet holds all rows loaded from a data source.
type DataSet struct {
	Rows    []Row
	Columns []string // ordered column names (from CSV header or JSON keys)
}

// Load reads a data file relative to baseDir and returns the parsed dataset.
// The format is auto-detected from the file extension unless Config.Format is set.
func Load(cfg Config, baseDir string) (*DataSet, error) {
	path := cfg.Source
	if !filepath.IsAbs(path) {
		path = filepath.Join(baseDir, path)
	}

	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s (checked: %s)", ErrFileNotFound, cfg.Source, path)
		}
		return nil, fmt.Errorf("data file stat: %w", err)
	}

	format := cfg.Format
	if format == "" {
		format = detectFormat(cfg.Source)
	}

	switch strings.ToLower(format) {
	case "csv":
		return loadCSV(path)
	case "json":
		return loadJSON(path)
	case "yaml", "yml":
		return loadYAML(path)
	default:
		return nil, fmt.Errorf("%w: %q (supported: csv, json, yaml)", ErrUnsupportedFormat, format)
	}
}

// LoadWithControls loads a data source and applies row controls (filter, range, limit).
// This is the primary entry point for data-driven execution with controls.
func LoadWithControls(cfg Config, baseDir string) (*DataSet, error) {
	ds, err := Load(cfg, baseDir)
	if err != nil {
		return nil, err
	}
	return ApplyControls(ds, cfg)
}

// detectFormat returns the format string based on the file extension.
func detectFormat(source string) string {
	ext := strings.ToLower(filepath.Ext(source))
	switch ext {
	case ".csv":
		return "csv"
	case ".json":
		return "json"
	case ".yaml", ".yml":
		return "yaml"
	default:
		return ext // will be caught by the unsupported format check
	}
}
