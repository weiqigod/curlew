package telemetry

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// ErrNotEnabled is returned by Status when no install_id has been generated.
var ErrNotEnabled = errors.New("telemetry: not enabled")

const (
	installIDBaseName     = "install_id"
	telemetryJSONBaseName = "telemetry.json"
	currentSchemaVersion  = 1
	eventsBaseName        = "telemetry.ndjson"
	maxRecentEmissions    = 10
)

// Emission records one event-append attempt for use by `telemetry export`.
type Emission struct {
	At        string `json:"at"` // RFC3339
	EventType string `json:"event_type"`
	Status    string `json:"status"` // "ok" | "error" | "disabled"
	Error     string `json:"error,omitempty"`
}

// State is the on-disk shape of telemetry.json.
type State struct {
	SchemaVersion   int        `json:"schema_version"`
	Enabled         bool       `json:"enabled"`
	File            string     `json:"file,omitempty"`
	RecentEmissions []Emission `json:"recent_emissions,omitempty"`
}

// Store persists install_id and telemetry.json under configDir.
type Store struct {
	mu        sync.Mutex
	configDir string
}

// NewStore returns a Store rooted at configDir.
// It calls os.MkdirAll so the directory hierarchy is created if needed.
func NewStore(configDir string) (*Store, error) {
	if configDir == "" {
		return nil, errors.New("telemetry: configDir must be non-empty")
	}
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		return nil, fmt.Errorf("telemetry: create config dir: %w", err)
	}
	return &Store{configDir: configDir}, nil
}

// installIDPath returns the full path to the install_id file.
func (s *Store) installIDPath() string {
	return filepath.Join(s.configDir, installIDBaseName)
}

// telemetryJSONPath returns the full path to telemetry.json.
func (s *Store) telemetryJSONPath() string {
	return filepath.Join(s.configDir, telemetryJSONBaseName)
}

// readInstallID reads the install_id file (caller must hold mu).
func (s *Store) readInstallID() (string, error) {
	b, err := os.ReadFile(s.installIDPath())
	if os.IsNotExist(err) {
		return "", ErrNotEnabled
	}
	if err != nil {
		return "", fmt.Errorf("telemetry: read install_id: %w", err)
	}
	return string(b), nil
}

// writeInstallID atomically writes id to the install_id file (caller holds mu).
func (s *Store) writeInstallID(id string) error {
	return atomicWrite(s.installIDPath(), []byte(id))
}

// Enable generates an install_id if absent and persists telemetry.json with
// enabled=true. Returns the (possibly newly minted) install_id.
func (s *Store) Enable(file string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Re-use existing install_id if present.
	id, err := s.readInstallID()
	if errors.Is(err, ErrNotEnabled) {
		id, err = newUUIDv4()
		if err != nil {
			return "", fmt.Errorf("telemetry: generate install_id: %w", err)
		}
		if err := s.writeInstallID(id); err != nil {
			return "", err
		}
	} else if err != nil {
		return "", err
	}

	// Load existing state so we preserve recent_emissions.
	state, _ := s.readState()
	state.SchemaVersion = currentSchemaVersion
	state.Enabled = true
	if file != "" {
		state.File = file
	}
	if err := s.writeState(state); err != nil {
		return "", err
	}
	return id, nil
}

// Disable flips enabled=false but retains install_id and telemetry.json.
// If telemetry.json is absent (telemetry was never enabled), Disable is a
// no-op and returns nil — the state is already disabled.
func (s *Store) Disable() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.readState()
	if errors.Is(err, errStateMissing) {
		// Already disabled (never enabled); treat as no-op.
		return nil
	}
	if err != nil {
		return fmt.Errorf("telemetry: disable: %w", err)
	}
	state.Enabled = false
	return s.writeState(state)
}

// Status reports the current enabled flag, install_id, and resolved events file.
// Returns ErrNotEnabled when no install_id file exists.
func (s *Store) Status() (enabled bool, installID, file string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id, err := s.readInstallID()
	if err != nil {
		return false, "", "", err
	}
	state, _ := s.readState()
	return state.Enabled, id, s.resolvedFile(state), nil
}

