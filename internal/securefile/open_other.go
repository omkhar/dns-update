//go:build !unix

package securefile

import (
	"fmt"
	"os"
)

func openNoFollow(path string) (*os.File, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	return file, nil
}
