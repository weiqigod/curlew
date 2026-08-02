package variable

import (
	"sort"
	"strings"
	"sync"
)

// Redacted is the replacement text for sensitive variable values in output.
const Redacted = "[REDACTED]"

// sensitiveKeywords are lowercase substrings that trigger auto-sensitivity detection.
var sensitiveKeywords = []string{
	"password", "passwd", "token", "secret",
	"api_key", "apikey", "credential", "authorization",
}

// sensitiveHeaderNames are lowercase header names always treated as sensitive.
var sensitiveHeaderNames = []string{
	"authorization", "x-api-key", "x-auth-token",
	"proxy-authorization", "cookie",
}

// IsSensitiveName returns true if name contains a known sensitive keyword
// (case-insensitive substring match).
func IsSensitiveName(name string) bool {
	lower := strings.ToLower(name)
	for _, kw := range sensitiveKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// IsSensitiveHeaderName returns true if the HTTP header name is inherently sensitive.
func IsSensitiveHeaderName(name string) bool {
	lower := strings.ToLower(name)
	for _, h := range sensitiveHeaderNames {
		if lower == h {
			return true
		}
	}
	return false
}

// SensitiveSet tracks sensitive variable names and the concrete values they
// resolved to (used to redact values that appear inside request and response
// bodies). All exported methods are safe for concurrent use.
type SensitiveSet struct {
	mu     sync.RWMutex
	names  map[string]bool
	values map[string]struct{}
}

// NewSensitiveSet returns an initialised empty SensitiveSet.
func NewSensitiveSet() *SensitiveSet {
	return &SensitiveSet{
		names:  make(map[string]bool),
		values: make(map[string]struct{}),
	}
}

// AddValue registers a concrete sensitive string that RedactBody should
// replace wherever it appears. Empty values are ignored to avoid degenerate
// matches. Safe for concurrent use.
func (s *SensitiveSet) AddValue(value string) {
	if s == nil || value == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.values == nil {
		s.values = make(map[string]struct{})
	}
	s.values[value] = struct{}{}
}

// Values returns the registered sensitive values sorted longest-first, then
// lexicographically, so RedactBody always redacts the most-specific match
// first. Safe for concurrent use.
func (s *SensitiveSet) Values() []string {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.values) == 0 {
		return nil
	}
	out := make([]string, 0, len(s.values))
	for v := range s.values {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool {
		if len(out[i]) != len(out[j]) {
			return len(out[i]) > len(out[j])
		}
		return out[i] < out[j]
	})
	return out
}

// Add marks name as sensitive. Safe for concurrent use.
func (s *SensitiveSet) Add(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.names[name] = true
}

// IsSensitive returns true if name was explicitly added to the set.
// Safe for concurrent use.
func (s *SensitiveSet) IsSensitive(name string) bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.names[name]
}

// Merge adds all names and values from other into s. Nil other is a no-op.
// Safe for concurrent use.
func (s *SensitiveSet) Merge(other *SensitiveSet) {
	if other == nil {
		return
	}
	// Read from other under its own lock.
	other.mu.RLock()
	otherNames := make(map[string]bool, len(other.names))
	for k, v := range other.names {
		otherNames[k] = v
	}
	otherValues := make(map[string]struct{}, len(other.values))
	for v := range other.values {
		otherValues[v] = struct{}{}
	}
	other.mu.RUnlock()

	// Write into s under its own lock.
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.names == nil {
		s.names = make(map[string]bool)
	}
	for k, v := range otherNames {
		s.names[k] = v
	}
	if s.values == nil {
		s.values = make(map[string]struct{})
	}
	for v := range otherValues {
		s.values[v] = struct{}{}
	}
}

// AddHeuristicNames scans keys in vars and adds any whose name matches a
// sensitive keyword heuristic. Nil or empty vars is a no-op. Safe for
// concurrent use.
func (s *SensitiveSet) AddHeuristicNames(vars map[string]string) {
	for name := range vars {
		if IsSensitiveName(name) {
			s.Add(name)
		}
	}
}

// Names returns the list of sensitive variable names (for debugging).
// Safe for concurrent use.
func (s *SensitiveSet) Names() []string {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.names))
	for k := range s.names {
		out = append(out, k)
	}
	return out
}

// RedactValue returns Redacted if name is in the sensitive set and allow is false;
// otherwise it returns value unchanged.
func RedactValue(name, value string, s *SensitiveSet, allow bool) string {
	if allow {
		return value
	}
	if s.IsSensitive(name) {
		return Redacted
	}
	return value
}

// RedactHeaders returns a shallow copy of headers with sensitive values replaced.
// A header is redacted when its name is inherently sensitive (Authorization, etc.)
// or when it is listed in s. Returns nil if headers is nil.
func RedactHeaders(headers map[string]string, s *SensitiveSet, allow bool) map[string]string {
	if headers == nil {
		return nil
	}
	out := make(map[string]string, len(headers))
	if allow {
		for k, v := range headers {
			out[k] = v
		}
		return out
	}
	for k, v := range headers {
		if IsSensitiveHeaderName(k) || s.IsSensitive(k) {
			out[k] = Redacted
		} else {
			out[k] = v
		}
	}
	return out
}
