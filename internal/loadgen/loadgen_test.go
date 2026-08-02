package loadgen

import (
	"errors"
	"testing"
	"time"
)

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want error
	}{
		{"zero VUs", Config{VUs: 0, Duration: time.Second}, ErrInvalidVUs},
		{"negative VUs", Config{VUs: -1, Duration: time.Second}, ErrInvalidVUs},
		{"zero duration", Config{VUs: 1, Duration: 0}, ErrInvalidDuration},
		{"negative duration", Config{VUs: 1, Duration: -time.Second}, ErrInvalidDuration},
		{"ramp-up negative", Config{VUs: 1, Duration: time.Second, RampUp: -time.Millisecond}, ErrInvalidRampUp},
		{"ramp-up exceeds duration", Config{VUs: 1, Duration: time.Second, RampUp: 2 * time.Second}, ErrInvalidRampUp},
		{"negative RPS", Config{VUs: 1, Duration: time.Second, RPS: -1}, ErrInvalidRPS},
		{"minimum valid config", Config{VUs: 1, Duration: time.Millisecond}, nil},
		{"ramp equal to duration", Config{VUs: 5, Duration: time.Second, RampUp: time.Second}, nil},
		{"rps > 0", Config{VUs: 2, Duration: time.Second, RPS: 100}, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.cfg.Validate()
			if !errors.Is(got, tc.want) {
				t.Fatalf("Validate() = %v, want %v", got, tc.want)
			}
		})
	}
}
