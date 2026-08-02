// Package loadgen implements a file-driven HTTP load generator used by the
// curlew perf subcommand. A fixed pool of virtual users (VUs) executes a
// single parsed request in a loop against a target server. Optional
// linear ramp-up paces VU activation; optional constant-RPS throughput
// mode paces requests via a shared time.Ticker.
package loadgen

import (
	"errors"
	"time"
)

// Sentinel errors for well-known configuration failures.
var (
	ErrInvalidVUs      = errors.New("--vus must be >= 1")
	ErrInvalidDuration = errors.New("--duration must be > 0")
	ErrInvalidRampUp   = errors.New("--ramp-up must be >= 0 and <= duration")
	ErrInvalidRPS      = errors.New("--rps must be >= 0")
)

// Config captures the run configuration. VUs and Duration are required.
type Config struct {
	VUs      int           // --vus
	Duration time.Duration // --duration
	RampUp   time.Duration // --ramp-up (0 = no ramp)
	RPS      int           // --rps (0 = unbounded throughput)
}

// Validate returns an error if the Config is unusable.
func (c Config) Validate() error {
	if c.VUs < 1 {
		return ErrInvalidVUs
	}
	if c.Duration <= 0 {
		return ErrInvalidDuration
	}
	if c.RampUp < 0 || c.RampUp > c.Duration {
		return ErrInvalidRampUp
	}
	if c.RPS < 0 {
		return ErrInvalidRPS
	}
	return nil
}

// Summary captures the outcome of a perf run.
type Summary struct {
	Requests  int           // total issued
	Successes int           // 2xx/3xx responses
	Failures  int           // transport errors or >=4xx
	Elapsed   time.Duration // from first VU start to final return
	Aborted   bool          // set true when run ended due to ctx cancel
}
