//go:build !windows

package examplecheck

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// prepareProcessCommand gives a child its own process group so context
// cancellation reaches background descendants, not only the direct process.
func prepareProcessCommand(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Cancel = func() error {
		if command.Process == nil {
			return nil
		}
		err := syscall.Kill(-command.Process.Pid, syscall.SIGTERM)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	command.WaitDelay = 2 * time.Second
}

// finishProcessCommand verifies that the process group is gone before the
// checker continues. It gives graceful termination a short window before
// escalating to SIGKILL.
func finishProcessCommand(command *exec.Cmd) error {
	if command.Process == nil {
		return nil
	}
	pgid := command.Process.Pid
	if err := syscall.Kill(-pgid, 0); errors.Is(err, syscall.ESRCH) {
		return nil
	}
	if err := syscall.Kill(-pgid, syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(-pgid, 0); errors.Is(err, syscall.ESRCH) {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := syscall.Kill(-pgid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	deadline = time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(-pgid, 0); errors.Is(err, syscall.ESRCH) {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return fmt.Errorf("process group did not terminate")
}

// stopMockProcess requests the same graceful shutdown used by an interactive
// mock process.
func stopMockProcess(process *os.Process) error {
	return process.Signal(syscall.SIGTERM)
}

func expectedMockStopWaitError(error) bool {
	return false
}
