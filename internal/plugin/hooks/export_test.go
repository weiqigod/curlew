package hooks

import "time"

// SetHookTimeoutForTesting directly sets the hook timeout on d without going
// through NewDispatcher options. Use this in tests that construct a Dispatcher
// via buildDispatcher and need to shorten the timeout afterwards.
func SetHookTimeoutForTesting(d *Dispatcher, timeout time.Duration) {
	d.hookTimeout = timeout
}
