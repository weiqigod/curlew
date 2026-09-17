//go:build !windows

package plugin

import "os"

func isExecutable(_ string, mode os.FileMode) bool {
	return mode.IsRegular() && mode&0o111 != 0
}