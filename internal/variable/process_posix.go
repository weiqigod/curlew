//go:build !windows

package variable

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

func runContainedCommand(cmd *exec.Cmd) error {
	attributes := syscall.SysProcAttr{}
	if cmd.SysProcAttr != nil {
		attributes = *cmd.SysProcAttr
	}
	attributes.Setpgid = true
	attributes.Pgid = 0
	cmd.SysProcAttr = &attributes
	killGroup := func() error {
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		if err != nil {
			return fmt.Errorf("kill command process group: %w", err)
		}
		return nil
	}
	cmd.Cancel = killGroup
	if err := cmd.Start(); err != nil {
		return err
	}
	result := cmd.Wait()
	if err := killGroup(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return errors.Join(result, err)
	}
	return result
}
