package securefile

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
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
	lstatPath              = os.Lstat
	openTokenFile          = openNoFollow
	getWorkingDir          = os.Getwd
	lookupEnv              = os.LookupEnv
	usesUnixPermissionBits = func() bool {
		return runtime.GOOS != "windows"
	}
)

// Validate ensures path points to a non-symlink regular file with private permissions.
func Validate(path string) error {
	if err := validateParentDirectory(path); err != nil {
		return err
	}

	info, err := lstatPath(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return errors.New("must not be a symlink")
	}
	if !info.Mode().IsRegular() {
		return errors.New("must be a regular file")
	}
	if !hasSecurePermissions(path, info.Mode().Perm()) {
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
	if err := validatePathLeaf(path); err != nil {
		return "", err
	}

	file, err := openTokenFile(path)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = file.Close()
	}()

	info, err := statFile(file)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("must be a regular file")
	}
	if !hasSecurePermissions(path, info.Mode().Perm()) {
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

func validatePathLeaf(path string) error {
	info, err := lstatPath(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return errors.New("must not be a symlink")
	}
	return nil
}

func validateParentDirectory(path string) error {
	directories, err := tokenPathDirectories(path)
	if err != nil {
		return err
	}

	var parentInfo os.FileInfo
	for index, directory := range directories {
		info, err := lstatPath(directory)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			if allowRootAliasSymlink(index, directories) {
				continue
			}
			if index == len(directories)-1 {
				return errors.New("parent directory must not be a symlink")
			}
			return fmt.Errorf("path ancestor %q must not be a symlink", directory)
		}
		if !info.IsDir() {
			if index == len(directories)-1 {
				return errors.New("parent path must be a directory")
			}
			return fmt.Errorf("path ancestor %q must be a directory", directory)
		}
		parentInfo = info
	}

	if usesUnixPermissionBits() && parentInfo.Mode().Perm()&0o022 != 0 {
		return errors.New("parent directory must not be writable by group or other users")
	}
	return nil
}

func allowRootAliasSymlink(index int, directories []string) bool {
	if index != 1 || index == len(directories)-1 || len(directories) < 2 {
		return false
	}

	root := directories[0]
	// macOS commonly exposes stable root aliases such as /var -> /private/var.
	// Keep rejecting user-controlled deeper path components while tolerating
	// those top-level system aliases.
	return root == filepath.Dir(root)
}

func tokenPathDirectories(path string) ([]string, error) {
	parentPath, _ := filepath.Split(path)
	if parentPath == "" {
		parentPath = "."
	}

	volume := filepath.VolumeName(parentPath)
	remainder := strings.TrimPrefix(parentPath, volume)

	current := ""
	if filepath.IsAbs(parentPath) {
		current = volume + string(filepath.Separator)
	} else {
		workingDir, err := getWorkingDir()
		if err != nil {
			return nil, err
		}
		current = workingDir
	}

	directories := []string{current}
	for component := range strings.SplitSeq(remainder, string(filepath.Separator)) {
		switch component {
		case "", ".":
			continue
		case "..":
			current = filepath.Dir(current)
		default:
			current = filepath.Join(current, component)
		}
		if directories[len(directories)-1] == current {
			continue
		}
		directories = append(directories, current)
	}

	return directories, nil
}

func hasSecurePermissions(path string, perm os.FileMode) bool {
	if !usesUnixPermissionBits() {
		return true
	}

	if perm&0o077 == 0 {
		return true
	}

	// systemd credentials may surface with an ACL-derived mask that makes the
	// file appear group-readable (for example 0440) even though access remains
	// restricted to the service identity and root.
	if isSystemdCredentialPath(path) && perm&0o337 == 0 {
		return true
	}

	return false
}

func isSystemdCredentialPath(path string) bool {
	dir, ok := lookupEnv("CREDENTIALS_DIRECTORY")
	if !ok || strings.TrimSpace(dir) == "" {
		return false
	}

	cleanDir := filepath.Clean(dir)
	if filepath.Base(filepath.Dir(cleanDir)) != "credentials" {
		return false
	}

	cleanPath := filepath.Clean(path)
	if cleanPath == cleanDir {
		return false
	}

	rel, err := filepath.Rel(cleanDir, cleanPath)
	if err != nil {
		return false
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	return true
}
