//go:build linux

package herdr

import (
	"math"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"golang.org/x/sys/unix"
)

// openPTY allocates a pseudo-terminal pair and returns the slave path. The
// master stays open until the test ends so the slave keeps existing.
func openPTY(t *testing.T) string {
	t.Helper()
	master, err := unix.Open("/dev/ptmx", unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Skipf("open /dev/ptmx: %v", err)
	}
	t.Cleanup(func() { unix.Close(master) })
	index, err := unix.IoctlGetUint32(master, unix.TIOCGPTN)
	if err != nil {
		t.Fatal(err)
	}
	if err := unix.IoctlSetPointerInt(master, unix.TIOCSPTLCK, 0); err != nil {
		t.Fatal(err)
	}
	return "/dev/pts/" + strconv.FormatUint(uint64(index), 10)
}

// shellLike starts a long-lived process whose controlling terminal is the
// PTY at path and whose stdin is that terminal, or a pipe when stdinPipe is
// set. Like a real shell it leads its own session and process group.
func shellLike(t *testing.T, path string, stdinPipe bool) *exec.Cmd {
	t.Helper()
	slave, err := os.OpenFile(path, os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer slave.Close()

	cmd := exec.Command("sleep", "600")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = slave, slave, slave
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	if stdinPipe {
		reader, writer, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { writer.Close() })
		defer reader.Close()
		cmd.Stdin = reader
		cmd.SysProcAttr.Ctty = 1
	}
	if err := cmd.Start(); err != nil {
		t.Skipf("start process with controlling terminal: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	return cmd
}

func setCanonical(t *testing.T, path string, canonical bool) {
	t.Helper()
	fd, err := unix.Open(path, unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(fd)
	termios, err := unix.IoctlGetTermios(fd, unix.TCGETS)
	if err != nil {
		t.Fatal(err)
	}
	if canonical {
		termios.Lflag |= unix.ICANON
	} else {
		termios.Lflag &^= unix.ICANON
	}
	if err := unix.IoctlSetTermios(fd, unix.TCSETS, termios); err != nil {
		t.Fatal(err)
	}
}

func shellInfo(cmd *exec.Cmd, tty *string) ProcessInfo {
	pid := uint32(cmd.Process.Pid)
	return ProcessInfo{PaneID: "w1:p1", ShellPID: &pid, TTY: tty, ForegroundPGID: &pid}
}

func openDescriptors(t *testing.T) int {
	t.Helper()
	entries, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Fatal(err)
	}
	return len(entries)
}

func TestObservePaneReadyFollowsCanonicalMode(t *testing.T) {
	path := openPTY(t)
	shell := shellLike(t, path, false)
	info := shellInfo(shell, &path)
	before := openDescriptors(t)

	if ready, err := observePaneReady(info); ready || err != nil {
		t.Fatalf("fresh terminal: ready=%v err=%v, want not ready in canonical mode", ready, err)
	}
	setCanonical(t, path, false)
	if ready, err := observePaneReady(info); !ready || err != nil {
		t.Fatalf("raw terminal: ready=%v err=%v, want ready", ready, err)
	}
	setCanonical(t, path, true)
	if ready, err := observePaneReady(info); ready || err != nil {
		t.Fatalf("canonical again: ready=%v err=%v, want not ready", ready, err)
	}
	if after := openDescriptors(t); after != before {
		t.Fatalf("descriptors open: %d before, %d after", before, after)
	}
}

func TestObservePaneReadyFallsBackToShellStdin(t *testing.T) {
	path := openPTY(t)
	shell := shellLike(t, path, false)
	setCanonical(t, path, false)

	for name, tty := range map[string]*string{"null tty": nil, "non-pts tty": ptr("/dev/tty"), "relative tty": ptr("/dev/pts/../pts/1")} {
		t.Run(name, func(t *testing.T) {
			if ready, err := observePaneReady(shellInfo(shell, tty)); !ready || err != nil {
				t.Fatalf("ready=%v err=%v, want ready through /proc/PID/fd/0", ready, err)
			}
		})
	}
}

func TestObservePaneReadyRejectsWrongTerminalOrOwner(t *testing.T) {
	path := openPTY(t)
	other := openPTY(t)
	shell := shellLike(t, path, false)
	setCanonical(t, path, false)
	setCanonical(t, other, false)
	pid := uint32(shell.Process.Pid)
	otherGroup := pid + 1
	before := openDescriptors(t)

	tests := []struct {
		name string
		info ProcessInfo
		want string
	}{
		{name: "other terminal", info: ProcessInfo{ShellPID: &pid, TTY: &other, ForegroundPGID: &pid}, want: "not the shell's controlling terminal"},
		{name: "foreground mismatch", info: ProcessInfo{ShellPID: &pid, TTY: &path, ForegroundPGID: &otherGroup}, want: "reported foreground group is not the shell"},
		{name: "no shell pid", info: ProcessInfo{TTY: &path}, want: "no shell pid"},
		{name: "zero shell pid", info: ProcessInfo{ShellPID: ptr[uint32](0), TTY: &path}, want: "no shell pid"},
		{name: "oversized shell pid", info: ProcessInfo{ShellPID: ptr[uint32](math.MaxUint32), TTY: &path}, want: "no shell pid"},
		{name: "missing process", info: ProcessInfo{ShellPID: ptr[uint32](math.MaxInt32), TTY: &path}, want: "read shell stat"},
		{name: "no controlling terminal", info: ProcessInfo{ShellPID: ptr(uint32(os.Getpid())), TTY: &path}, want: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ready, err := observePaneReady(tc.info)
			if ready || err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("ready=%v err=%v, want error containing %q", ready, err, tc.want)
			}
		})
	}
	if after := openDescriptors(t); after != before {
		t.Fatalf("descriptors open: %d before, %d after", before, after)
	}
}

