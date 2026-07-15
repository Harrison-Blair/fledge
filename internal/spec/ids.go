package spec

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"unicode"
)

// NextID returns the next sequential zero-padded ID (e.g. FTHR-004) for the
// given prefix, scanning existing filenames in dir. Padding is 3 digits, or
// wider if an existing ID is wider.
func NextID(dir, prefix string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Sprintf("%s-%03d", prefix, 1), nil
		}
		return "", err
	}
	re := regexp.MustCompile(`^` + regexp.QuoteMeta(prefix) + `-(\d+)[-.]`)
	max, width := 0, 3
	for _, e := range entries {
		m := re.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		n, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}
		if n > max {
			max = n
		}
		if len(m[1]) > width {
			width = len(m[1])
		}
	}
	return fmt.Sprintf("%s-%0*d", prefix, width, max+1), nil
}

// allocLockName is the dedicated hidden lock file used to serialize
// AllocateAndCreate within a single allocation directory. It is a dotfile
// with no .md suffix, so NextID's regexp scan, spec loading, and preen all
// ignore it.
const allocLockName = ".alloc.lock"

// AllocateAndCreate serializes NextID and the O_EXCL file create it guards
// behind an exclusive flock on <dir>/.alloc.lock, so two processes racing to
// allocate an ID in the same dir never both win: the loser blocks on the
// flock until the winner has created its file and released the lock, then
// sees the winner's file in its own NextID scan. build receives the
// allocated id and returns the file path and content to create. Separate
// dirs (e.g. plumages vs. feathers) use separate lock files and never block
// each other.
func AllocateAndCreate(dir, prefix string, build func(id string) (path string, content []byte)) (id, path string, err error) {
	unlock, err := lockAllocDir(dir)
	if err != nil {
		return "", "", err
	}
	defer unlock()

	id, err = NextID(dir, prefix)
	if err != nil {
		return "", "", err
	}
	path, content := build(id)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", "", err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return "", "", err
	}
	if _, err := f.Write(content); err != nil {
		f.Close()
		return "", "", err
	}
	if err := f.Close(); err != nil {
		return "", "", err
	}
	return id, path, nil
}

// lockAllocDir acquires an exclusive flock on dir's allocation lock file,
// creating dir and the lock file if absent, and returns a func that releases
// the lock and closes the file.
func lockAllocDir(dir string) (func(), error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(dir, allocLockName), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, err
	}
	return func() {
		syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}, nil
}

// Kebab lowercases s and replaces every run of non-alphanumeric characters
// with a single hyphen. Unicode letters and digits are preserved.
func Kebab(s string) string {
	var b strings.Builder
	prevHyphen := true // suppress leading hyphen
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			prevHyphen = false
		} else if !prevHyphen {
			b.WriteByte('-')
			prevHyphen = true
		}
	}
	return strings.TrimRight(b.String(), "-")
}
