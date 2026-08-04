//go:build unix

package codeberg

import (
	"os"
	"syscall"
)

// openProtectedTokenFile opens path without following its final symlink.
// O_NONBLOCK prevents a malicious special file from blocking open before the
// caller can validate the opened descriptor with Stat.
func openProtectedTokenFile(path string) (*os.File, error) {
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), path), nil
}
