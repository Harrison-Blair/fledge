//go:build linux

package cli

import (
	"io"
	"os"
	"syscall"
	"unsafe"
)

func isTerminalReader(reader io.Reader) bool {
	file, ok := reader.(*os.File)
	if !ok {
		return false
	}
	var state syscall.Termios
	_, _, errno := syscall.Syscall6(
		syscall.SYS_IOCTL,
		file.Fd(),
		uintptr(syscall.TCGETS),
		uintptr(unsafe.Pointer(&state)),
		0,
		0,
		0,
	)
	return errno == 0
}
