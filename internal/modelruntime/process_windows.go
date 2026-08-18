//go:build windows

package modelruntime

import (
	"os"
	"os/exec"
)

func configureCommand(command *exec.Cmd) {}

func interruptProcess(process *os.Process) error {
	return process.Kill()
}
