//go:build !windows

package securefile

var validatePlatformFilePermissions = func(string) error {
	return nil
}

var validatePlatformParentDirectoryPermissions = func(string) error {
	return nil
}
