//go:build !windows

package lifecycle

import (
	"errors"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestWatcherCommandDetachesFromTheCallersProcessGroup(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	command, devNull, err := watcherCommand("/usr/local/bin/fledge", root)
	if err != nil {
		t.Fatal(err)
	}
	defer devNull.Close()

	if got := strings.Join(command.Args, " "); got != "/usr/local/bin/fledge watch --daemon" {
		t.Errorf("watcher args = %q", got)
	}
	if command.Dir != root {
		t.Errorf("watcher Dir = %q, want %q", command.Dir, root)
	}
	if command.SysProcAttr == nil || !command.SysProcAttr.Setsid {
		t.Errorf("watcher SysProcAttr = %#v, want Setsid so Ctrl-C in the TUI cannot kill it", command.SysProcAttr)
	}
	if command.Stdin != devNull || command.Stdout != devNull || command.Stderr != devNull {
		t.Errorf("watcher stdio = %v/%v/%v, want the null device", command.Stdin, command.Stdout, command.Stderr)
	}
}

func TestStartAndReapCollectsTheExitedProcess(t *testing.T) {
	t.Parallel()

	command := exec.Command("/bin/sh", "-c", "exit 0")
	if err := startAndReap(command); err != nil {
		t.Fatal(err)
	}
	pid := command.Process.Pid
	if pid <= 0 {
		t.Fatalf("watcher pid = %d; a released process can never be reaped", pid)
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		if err := syscall.Kill(pid, 0); errors.Is(err, syscall.ESRCH) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("process %d still exists after exiting; startAndReap left a zombie", pid)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
