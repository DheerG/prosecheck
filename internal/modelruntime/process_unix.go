//go:build !windows

package modelruntime

import (
	"os"
	"os/exec"
	"syscall"
)

func configureCommand(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

func interruptProcess(process *os.Process) error {
	return process.Signal(os.Interrupt)
}
