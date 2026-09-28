//go:build windows

package examplecheck

import (
	"errors"
	"os"
	"os/exec"
	"time"
)

// prepareProcessCommand keeps the package portable on Windows. Checked shell
// workflows do not run there, so cancellation targets only the direct child.
func prepareProcessCommand(command *exec.Cmd) {
	command.Cancel = func() error {
		if command.Process == nil {
			return nil
		}
		return command.Process.Kill()
	}
	command.WaitDelay = 2 * time.Second
}

// finishProcessCommand has no process-group work on Windows.
func finishProcessCommand(_ *exec.Cmd) error {
	return nil
}

// stopMockProcess terminates the standalone mock on Windows, where the Unix
// signal-based shutdown path is unavailable.
func stopMockProcess(process *os.Process) error {
	return process.Kill()
}

func expectedMockStopWaitError(err error) bool {
	var exitErr *exec.ExitError
	return errors.Is(err, os.ErrProcessDone) || errors.As(err, &exitErr)
}