// ResetID regenerates install_id (overwrites the file with a new UUID v4) and
// returns the new id. Requires that telemetry.json exists; clears recent_emissions.
func (s *Store) ResetID() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// install_id must already exist (telemetry must have been enabled at least once).
	if _, err := s.readInstallID(); err != nil {
		return "", fmt.Errorf("telemetry: reset-id: %w", err)
	}
	id, err := newUUIDv4()
	if err != nil {
		return "", fmt.Errorf("telemetry: generate new install_id: %w", err)
	}
	if err := s.writeInstallID(id); err != nil {
		return "", err
	}
	// Clear emissions in telemetry.json.
	state, _ := s.readState()
	state.RecentEmissions = nil
	if err := s.writeState(state); err != nil {
		return "", err
	}
	return id, nil
}

// DeleteLocal removes install_id and telemetry.json. Missing files are not an
// error. Returns the install_id that existed before removal (or "").
func (s *Store) DeleteLocal() (priorInstallID string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	priorID, _ := s.readInstallID() // ignore ErrNotEnabled

	if err := removeIfExists(s.installIDPath()); err != nil {
		return "", err
	}
	if err := removeIfExists(s.telemetryJSONPath()); err != nil {
		return "", err
	}
	return priorID, nil
}

// RecordEmission appends an Emission to telemetry.json, capping at
// maxRecentEmissions (newest first). No-op when telemetry.json is absent.
func (s *Store) RecordEmission(em Emission) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	state, err := s.readState()
	if os.IsNotExist(err) || errors.Is(err, errStateMissing) {
		return nil // no-op when not enabled
	}
	if err != nil {
		return fmt.Errorf("telemetry: record emission: %w", err)
	}

	// Prepend new emission; cap at maxRecentEmissions.
	updated := make([]Emission, 0, maxRecentEmissions)
	updated = append(updated, em)
	for _, e := range state.RecentEmissions {
		if len(updated) >= maxRecentEmissions {
			break
		}
		updated = append(updated, e)
	}
	state.RecentEmissions = updated
	return s.writeState(state)
}

// RecentEmissions returns the most recent emissions, newest first.
func (s *Store) RecentEmissions() ([]Emission, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.readState()
	if errors.Is(err, errStateMissing) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("telemetry: read emissions: %w", err)
	}
	return state.RecentEmissions, nil
}

// resolvedFile honours CURLEW_TELEMETRY_FILE, then State.File, then
// <configDir>/telemetry.ndjson.
func (s *Store) resolvedFile(state State) string {
	if f := os.Getenv("CURLEW_TELEMETRY_FILE"); f != "" {
		return f
	}
	if state.File != "" {
		return state.File
	}
	return filepath.Join(s.configDir, eventsBaseName)
}

// errStateMissing is a sentinel returned by readState when telemetry.json is absent.
var errStateMissing = errors.New("telemetry: state file absent")

// readState reads telemetry.json. Returns errStateMissing if the file does not exist.
func (s *Store) readState() (State, error) {
	b, err := os.ReadFile(s.telemetryJSONPath())
	if os.IsNotExist(err) {
		return State{SchemaVersion: currentSchemaVersion}, errStateMissing
	}
	if err != nil {
		return State{}, fmt.Errorf("telemetry: read telemetry.json: %w", err)
	}
	var st State
	if err := json.Unmarshal(b, &st); err != nil {
		return State{}, fmt.Errorf("telemetry: parse telemetry.json: %w", err)
	}
	return st, nil
}

// writeState atomically writes state to telemetry.json with mode 0600.
func (s *Store) writeState(state State) error {
	state.SchemaVersion = currentSchemaVersion
	b, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("telemetry: marshal state: %w", err)
	}
	return atomicWrite(s.telemetryJSONPath(), b)
}

// atomicWrite writes data to path atomically: write to <path>.tmp, chmod 0600,
// then rename to path.
func atomicWrite(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("telemetry: write temp file %s: %w", tmp, err)
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("telemetry: chmod temp file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("telemetry: rename temp file: %w", err)
	}
	return nil
}

// removeIfExists removes path, ignoring not-found errors.
func removeIfExists(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("telemetry: remove %s: %w", path, err)
	}
	return nil
}

// ResolvedFile returns the effective events file for external callers (cmd/curlew).
// Precedence: CURLEW_TELEMETRY_FILE > telemetry.json.file > <configDir>/telemetry.ndjson.
func (s *Store) ResolvedFile() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, _ := s.readState()
	return s.resolvedFile(state)
}
