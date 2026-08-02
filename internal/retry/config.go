// Package retry provides configurable retry logic for HTTP request execution.
package retry

import "net/http"

const (
	// DefaultMaxAttempts is the spec default when retry is enabled without explicit max_attempts.
	DefaultMaxAttempts = 3
	// DefaultInitialDelayMs is the spec default when retry is enabled without explicit initial_delay_ms.
	DefaultInitialDelayMs = 1000
)

// Config represents retry configuration parsed from YAML.
type Config struct {
	Enabled         bool                `yaml:"enabled"`
	MaxAttempts     int                 `yaml:"max_attempts"`
	InitialDelayMs  int                 `yaml:"initial_delay_ms"`
	BackoffStrategy string              `yaml:"backoff_strategy"`
	MaxDelayMs      int                 `yaml:"max_delay_ms"`
	Jitter          bool                `yaml:"jitter"`
	JitterFactor    float64             `yaml:"jitter_factor"`
	RetryOn         *RetryOnConfig      // nil = use defaults
	DoNotRetryOn    *DoNotRetryOnConfig // nil = no exclusions
}

// RetryOnConfig controls which conditions trigger retries.
type RetryOnConfig struct {
	StatusCodes   []int    `yaml:"status_codes,omitempty"`
	StatusRanges  []string `yaml:"status_ranges,omitempty"`
	NetworkErrors *bool    `yaml:"network_errors,omitempty"`
	Timeouts      *bool    `yaml:"timeouts,omitempty"`
	Methods       []string `yaml:"methods,omitempty"`
}

// DoNotRetryOnConfig controls exclusions from retry.
type DoNotRetryOnConfig struct {
	StatusCodes  []int    `yaml:"status_codes,omitempty"`
	StatusRanges []string `yaml:"status_ranges,omitempty"`
	Methods      []string `yaml:"methods,omitempty"`
}

// FullConfig represents retry configuration with pointer fields for merge semantics.
// Nil fields mean "not set" (inherit from lower precedence).
type FullConfig struct {
	Enabled           *bool               `yaml:"enabled,omitempty"`
	MaxAttempts       *int                `yaml:"max_attempts,omitempty"`
	BackoffStrategy   *string             `yaml:"backoff_strategy,omitempty"`
	InitialDelayMs    *int                `yaml:"initial_delay_ms,omitempty"`
	MaxDelayMs        *int                `yaml:"max_delay_ms,omitempty"`
	Jitter            *bool               `yaml:"jitter,omitempty"`
	JitterFactor      *float64            `yaml:"jitter_factor,omitempty"`
	RespectRetryAfter *bool               `yaml:"respect_retry_after,omitempty"`
	RetryOn           *RetryOnConfig      `yaml:"retry_on,omitempty"`
	DoNotRetryOn      *DoNotRetryOnConfig `yaml:"do_not_retry_on,omitempty"`
}

// BoolPtr returns a pointer to v.
func BoolPtr(v bool) *bool { return &v }

// IntPtr returns a pointer to v.
func IntPtr(v int) *int { return &v }

// Float64Ptr returns a pointer to v.
func Float64Ptr(v float64) *float64 { return &v }

// StringPtr returns a pointer to v.
func StringPtr(v string) *string { return &v }

// BuiltinDefaults returns the spec-defined built-in retry defaults.
func BuiltinDefaults() FullConfig {
	return FullConfig{
		Enabled:           BoolPtr(false),
		MaxAttempts:       IntPtr(DefaultMaxAttempts),
		BackoffStrategy:   StringPtr("exponential"),
		InitialDelayMs:    IntPtr(DefaultInitialDelayMs),
		MaxDelayMs:        IntPtr(30000),
		Jitter:            BoolPtr(false),
		RespectRetryAfter: BoolPtr(true),
	}
}

// Resolve converts a fully-merged FullConfig into the concrete Config used by ExecuteWithRetry.
func (fc FullConfig) Resolve() Config {
	var c Config
	if fc.Enabled != nil {
		c.Enabled = *fc.Enabled
	}
	if fc.MaxAttempts != nil {
		c.MaxAttempts = *fc.MaxAttempts
	}
	if fc.InitialDelayMs != nil {
		c.InitialDelayMs = *fc.InitialDelayMs
	}
	if fc.BackoffStrategy != nil {
		c.BackoffStrategy = *fc.BackoffStrategy
	}
	if fc.MaxDelayMs != nil {
		c.MaxDelayMs = *fc.MaxDelayMs
	}
	if fc.Jitter != nil {
		c.Jitter = *fc.Jitter
	}
	if fc.JitterFactor != nil {
		c.JitterFactor = *fc.JitterFactor
	}
	if fc.RetryOn != nil {
		cp := *fc.RetryOn
		if len(fc.RetryOn.StatusCodes) > 0 {
			cp.StatusCodes = make([]int, len(fc.RetryOn.StatusCodes))
			copy(cp.StatusCodes, fc.RetryOn.StatusCodes)
		}
		if len(fc.RetryOn.StatusRanges) > 0 {
			cp.StatusRanges = make([]string, len(fc.RetryOn.StatusRanges))
			copy(cp.StatusRanges, fc.RetryOn.StatusRanges)
		}
		if len(fc.RetryOn.Methods) > 0 {
			cp.Methods = make([]string, len(fc.RetryOn.Methods))
			copy(cp.Methods, fc.RetryOn.Methods)
		}
		c.RetryOn = &cp
	}
	if fc.DoNotRetryOn != nil {
		cp := *fc.DoNotRetryOn
		if len(fc.DoNotRetryOn.StatusCodes) > 0 {
			cp.StatusCodes = make([]int, len(fc.DoNotRetryOn.StatusCodes))
			copy(cp.StatusCodes, fc.DoNotRetryOn.StatusCodes)
		}
		if len(fc.DoNotRetryOn.StatusRanges) > 0 {
			cp.StatusRanges = make([]string, len(fc.DoNotRetryOn.StatusRanges))
			copy(cp.StatusRanges, fc.DoNotRetryOn.StatusRanges)
		}
		if len(fc.DoNotRetryOn.Methods) > 0 {
			cp.Methods = make([]string, len(fc.DoNotRetryOn.Methods))
			copy(cp.Methods, fc.DoNotRetryOn.Methods)
		}
		c.DoNotRetryOn = &cp
	}
	return c
}

// DefaultRetriableStatusCodes returns status codes that trigger retries.
func DefaultRetriableStatusCodes() map[int]bool {
	return map[int]bool{429: true, 502: true, 503: true, 504: true}
}

// DefaultIdempotentMethods returns HTTP methods safe to retry.
func DefaultIdempotentMethods() map[string]bool {
	return map[string]bool{
		http.MethodGet:     true,
		http.MethodHead:    true,
		http.MethodOptions: true,
		http.MethodTrace:   true,
	}
}
