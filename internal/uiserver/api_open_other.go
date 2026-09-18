//go:build !windows

package uiserver

import "os/exec"

func startEditor(dir string, args []string) error {
	cmd := exec.Command(args[0], args[1:]...) //nolint:gosec // explicitly configured local editor
	cmd.Dir = dir
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
