package securefile

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const maxTokenBytes = 4096

var (
	statFile = func(file *os.File) (os.FileInfo, error) {
		return file.Stat()
	}
	readTokenBytes = func(file *os.File) ([]byte, error) {
		return io.ReadAll(io.LimitReader(file, maxTokenBytes+1))
	}
)

// Validate ensures path points to a non-symlink regular file with private permissions.
func Validate(path string) error {
	if err := validateParentDirectory(path); err != nil {
		return err
	}

	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return errors.New("must not be a symlink")
	}
	if !info.Mode().IsRegular() {
		return errors.New("must be a regular file")
	}
	if info.Mode().Perm()&0o077 != 0 {
		return errors.New("must not be accessible by group or other users")
	}
	return nil
}

// ReadSingleToken opens path without following symlinks when supported, then
// validates and returns exactly one trimmed token.
func ReadSingleToken(path string) (string, error) {
	if err := validateParentDirectory(path); err != nil {
		return "", err
	}

	file, err := openNoFollow(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	info, err := statFile(file)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("must be a regular file")
	}
	if info.Mode().Perm()&0o077 != 0 {
		return "", errors.New("must not be accessible by group or other users")
	}

	data, err := readTokenBytes(file)
	if err != nil {
		return "", err
	}
	if len(data) > maxTokenBytes {
		return "", fmt.Errorf("must be %d bytes or smaller", maxTokenBytes)
	}

	token := strings.TrimSpace(string(data))
	switch {
	case token == "":
		return "", errors.New("must not be empty")
	case strings.ContainsAny(token, " \t\r\n"):
		return "", errors.New("must contain exactly one token")
	default:
		return token, nil
	}
}

func validateParentDirectory(path string) error {
	parent := filepath.Dir(path)
	info, err := os.Lstat(parent)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return errors.New("parent directory must not be a symlink")
	}
	if !info.IsDir() {
		return errors.New("parent path must be a directory")
	}
	if info.Mode().Perm()&0o022 != 0 {
		return errors.New("parent directory must not be writable by group or other users")
	}
	return nil
}
