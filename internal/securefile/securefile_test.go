package securefile

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestValidate(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	regular := filepath.Join(dir, "secret")
	if err := os.WriteFile(regular, []byte("secret"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := Validate(regular); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	if err := Validate(filepath.Join(dir, "missing")); err == nil {
		t.Fatal("Validate() error = nil, want missing-file error")
	}

	symlinkPath := filepath.Join(dir, "secret.link")
	if err := os.Symlink(regular, symlinkPath); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}
	if err := Validate(symlinkPath); err == nil {
		t.Fatal("Validate() error = nil, want symlink error")
	}

	subdir := filepath.Join(dir, "dir")
	if err := os.Mkdir(subdir, 0o700); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	if err := Validate(subdir); err == nil {
		t.Fatal("Validate() error = nil, want regular-file error")
	}

	insecure := filepath.Join(dir, "insecure")
	if err := os.WriteFile(insecure, []byte("secret"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if runtime.GOOS != "windows" {
		if err := Validate(insecure); err == nil {
			t.Fatal("Validate() error = nil, want permission error")
		}
	}

	writableDir := filepath.Join(dir, "writable")
	if err := os.Mkdir(writableDir, 0o733); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	if err := os.Chmod(writableDir, 0o733); err != nil {
		t.Fatalf("Chmod() error = %v", err)
	}
	writablePath := filepath.Join(writableDir, "secret")
	if err := os.WriteFile(writablePath, []byte("secret"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if runtime.GOOS != "windows" {
		if err := Validate(writablePath); err == nil {
			t.Fatal("Validate() error = nil, want parent-directory permission error")
		}
	}

	targetDir := filepath.Join(dir, "target")
	if err := os.Mkdir(targetDir, 0o700); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	targetPath := filepath.Join(targetDir, "secret")
	if err := os.WriteFile(targetPath, []byte("secret"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	symlinkDir := filepath.Join(dir, "dir.link")
	if err := os.Symlink(targetDir, symlinkDir); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}
	if err := Validate(filepath.Join(symlinkDir, "secret")); err == nil {
		t.Fatal("Validate() error = nil, want symlink-component error")
	}

	ancestorTarget := filepath.Join(dir, "ancestor-target")
	if err := os.MkdirAll(filepath.Join(ancestorTarget, "nested"), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	ancestorPath := filepath.Join(ancestorTarget, "nested", "secret")
	if err := os.WriteFile(ancestorPath, []byte("secret"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	ancestorLink := filepath.Join(dir, "ancestor.link")
	if err := os.Symlink(ancestorTarget, ancestorLink); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}
	if err := Validate(filepath.Join(ancestorLink, "nested", "secret")); err == nil {
		t.Fatal("Validate() error = nil, want ancestor-symlink error")
	}
}

func TestValidateAcceptsSystemdCredentialPermissionMask(t *testing.T) {
	rootDir := filepath.Join(t.TempDir(), "run", "credentials")
	credDir := filepath.Join(rootDir, "test.service")
	if err := os.MkdirAll(credDir, 0o700); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}

	secretPath := filepath.Join(credDir, "cloudflare.token")
	if err := os.WriteFile(secretPath, []byte("secret"), 0o440); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	t.Setenv("CREDENTIALS_DIRECTORY", credDir)
	if err := Validate(secretPath); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestReadSingleToken(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	secretPath := filepath.Join(dir, "cloudflare.token")
	if err := os.WriteFile(secretPath, []byte("  secret-token \n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	token, err := ReadSingleToken(secretPath)
	if err != nil {
		t.Fatalf("ReadSingleToken() error = %v", err)
	}
	if got, want := token, "secret-token"; got != want {
		t.Fatalf("ReadSingleToken() = %q, want %q", got, want)
	}
}

func TestReadSingleTokenAcceptsSystemdCredentialPermissionMask(t *testing.T) {
	rootDir := filepath.Join(t.TempDir(), "run", "credentials")
	credDir := filepath.Join(rootDir, "test.service")
	if err := os.MkdirAll(credDir, 0o700); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}

	secretPath := filepath.Join(credDir, "cloudflare.token")
	if err := os.WriteFile(secretPath, []byte("secret\n"), 0o440); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	t.Setenv("CREDENTIALS_DIRECTORY", credDir)

	token, err := ReadSingleToken(secretPath)
	if err != nil {
		t.Fatalf("ReadSingleToken() error = %v", err)
	}
	if got, want := token, "secret"; got != want {
		t.Fatalf("ReadSingleToken() = %q, want %q", got, want)
	}
}

func TestValidateAndReadSingleTokenEnforceUnixPermissionBits(t *testing.T) {
	originalUsesUnixPermissionBits := usesUnixPermissionBits
	t.Cleanup(func() {
		usesUnixPermissionBits = originalUsesUnixPermissionBits
	})
	usesUnixPermissionBits = func() bool { return true }

	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "cloudflare.token")
	if err := os.WriteFile(tokenPath, []byte("secret\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if err := Validate(tokenPath); err == nil {
		t.Fatal("Validate() error = nil, want permission error")
	}
	if _, err := ReadSingleToken(tokenPath); err == nil {
		t.Fatal("ReadSingleToken() error = nil, want permission error")
	}
}

func TestValidateEnforcesFilePermissionBitsWithSecureParent(t *testing.T) {
	originalUsesUnixPermissionBits := usesUnixPermissionBits
	originalGetWorkingDir := getWorkingDir
	originalLstatPath := lstatPath
	t.Cleanup(func() {
		usesUnixPermissionBits = originalUsesUnixPermissionBits
		getWorkingDir = originalGetWorkingDir
		lstatPath = originalLstatPath
	})

	usesUnixPermissionBits = func() bool { return true }

	workingDir := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(workingDir, 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	getWorkingDir = func() (string, error) {
		return workingDir, nil
	}

	lstatPath = func(path string) (os.FileInfo, error) {
		switch path {
		case workingDir:
			return testFileInfo{name: filepath.Base(workingDir), mode: os.ModeDir | 0o700}, nil
		case "cloudflare.token":
			return testFileInfo{name: "cloudflare.token", mode: 0o644}, nil
		default:
			return nil, os.ErrNotExist
		}
	}

	err := Validate("cloudflare.token")
	if err == nil || !strings.Contains(err.Error(), "group or other users") {
		t.Fatalf("Validate() error = %v, want file permission error", err)
	}
}

func TestReadSingleTokenEnforcesFilePermissionBitsWithSecureParent(t *testing.T) {
	originalUsesUnixPermissionBits := usesUnixPermissionBits
	originalGetWorkingDir := getWorkingDir
	originalLstatPath := lstatPath
	originalOpenTokenFile := openTokenFile
	originalStatFile := statFile
	t.Cleanup(func() {
		usesUnixPermissionBits = originalUsesUnixPermissionBits
		getWorkingDir = originalGetWorkingDir
		lstatPath = originalLstatPath
		openTokenFile = originalOpenTokenFile
		statFile = originalStatFile
	})

	usesUnixPermissionBits = func() bool { return true }

	workingDir := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(workingDir, 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	realPath := filepath.Join(workingDir, "real.token")
	if err := os.WriteFile(realPath, []byte("secret\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	getWorkingDir = func() (string, error) {
		return workingDir, nil
	}

	lstatPath = func(path string) (os.FileInfo, error) {
		switch path {
		case workingDir:
			return testFileInfo{name: filepath.Base(workingDir), mode: os.ModeDir | 0o700}, nil
		case "cloudflare.token":
			return testFileInfo{name: "cloudflare.token", mode: 0o600}, nil
		default:
			return nil, os.ErrNotExist
		}
	}
	openTokenFile = func(path string) (*os.File, error) {
		if path != "cloudflare.token" {
			return nil, os.ErrNotExist
		}
		return os.Open(realPath)
	}
	statFile = func(*os.File) (os.FileInfo, error) {
		return testFileInfo{name: "cloudflare.token", mode: 0o644}, nil
	}

	_, err := ReadSingleToken("cloudflare.token")
	if err == nil || !strings.Contains(err.Error(), "group or other users") {
		t.Fatalf("ReadSingleToken() error = %v, want file permission error", err)
	}
}

func TestValidateAndReadSingleTokenLeafLstatError(t *testing.T) {
	originalLstatPath := lstatPath
	t.Cleanup(func() {
		lstatPath = originalLstatPath
	})

	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "cloudflare.token")
	lstatPath = func(path string) (os.FileInfo, error) {
		if path == tokenPath {
			return nil, errors.New("boom")
		}
		return originalLstatPath(path)
	}

	if err := Validate(tokenPath); err == nil {
		t.Fatal("Validate() error = nil, want leaf lstat error")
	}
	if _, err := ReadSingleToken(tokenPath); err == nil {
		t.Fatal("ReadSingleToken() error = nil, want leaf lstat error")
	}
}

func TestReadSingleTokenErrors(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	missing := filepath.Join(dir, "missing")
	if _, err := ReadSingleToken(missing); err == nil {
		t.Fatal("ReadSingleToken() error = nil, want missing-file error")
	}

	empty := filepath.Join(dir, "empty.token")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := ReadSingleToken(empty); err == nil {
		t.Fatal("ReadSingleToken() error = nil, want empty-token error")
	}

	multi := filepath.Join(dir, "multi.token")
	if err := os.WriteFile(multi, []byte("token one"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := ReadSingleToken(multi); err == nil {
		t.Fatal("ReadSingleToken() error = nil, want multi-token error")
	}

	insecure := filepath.Join(dir, "insecure.token")
	if err := os.WriteFile(insecure, []byte("secret"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if runtime.GOOS != "windows" {
		if _, err := ReadSingleToken(insecure); err == nil {
			t.Fatal("ReadSingleToken() error = nil, want permission error")
		}
	}

	subdir := filepath.Join(dir, "subdir")
	if err := os.Mkdir(subdir, 0o700); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	if _, err := ReadSingleToken(subdir); err == nil {
		t.Fatal("ReadSingleToken() error = nil, want regular-file error")
	}

	oversized := filepath.Join(dir, "oversized.token")
	if err := os.WriteFile(oversized, []byte(strings.Repeat("a", maxTokenBytes+1)), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := ReadSingleToken(oversized); err == nil {
		t.Fatal("ReadSingleToken() error = nil, want oversized-token error")
	}

	symlinkTarget := filepath.Join(dir, "target.token")
	if err := os.WriteFile(symlinkTarget, []byte("secret"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	symlinkPath := filepath.Join(dir, "symlink.token")
	if err := os.Symlink(symlinkTarget, symlinkPath); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}
	if _, err := ReadSingleToken(symlinkPath); err == nil {
		t.Fatal("ReadSingleToken() error = nil, want symlink error")
	}

	writableDir := filepath.Join(dir, "writable")
	if err := os.Mkdir(writableDir, 0o733); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	if err := os.Chmod(writableDir, 0o733); err != nil {
		t.Fatalf("Chmod() error = %v", err)
	}
	writablePath := filepath.Join(writableDir, "cloudflare.token")
	if err := os.WriteFile(writablePath, []byte("secret"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if runtime.GOOS != "windows" {
		if _, err := ReadSingleToken(writablePath); err == nil {
			t.Fatal("ReadSingleToken() error = nil, want parent-directory permission error")
		}
	}

	targetDir := filepath.Join(dir, "target.dir")
	if err := os.Mkdir(targetDir, 0o700); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	targetFile := filepath.Join(targetDir, "cloudflare.token")
	if err := os.WriteFile(targetFile, []byte("secret"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	symlinkDir := filepath.Join(dir, "dir.link")
	if err := os.Symlink(targetDir, symlinkDir); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}
	if _, err := ReadSingleToken(filepath.Join(symlinkDir, "cloudflare.token")); err == nil {
		t.Fatal("ReadSingleToken() error = nil, want symlink-component error")
	}

	ancestorTarget := filepath.Join(dir, "ancestor-target")
	if err := os.MkdirAll(filepath.Join(ancestorTarget, "nested"), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	ancestorToken := filepath.Join(ancestorTarget, "nested", "cloudflare.token")
	if err := os.WriteFile(ancestorToken, []byte("secret"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	ancestorLink := filepath.Join(dir, "ancestor.link")
	if err := os.Symlink(ancestorTarget, ancestorLink); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}
	if _, err := ReadSingleToken(filepath.Join(ancestorLink, "nested", "cloudflare.token")); err == nil {
		t.Fatal("ReadSingleToken() error = nil, want ancestor-symlink error")
	}
}

func TestReadSingleTokenInternalErrors(t *testing.T) {
	originalStatFile := statFile
	originalReadTokenBytes := readTokenBytes
	originalLstatPath := lstatPath
	originalOpenTokenFile := openTokenFile
	originalGetWorkingDir := getWorkingDir
	originalLookupEnv := lookupEnv
	t.Cleanup(func() {
		statFile = originalStatFile
		readTokenBytes = originalReadTokenBytes
		lstatPath = originalLstatPath
		openTokenFile = originalOpenTokenFile
		getWorkingDir = originalGetWorkingDir
		lookupEnv = originalLookupEnv
	})

	dir := t.TempDir()
	secretPath := filepath.Join(dir, "cloudflare.token")
	if err := os.WriteFile(secretPath, []byte("secret"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	lstatPath = func(string) (os.FileInfo, error) {
		return nil, errors.New("boom")
	}
	if _, err := ReadSingleToken(secretPath); err == nil {
		t.Fatal("ReadSingleToken() error = nil, want lstat error")
	}

	lstatPath = originalLstatPath
	openTokenFile = func(string) (*os.File, error) {
		return nil, errors.New("boom")
	}
	if _, err := ReadSingleToken(secretPath); err == nil {
		t.Fatal("ReadSingleToken() error = nil, want open error")
	}

	openTokenFile = originalOpenTokenFile
	statFile = func(*os.File) (os.FileInfo, error) {
		return nil, errors.New("boom")
	}
	if _, err := ReadSingleToken(secretPath); err == nil {
		t.Fatal("ReadSingleToken() error = nil, want stat error")
	}

	statFile = originalStatFile
	readTokenBytes = func(*os.File) ([]byte, error) {
		return nil, errors.New("boom")
	}
	if _, err := ReadSingleToken(secretPath); err == nil {
		t.Fatal("ReadSingleToken() error = nil, want read error")
	}

	getWorkingDir = func() (string, error) {
		return "", errors.New("boom")
	}
	if _, err := ReadSingleToken("cloudflare.token"); err == nil {
		t.Fatal("ReadSingleToken() error = nil, want getwd error")
	}
}

func TestValidateParentDirectory(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	secureDir := filepath.Join(dir, "secure")
	if err := os.Mkdir(secureDir, 0o700); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	if err := validateParentDirectory(filepath.Join(secureDir, "secret")); err != nil {
		t.Fatalf("validateParentDirectory() error = %v", err)
	}

	missingParentPath := filepath.Join(dir, "missing", "secret")
	if err := validateParentDirectory(missingParentPath); err == nil {
		t.Fatal("validateParentDirectory() error = nil, want missing-parent error")
	}

	parentFile := filepath.Join(dir, "parent.file")
	if err := os.WriteFile(parentFile, []byte("secret"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := validateParentDirectory(filepath.Join(parentFile, "secret")); err == nil {
		t.Fatal("validateParentDirectory() error = nil, want non-directory parent error")
	}

	ancestorFile := filepath.Join(dir, "ancestor.file")
	if err := os.WriteFile(ancestorFile, []byte("secret"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := validateParentDirectory(filepath.Join(ancestorFile, "nested", "secret")); err == nil {
		t.Fatal("validateParentDirectory() error = nil, want non-directory ancestor error")
	}

	targetDir := filepath.Join(dir, "target")
	if err := os.MkdirAll(filepath.Join(targetDir, "nested"), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	ancestorLink := filepath.Join(dir, "ancestor.link")
	if err := os.Symlink(targetDir, ancestorLink); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}
	if err := validateParentDirectory(filepath.Join(ancestorLink, "nested", "secret")); err == nil {
		t.Fatal("validateParentDirectory() error = nil, want ancestor-symlink error")
	}
}

func TestValidateParentDirectoryAllowsRootAliasSymlink(t *testing.T) {
	originalLstatPath := lstatPath
	t.Cleanup(func() {
		lstatPath = originalLstatPath
	})

	root := testRootPath(t)
	aliasDir := filepath.Join(root, "var")
	parentDir := filepath.Join(aliasDir, "tmp")
	targetPath := filepath.Join(parentDir, "secret")

	lstatPath = func(path string) (os.FileInfo, error) {
		switch path {
		case root:
			return testFileInfo{name: root, mode: os.ModeDir | 0o755}, nil
		case aliasDir:
			return testFileInfo{name: filepath.Base(aliasDir), mode: os.ModeSymlink}, nil
		case parentDir:
			return testFileInfo{name: filepath.Base(parentDir), mode: os.ModeDir | 0o700}, nil
		default:
			return nil, os.ErrNotExist
		}
	}

	if err := validateParentDirectory(targetPath); err != nil {
		t.Fatalf("validateParentDirectory() error = %v", err)
	}
}

func TestValidateParentDirectoryEnforcesUnixPermissionBits(t *testing.T) {
	originalUsesUnixPermissionBits := usesUnixPermissionBits
	t.Cleanup(func() {
		usesUnixPermissionBits = originalUsesUnixPermissionBits
	})
	usesUnixPermissionBits = func() bool { return true }

	dir := t.TempDir()
	writableDir := filepath.Join(dir, "writable")
	if err := os.Mkdir(writableDir, 0o733); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	if err := os.Chmod(writableDir, 0o733); err != nil {
		t.Fatalf("Chmod() error = %v", err)
	}

	if err := validateParentDirectory(filepath.Join(writableDir, "secret")); err == nil {
		t.Fatal("validateParentDirectory() error = nil, want permission error")
	}
}

func TestAllowRootAliasSymlink(t *testing.T) {
	t.Parallel()

	root := testRootPath(t)

	if !allowRootAliasSymlink(1, []string{root, filepath.Join(root, "var"), filepath.Join(root, "var", "tmp")}) {
		t.Fatal("allowRootAliasSymlink() = false, want true for root-level alias ancestor")
	}
	if allowRootAliasSymlink(1, []string{root, filepath.Join(root, "var")}) {
		t.Fatal("allowRootAliasSymlink() = true, want false when alias is the direct parent")
	}
	if allowRootAliasSymlink(2, []string{root, filepath.Join(root, "var"), filepath.Join(root, "var", "tmp")}) {
		t.Fatal("allowRootAliasSymlink() = true, want false for deeper components")
	}
}

func TestTokenPathDirectories(t *testing.T) {
	originalGetWorkingDir := getWorkingDir
	t.Cleanup(func() {
		getWorkingDir = originalGetWorkingDir
	})

	workingDir := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(workingDir, 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	getWorkingDir = func() (string, error) {
		return workingDir, nil
	}

	directories, err := tokenPathDirectories("cloudflare.token")
	if err != nil {
		t.Fatalf("tokenPathDirectories(relative leaf) error = %v", err)
	}
	if want := []string{workingDir}; !reflect.DeepEqual(directories, want) {
		t.Fatalf("tokenPathDirectories(relative leaf) = %v, want %v", directories, want)
	}

	rawRelativePath := "child" + string(filepath.Separator) + ".." + string(filepath.Separator) + "nested" + string(filepath.Separator) + "cloudflare.token"
	directories, err = tokenPathDirectories(rawRelativePath)
	if err != nil {
		t.Fatalf("tokenPathDirectories(relative path) error = %v", err)
	}
	wantRelative := []string{
		workingDir,
		filepath.Join(workingDir, "child"),
		workingDir,
		filepath.Join(workingDir, "nested"),
	}
	if !reflect.DeepEqual(directories, wantRelative) {
		t.Fatalf("tokenPathDirectories(relative path) = %v, want %v", directories, wantRelative)
	}

	absoluteDir := filepath.Join(t.TempDir(), "absolute", "nested")
	if err := os.MkdirAll(absoluteDir, 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	absolutePath := filepath.Join(absoluteDir, "cloudflare.token")
	directories, err = tokenPathDirectories(absolutePath)
	if err != nil {
		t.Fatalf("tokenPathDirectories(absolute path) error = %v", err)
	}
	if got, want := directories[0], filepath.VolumeName(absolutePath)+string(filepath.Separator); got != want {
		t.Fatalf("tokenPathDirectories(absolute path) root = %q, want %q", got, want)
	}
	if got, want := directories[len(directories)-1], absoluteDir; got != want {
		t.Fatalf("tokenPathDirectories(absolute path) last = %q, want %q", got, want)
	}

	root := filepath.VolumeName(absolutePath) + string(filepath.Separator)
	rawPath := root + ".." + string(filepath.Separator) + "tmp" + string(filepath.Separator) + "cloudflare.token"
	directories, err = tokenPathDirectories(rawPath)
	if err != nil {
		t.Fatalf("tokenPathDirectories(root traversal) error = %v", err)
	}
	if got, want := directories, []string{root, filepath.Join(root, "tmp")}; !reflect.DeepEqual(got, want) {
		t.Fatalf("tokenPathDirectories(root traversal) = %v, want %v", got, want)
	}
}

func TestIsSystemdCredentialPath(t *testing.T) {
	originalLookupEnv := lookupEnv
	t.Cleanup(func() {
		lookupEnv = originalLookupEnv
	})

	lookupEnv = func(key string) (string, bool) {
		if key != "CREDENTIALS_DIRECTORY" {
			return "", false
		}
		return "/run/credentials/test.service", true
	}

	if !isSystemdCredentialPath("/run/credentials/test.service/cloudflare.token") {
		t.Fatal("isSystemdCredentialPath() = false, want true for child path")
	}
	if isSystemdCredentialPath("/run/credentials/test.service") {
		t.Fatal("isSystemdCredentialPath() = true, want false for directory root")
	}
	if isSystemdCredentialPath("/run/credentials/other.service/cloudflare.token") {
		t.Fatal("isSystemdCredentialPath() = true, want false for sibling path")
	}
	if isSystemdCredentialPath("/run/credentials/test.service/../other.service/cloudflare.token") {
		t.Fatal("isSystemdCredentialPath() = true, want false for escaped path")
	}

	lookupEnv = func(key string) (string, bool) {
		return "", false
	}
	if isSystemdCredentialPath("/run/credentials/test.service/cloudflare.token") {
		t.Fatal("isSystemdCredentialPath() = true, want false when CREDENTIALS_DIRECTORY is unset")
	}

	lookupEnv = func(key string) (string, bool) {
		if key != "CREDENTIALS_DIRECTORY" {
			return "", false
		}
		return "   ", true
	}
	if isSystemdCredentialPath("/run/credentials/test.service/cloudflare.token") {
		t.Fatal("isSystemdCredentialPath() = true, want false when CREDENTIALS_DIRECTORY is blank")
	}

	lookupEnv = func(key string) (string, bool) {
		if key != "CREDENTIALS_DIRECTORY" {
			return "", false
		}
		return "/tmp/test.service", true
	}
	if isSystemdCredentialPath("/tmp/test.service/cloudflare.token") {
		t.Fatal("isSystemdCredentialPath() = true, want false outside /run/credentials")
	}

	lookupEnv = func(key string) (string, bool) {
		if key != "CREDENTIALS_DIRECTORY" {
			return "", false
		}
		return "run/credentials/test.service", true
	}
	if isSystemdCredentialPath("/run/credentials/test.service/cloudflare.token") {
		t.Fatal("isSystemdCredentialPath() = true, want false for relative/absolute mismatch")
	}
}

type testFileInfo struct {
	name string
	mode os.FileMode
}

func (i testFileInfo) Name() string       { return i.name }
func (i testFileInfo) Size() int64        { return 0 }
func (i testFileInfo) Mode() os.FileMode  { return i.mode }
func (i testFileInfo) ModTime() time.Time { return time.Time{} }
func (i testFileInfo) IsDir() bool        { return i.mode.IsDir() }
func (i testFileInfo) Sys() any           { return nil }

func TestHasSecurePermissions(t *testing.T) {
	originalUsesUnixPermissionBits := usesUnixPermissionBits
	originalLookupEnv := lookupEnv
	t.Cleanup(func() {
		usesUnixPermissionBits = originalUsesUnixPermissionBits
		lookupEnv = originalLookupEnv
	})

	usesUnixPermissionBits = func() bool { return true }

	if !hasSecurePermissions("cloudflare.token", 0o600) {
		t.Fatal("hasSecurePermissions() = false, want private permissions to pass")
	}

	credDir := filepath.Join(testRootPath(t), "run", "credentials", "test.service")
	lookupEnv = func(key string) (string, bool) {
		if key == "CREDENTIALS_DIRECTORY" {
			return credDir, true
		}
		return "", false
	}
	if !hasSecurePermissions(filepath.Join(credDir, "cloudflare.token"), 0o440) {
		t.Fatal("hasSecurePermissions() = false, want systemd credential mask to pass")
	}
	if hasSecurePermissions("cloudflare.token", 0o644) {
		t.Fatal("hasSecurePermissions() = true, want group-readable permissions to fail")
	}

	usesUnixPermissionBits = func() bool { return false }
	if !hasSecurePermissions("cloudflare.token", 0o644) {
		t.Fatal("hasSecurePermissions() = false, want disabled Unix-bit checks to skip mode enforcement")
	}
}

func TestValidateReturnsPlatformFilePermissionError(t *testing.T) {
	originalValidatePlatformFilePermissions := validatePlatformFilePermissions
	t.Cleanup(func() {
		validatePlatformFilePermissions = originalValidatePlatformFilePermissions
	})

	validatePlatformFilePermissions = func(string) error {
		return errors.New("platform file ACL error")
	}

	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "cloudflare.token")
	if err := os.WriteFile(tokenPath, []byte("secret"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	err := Validate(tokenPath)
	if err == nil || !strings.Contains(err.Error(), "platform file ACL error") {
		t.Fatalf("Validate() error = %v, want platform file ACL error", err)
	}
}

func TestReadSingleTokenReturnsPlatformFilePermissionError(t *testing.T) {
	originalValidatePlatformFilePermissions := validatePlatformFilePermissions
	t.Cleanup(func() {
		validatePlatformFilePermissions = originalValidatePlatformFilePermissions
	})

	validatePlatformFilePermissions = func(string) error {
		return errors.New("platform file ACL error")
	}

	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "cloudflare.token")
	if err := os.WriteFile(tokenPath, []byte("secret\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if _, err := ReadSingleToken(tokenPath); err == nil || !strings.Contains(err.Error(), "platform file ACL error") {
		t.Fatalf("ReadSingleToken() error = %v, want platform file ACL error", err)
	}
}

func TestValidateParentDirectoryReturnsPlatformPermissionError(t *testing.T) {
	originalValidatePlatformParentDirectoryPermissions := validatePlatformParentDirectoryPermissions
	t.Cleanup(func() {
		validatePlatformParentDirectoryPermissions = originalValidatePlatformParentDirectoryPermissions
	})

	validatePlatformParentDirectoryPermissions = func(string) error {
		return errors.New("platform directory ACL error")
	}

	dir := t.TempDir()
	if err := validateParentDirectory(filepath.Join(dir, "cloudflare.token")); err == nil || !strings.Contains(err.Error(), "platform directory ACL error") {
		t.Fatalf("validateParentDirectory() error = %v, want platform directory ACL error", err)
	}
}

func testRootPath(t *testing.T) string {
	t.Helper()

	if runtime.GOOS == "windows" {
		volume := filepath.VolumeName(t.TempDir())
		if volume == "" {
			t.Fatal("filepath.VolumeName() = empty, want drive root")
		}
		return volume + string(filepath.Separator)
	}

	return string(filepath.Separator)
}
