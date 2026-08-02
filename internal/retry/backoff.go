package retry

import (
	"math"
	"math/rand"
	"time"
)

// Strategy represents a backoff strategy type.
type Strategy string

const (
	// StrategyExponential doubles the delay after each attempt.
	StrategyExponential Strategy = "exponential"
	// StrategyLinear increases the delay linearly with each attempt.
	StrategyLinear Strategy = "linear"
	// StrategyConstant uses the same delay for every attempt.
	StrategyConstant Strategy = "constant"
)

// ValidStrategy reports whether s is a recognized backoff strategy.
func ValidStrategy(s string) bool {
	switch Strategy(s) {
	case StrategyExponential, StrategyLinear, StrategyConstant:
		return true
	}
	return false
}

// DefaultMaxDelayMs is the maximum delay cap when none is specified.
const DefaultMaxDelayMs = 30000

// CalculateDelay computes the delay for the given attempt using the specified strategy.
// attempt is 0-indexed (0 = first retry delay).
// The result is capped at maxDelayMs. If maxDelayMs <= 0, defaults to DefaultMaxDelayMs.
func CalculateDelay(strategy Strategy, attempt, initialDelayMs, maxDelayMs int) time.Duration {
	if maxDelayMs <= 0 {
		maxDelayMs = DefaultMaxDelayMs
	}

	var ms float64
	switch strategy {
	case StrategyLinear:
		ms = float64(initialDelayMs) * float64(attempt+1)
	case StrategyConstant:
		ms = float64(initialDelayMs)
	default: // exponential (and unknown strategies)
		ms = float64(initialDelayMs) * math.Pow(2, float64(attempt))
	}

	maxMs := float64(maxDelayMs)
	if ms > maxMs {
		ms = maxMs
	}
	return time.Duration(ms) * time.Millisecond
}

// JitterFunc is a function that returns a random float64 in [-1, 1).
// Injectable for deterministic testing.
type JitterFunc func() float64

// DefaultJitter returns a random float64 in [-1, 1).
func DefaultJitter() float64 {
	return rand.Float64()*2 - 1
}

// ApplyJitter adds randomness to a delay.
// actual_delay = delay * (1 + jitterFactor * randomValue)
// where randomValue is in [-1, 1).
// jitterFactor should be in [0, 1]. If 0 or negative, returns delay unchanged.
func ApplyJitter(delay time.Duration, jitterFactor float64, rng JitterFunc) time.Duration {
	if jitterFactor <= 0 || rng == nil {
		return delay
	}
	factor := 1.0 + jitterFactor*rng()
	return time.Duration(float64(delay) * factor)
}
