//go:build windows

package plugin

import "os"

// interruptSignal returns the signal used to request graceful shutdown.
func interruptSignal() os.Signal { return os.Interrupt }
