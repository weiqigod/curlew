package variable

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestExecuteProgramWithTimeout(t *testing.T) {
	program := buildProgramHelper(t)

	t.Run("runs_past_the_default_30s_cap", func(t *testing.T) {
		if testing.Short() {
			t.Skip("waits 32 seconds")
		}
		started := time.Now()
		output, err := ExecuteProgramWithTimeout(context.Background(), time.Minute, program, []string{"sleep", "32s"}, nil)
		if err != nil || output != "slept" {
			t.Fatalf("output = %q, error = %v after %v", output, err, time.Since(started))
		}
	})

	t.Run("stops_at_the_given_timeout", func(t *testing.T) {
		started := time.Now()
		_, err := ExecuteProgramWithTimeout(context.Background(), 2*time.Second, program, []string{"wait"}, nil)
		elapsed := time.Since(started)
		if !errors.Is(err, context.DeadlineExceeded) || !errors.Is(err, ErrCommandFailed) || elapsed > 15*time.Second {
			t.Fatalf("error = %v, elapsed = %v, want deadline near 2s", err, elapsed)
		}
	})
}
