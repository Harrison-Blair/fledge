//go:build linux

package herdr

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

// observePaneReady reports whether the pane's shell is at an interactive
// prompt: the shell has a controlling terminal, the shell's process group is
// that terminal's foreground group (as seen by the kernel and, when reported,
// by Herder), and the terminal is out of canonical mode. Any error means the
// sample counts as not ready; only errReadinessUnsupported stops the wait.
//
// The foreground group comes from the tpgid field of /proc/PID/stat rather
// than TIOCGPGRP: the kernel refuses that ioctl on a slave terminal unless it
// is the caller's own controlling terminal, and Fledge never sits in the
// pane's session.
func observePaneReady(info ProcessInfo) (bool, error) {
	if info.ShellPID == nil || *info.ShellPID == 0 || *info.ShellPID > math.MaxInt32 {
		return false, errors.New("no shell pid")
	}
	pid := int(*info.ShellPID)
	stat, err := readProcStat(pid)
	if err != nil {
		return false, err
	}
	if stat.pgrp <= 0 || stat.ttyNr == 0 {
		return false, errors.New("shell has no controlling terminal")
	}
	if stat.tpgid != stat.pgrp {
		return false, errors.New("shell is not the terminal foreground group")
	}
	if info.ForegroundPGID != nil && uint64(*info.ForegroundPGID) != uint64(stat.pgrp) {
		return false, errors.New("reported foreground group is not the shell")
	}

	path := "/proc/" + strconv.Itoa(pid) + "/fd/0"
	if info.TTY != nil && isPtsPath(*info.TTY) {
		path = *info.TTY
	}
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NOCTTY|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return false, fmt.Errorf("open terminal: %w", err)
	}
	defer unix.Close(fd)

	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		return false, fmt.Errorf("stat terminal: %w", err)
	}
	if st.Mode&unix.S_IFMT != unix.S_IFCHR {
		return false, errors.New("terminal is not a character device")
	}
	rdev := uint64(st.Rdev)
	if unix.Major(rdev) != unix.Major(stat.ttyNr) || unix.Minor(rdev) != unix.Minor(stat.ttyNr) {
		return false, errors.New("terminal is not the shell's controlling terminal")
	}
	termios, err := unix.IoctlGetTermios(fd, unix.TCGETS)
	if err != nil {
		return false, fmt.Errorf("terminal attributes: %w", err)
	}
	return termios.Lflag&unix.ICANON == 0, nil
}

// procStat is the subset of /proc/PID/stat the readiness check needs.
type procStat struct {
	pgrp  int
	ttyNr uint64
	tpgid int
}

func readProcStat(pid int) (procStat, error) {
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return procStat{}, fmt.Errorf("read shell stat: %w", err)
	}
	return parseProcStat(data)
}

// parseProcStat reads the fields following the command name, which is
// delimited by the final right parenthesis because the name itself may
// contain parentheses and spaces.
func parseProcStat(data []byte) (procStat, error) {
	end := bytes.LastIndexByte(data, ')')
	if end < 0 {
		return procStat{}, errors.New("malformed stat: no command name")
	}
	// After the name: state ppid pgrp session tty_nr tpgid ...
	fields := strings.Fields(string(data[end+1:]))
	if len(fields) < 6 {
		return procStat{}, errors.New("malformed stat: too few fields")
	}
	pgrp, err := strconv.Atoi(fields[2])
	if err != nil {
		return procStat{}, fmt.Errorf("malformed stat: pgrp: %w", err)
	}
	ttyNr, err := strconv.ParseInt(fields[4], 10, 64)
	if err != nil || ttyNr < 0 {
		return procStat{}, errors.New("malformed stat: tty_nr")
	}
	tpgid, err := strconv.Atoi(fields[5])
	if err != nil {
		return procStat{}, fmt.Errorf("malformed stat: tpgid: %w", err)
	}
	return procStat{pgrp: pgrp, ttyNr: uint64(ttyNr), tpgid: tpgid}, nil
}

// isPtsPath accepts only a concrete /dev/pts/N path with a canonical decimal
// index, never /dev/tty or a path with traversal components.
func isPtsPath(path string) bool {
	index, ok := strings.CutPrefix(path, "/dev/pts/")
	if !ok || index == "" || len(index) > 10 || (len(index) > 1 && index[0] == '0') {
		return false
	}
	for i := 0; i < len(index); i++ {
		if index[i] < '0' || index[i] > '9' {
			return false
		}
	}
	return true
}
