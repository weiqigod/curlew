package plugin

import "time"

func waitPluginExit(waited <-chan struct{}, timeout time.Duration) bool {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-waited:
		return true
	case <-timer.C:
		return false
	}
}
