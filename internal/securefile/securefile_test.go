package securefile

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	if err := Validate(insecure); err == nil {
		t.Fatal("Validate() error = nil, want permission error")
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
	if err := Validate(writablePath); err == nil {
		t.Fatal("Validate() error = nil, want parent-directory permission error")
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
	if _, err := ReadSingleToken(insecure); err == nil {
		t.Fatal("ReadSingleToken() error = nil, want permission error")
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
	if _, err := ReadSingleToken(writablePath); err == nil {
		t.Fatal("ReadSingleToken() error = nil, want parent-directory permission error")
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
}

func TestReadSingleTokenInternalErrors(t *testing.T) {
	originalStatFile := statFile
	originalReadTokenBytes := readTokenBytes
	t.Cleanup(func() {
		statFile = originalStatFile
		readTokenBytes = originalReadTokenBytes
	})

	dir := t.TempDir()
	secretPath := filepath.Join(dir, "cloudflare.token")
	if err := os.WriteFile(secretPath, []byte("secret"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

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
}