func TestObservePaneReadyRejectsNonTerminalStdin(t *testing.T) {
	path := openPTY(t)
	shell := shellLike(t, path, true)
	setCanonical(t, path, false)

	ready, err := observePaneReady(shellInfo(shell, nil))
	if ready || err == nil || !strings.Contains(err.Error(), "not a character device") {
		t.Fatalf("ready=%v err=%v", ready, err)
	}
	if ready, err := observePaneReady(shellInfo(shell, &path)); !ready || err != nil {
		t.Fatalf("explicit tty: ready=%v err=%v, want ready", ready, err)
	}
}

func TestParseProcStat(t *testing.T) {
	tests := []struct {
		name string
		data string
		want procStat
		err  string
	}{
		{
			name: "plain",
			data: "4242 (bash) S 4000 4242 4242 34823 4242 4194560 1 0 0 0\n",
			want: procStat{pgrp: 4242, ttyNr: 34823, tpgid: 4242},
		},
		{
			name: "command name with parenthesis and spaces",
			data: "4242 (a) S 1) (b) R 4000 77 77 34816 88 4194560\n",
			want: procStat{pgrp: 77, ttyNr: 34816, tpgid: 88},
		},
		{name: "no command name", data: "4242 bash S 1 2 3 4 5", err: "no command name"},
		{name: "too few fields", data: "4242 (bash) S 1 2 3 4", err: "too few fields"},
		{name: "negative tty", data: "4242 (bash) S 1 2 3 -4 5", err: "tty_nr"},
		{name: "non-numeric pgrp", data: "4242 (bash) S 1 x 3 4 5", err: "pgrp"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseProcStat([]byte(tc.data))
			if tc.err != "" {
				if err == nil || !strings.Contains(err.Error(), tc.err) {
					t.Fatalf("error = %v, want containing %q", err, tc.err)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("parseProcStat = %+v, %v; want %+v", got, err, tc.want)
			}
		})
	}
}

func TestIsPtsPath(t *testing.T) {
	tests := map[string]bool{
		"/dev/pts/0":           true,
		"/dev/pts/12":          true,
		"/dev/pts/":            false,
		"/dev/pts/01":          false,
		"/dev/pts/1a":          false,
		"/dev/pts/../../tty":   false,
		"/dev/tty":             false,
		"/dev/ttys001":         false,
		"/dev/pts/12345678901": false,
		"":                     false,
	}
	for path, want := range tests {
		if got := isPtsPath(path); got != want {
			t.Errorf("isPtsPath(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestObservePaneReadyNeverOpensBareTTY(t *testing.T) {
	// A shell whose /proc entry exists but whose reported tty is /dev/tty must
	// be checked through its own stdin, never through the caller's terminal.
	path := openPTY(t)
	shell := shellLike(t, path, true)
	setCanonical(t, path, false)
	ready, err := observePaneReady(shellInfo(shell, ptr("/dev/tty")))
	if ready || err == nil || !strings.Contains(err.Error(), "not a character device") {
		t.Fatalf("ready=%v err=%v", ready, err)
	}
}
