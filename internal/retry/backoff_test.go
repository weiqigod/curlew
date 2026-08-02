package retry

import (
	"testing"
	"time"
)

func TestCalculateDelay(t *testing.T) {
	tests := []struct {
		name           string
		strategy       Strategy
		attempt        int
		initialDelayMs int
		maxDelayMs     int
		want           time.Duration
	}{
		// Exponential
		{"exponential attempt 0", StrategyExponential, 0, 1000, 30000, 1 * time.Second},
		{"exponential attempt 1", StrategyExponential, 1, 1000, 30000, 2 * time.Second},
		{"exponential attempt 2", StrategyExponential, 2, 1000, 30000, 4 * time.Second},
		{"exponential attempt 3", StrategyExponential, 3, 1000, 30000, 8 * time.Second},
		{"exponential capped at max_delay_ms", StrategyExponential, 10, 1000, 30000, 30 * time.Second},
		// Linear
		{"linear attempt 0", StrategyLinear, 0, 1000, 30000, 1 * time.Second},
		{"linear attempt 1", StrategyLinear, 1, 1000, 30000, 2 * time.Second},
		{"linear attempt 2", StrategyLinear, 2, 1000, 30000, 3 * time.Second},
		{"linear attempt 3", StrategyLinear, 3, 1000, 30000, 4 * time.Second},
		{"linear capped at max_delay_ms", StrategyLinear, 40, 1000, 30000, 30 * time.Second},
		// Constant
		{"constant attempt 0", StrategyConstant, 0, 2000, 30000, 2 * time.Second},
		{"constant attempt 5", StrategyConstant, 5, 2000, 30000, 2 * time.Second},
		// Edge cases
		{"default max_delay_ms when zero", StrategyExponential, 10, 1000, 0, 30 * time.Second},
		{"custom max_delay_ms", StrategyExponential, 5, 1000, 5000, 5 * time.Second},
		{"unknown strategy defaults to exponential", Strategy("unknown"), 1, 1000, 30000, 2 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculateDelay(tt.strategy, tt.attempt, tt.initialDelayMs, tt.maxDelayMs)
			if got != tt.want {
				t.Errorf("CalculateDelay(%q, %d, %d, %d) = %v, want %v",
					tt.strategy, tt.attempt, tt.initialDelayMs, tt.maxDelayMs, got, tt.want)
			}
		})
	}
}

func TestApplyJitter(t *testing.T) {
	tests := []struct {
		name         string
		delay        time.Duration
		jitterFactor float64
		rngValue     float64
		wantNilRng   bool
		want         time.Duration
	}{
		{"positive jitter +10%", 1000 * time.Millisecond, 0.1, 1.0, false, 1100 * time.Millisecond},
		{"negative jitter -10%", 1000 * time.Millisecond, 0.1, -1.0, false, 900 * time.Millisecond},
		{"zero jitter factor returns unchanged", 1000 * time.Millisecond, 0.0, 0.5, false, 1000 * time.Millisecond},
		{"nil rng returns unchanged", 1000 * time.Millisecond, 0.1, 0, true, 1000 * time.Millisecond},
		{"20% jitter", 1000 * time.Millisecond, 0.2, 1.0, false, 1200 * time.Millisecond},
		{"50% jitter", 1000 * time.Millisecond, 0.5, -1.0, false, 500 * time.Millisecond},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var rng JitterFunc
			if !tt.wantNilRng {
				val := tt.rngValue
				rng = func() float64 { return val }
			}
			got := ApplyJitter(tt.delay, tt.jitterFactor, rng)
			if got != tt.want {
				t.Errorf("ApplyJitter(%v, %v, rng=%v) = %v, want %v",
					tt.delay, tt.jitterFactor, tt.rngValue, got, tt.want)
			}
		})
	}
}

func TestValidStrategy(t *testing.T) {
	tests := []struct {
		name string
		s    string
		want bool
	}{
		{"exponential valid", "exponential", true},
		{"linear valid", "linear", true},
		{"constant valid", "constant", true},
		{"empty invalid", "", false},
		{"unknown invalid", "random", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ValidStrategy(tt.s)
			if got != tt.want {
				t.Errorf("ValidStrategy(%q) = %v, want %v", tt.s, got, tt.want)
			}
		})
	}
}
