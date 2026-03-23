//go:build !unix

package securefile

import (
	"fmt"
	"os"
)

func openNoFollow(path string) (*os.File, error) {
	// Non-Unix platforms rely on ReadSingleToken's path-level Lstat check to
	// reject leaf symlinks before opening the file.
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	return file, nil
}
