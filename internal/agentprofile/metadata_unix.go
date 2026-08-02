//go:build unix

package agentprofile

import (
	"fmt"
	"io/fs"
	"os"
	"syscall"
)

func requireCurrentOwner(info fs.FileInfo) error {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return errorsForUnsupportedMetadata("ownership")
	}
	if stat.Uid != uint32(os.Geteuid()) {
		return fmt.Errorf("inode is owned by uid %d, current uid is %d", stat.Uid, os.Geteuid())
	}
	return nil
}

func requireSingleLink(info fs.FileInfo) error {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return errorsForUnsupportedMetadata("link count")
	}
	if stat.Nlink != 1 {
		return fmt.Errorf("inode has %d hard links; want exactly 1", stat.Nlink)
	}
	return nil
}

func errorsForUnsupportedMetadata(name string) error {
	return fmt.Errorf("cannot verify inode %s", name)
}
