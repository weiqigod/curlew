//go:build !windows

package plugin

import (
	"syscall"
	"testing"
	"time"
)

func TestStopPluginProcessGroupSkipsSignalsAfterCooperativeExit(t *testing.T) {
	waited := make(chan struct{})
	close(waited)
	var signals []syscall.Signal

	stopPluginProcessGroup(waited, 123, time.Millisecond, func(_ int, signal syscall.Signal) error {
		signals = append(signals, signal)
		return nil
	})

	if len(signals) != 0 {
		t.Fatalf("signals after cooperative exit = %v, want none", signals)
	}
}
