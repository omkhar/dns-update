//go:build unix

package securefile

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

func openNoFollow(path string) (*os.File, error) {
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	if err != nil {
		if errors.Is(err, syscall.ELOOP) {
			return nil, errors.New("must not be a symlink")
		}
		return nil, fmt.Errorf("open file: %w", err)
	}
	// #nosec G115 -- syscall.Open returns a non-negative file descriptor on success.
	return os.NewFile(uintptr(fd), path), nil
}
