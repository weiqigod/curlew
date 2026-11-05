//go:build !windows

package plugin

import (
	"fmt"
	"io"
	"os/exec"
	"sync"
	"syscall"
	"time"
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
			stopPluginProcessGroup(waited, cmd.Process.Pid, shutdownTimeout, syscall.Kill)
		})
	}
	return stdin, stdout, stop, nil
}

func stopPluginProcessGroup(waited <-chan struct{}, pid int, timeout time.Duration, kill func(int, syscall.Signal) error) {
	if waitPluginExit(waited, timeout) {
		return
	}
	_ = kill(-pid, syscall.SIGTERM)
	if waitPluginExit(waited, timeout) {
		return
	}
	_ = kill(-pid, syscall.SIGKILL)
	_ = waitPluginExit(waited, timeout)
}
