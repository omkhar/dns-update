//go:build unix

package securefile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenNoFollowUnix(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	regularPath := filepath.Join(dir, "cloudflare.token")
	if err := os.WriteFile(regularPath, []byte("secret"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	t.Run("opens regular file", func(t *testing.T) {
		t.Parallel()

		file, err := openNoFollow(regularPath)
		if err != nil {
			t.Fatalf("openNoFollow() error = %v", err)
		}
		if file == nil {
			t.Fatal("openNoFollow() file = nil, want non-nil")
		}
		_ = file.Close()
	})

	t.Run("rejects symlink", func(t *testing.T) {
		t.Parallel()

		symlinkPath := filepath.Join(dir, "cloudflare.link")
		if err := os.Symlink(regularPath, symlinkPath); err != nil {
			t.Fatalf("Symlink() error = %v", err)
		}

		if _, err := openNoFollow(symlinkPath); err == nil {
			t.Fatal("openNoFollow() error = nil, want symlink error")
		} else if !strings.Contains(err.Error(), "must not be a symlink") {
			t.Fatalf("openNoFollow() error = %v, want symlink error", err)
		}
	})

	t.Run("missing file", func(t *testing.T) {
		t.Parallel()

		if _, err := openNoFollow(filepath.Join(dir, "missing.token")); err == nil {
			t.Fatal("openNoFollow() error = nil, want missing-file error")
		} else if !strings.Contains(err.Error(), "open file") {
			t.Fatalf("openNoFollow() error = %v, want wrapped open error", err)
		}
	})
}
