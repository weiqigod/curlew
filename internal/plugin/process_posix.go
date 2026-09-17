//go:build !windows

package plugin

import (
	"fmt"
	"io"
	"os/exec"
	"sync"
	"syscall"
)

func spawnPlugin(path string, stderr io.Writer) (io.WriteCloser, io.ReadCloser, func(), error) {
	cmd := exec.Command(path) //nolint:gosec // plugin path is explicitly configured
	cmd.Stderr = stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, nil, nil, fmt.Errorf("stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return nil, nil, nil, fmt.Errorf("start: %w", err)
	}

	waited := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(waited)
	}()
	var once sync.Once
	stop := func() {
		once.Do(func() {
			_ = stdin.Close()
			if !waitPluginExit(waited, shutdownTimeout) {
				_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
				if !waitPluginExit(waited, shutdownTimeout) {
					_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
					_ = waitPluginExit(waited, shutdownTimeout)
				}
			}
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		})
	}
	return stdin, stdout, stop, nil
}
