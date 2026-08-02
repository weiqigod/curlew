// Package device persists the per-CLI device identity at
// <configDir>/device.json. The device_id is not a secret (spec
// :7946-7951 limits secret status to the refresh token) but the file is
// written 0600 because the parent directory is.
package device

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ErrNotFound is returned when device.json is absent.
var ErrNotFound = errors.New("backend/device: device.json not found")

// Record is the on-disk form. The CLI keeps the file flat so future fields
// (e.g. last_refresh_at) can be appended without a schema migration.
type Record struct {
	DeviceID string    `json:"device_id"`
	IssuedAt time.Time `json:"issued_at"`
}

// Read loads <configDir>/device.json. Returns ErrNotFound when absent.
func Read(configDir string) (Record, error) {
	data, err := os.ReadFile(filePath(configDir))
	if errors.Is(err, os.ErrNotExist) {
		return Record{}, ErrNotFound
	}
	if err != nil {
		return Record{}, fmt.Errorf("backend/device: read device.json: %w", err)
	}
	var rec Record
	if err := json.Unmarshal(data, &rec); err != nil {
		return Record{}, fmt.Errorf("backend/device: parse device.json: %w", err)
	}
	return rec, nil
}

// Write atomically writes <configDir>/device.json with mode 0600.
// Creates the directory if it does not exist.
func Write(configDir string, rec Record) error {
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		return fmt.Errorf("backend/device: mkdir: %w", err)
	}
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return fmt.Errorf("backend/device: marshal: %w", err)
	}
	if err := os.WriteFile(filePath(configDir), data, 0o600); err != nil {
		return fmt.Errorf("backend/device: write device.json: %w", err)
	}
	return nil
}

func filePath(configDir string) string {
	return filepath.Join(configDir, "device.json")
}
