package variable

import "os/exec"

func runContainedCommand(cmd *exec.Cmd) error {
	return cmd.Run()
}
