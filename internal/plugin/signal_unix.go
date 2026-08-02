//go:build !windows

package plugin

import (
	"os"
	"syscall"
)

// interruptSignal returns the signal used to request graceful shutdown.
func interruptSignal() os.Signal { return syscall.SIGTERM }
