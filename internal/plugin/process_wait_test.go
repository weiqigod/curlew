package plugin

import (
	"testing"
	"time"
)

func TestWaitPluginExitIsBounded(t *testing.T) {
	t.Run("reports process exit", func(t *testing.T) {
		waited := make(chan struct{})
		close(waited)
		if !waitPluginExit(waited, time.Second) {
			t.Fatal("closed process channel reported timeout")
		}
	})

	t.Run("returns after timeout", func(t *testing.T) {
		waited := make(chan struct{})
		start := time.Now()
		if waitPluginExit(waited, 20*time.Millisecond) {
			t.Fatal("open process channel reported exit")
		}
		if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
			t.Fatalf("bounded wait took %s", elapsed)
		}
	})
}