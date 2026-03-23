//go:build !unix

package securefile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenNoFollowOther(t *testing.T) {
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

	t.Run("missing file", func(t *testing.T) {
		t.Parallel()

		if _, err := openNoFollow(filepath.Join(dir, "missing.token")); err == nil {
			t.Fatal("openNoFollow() error = nil, want missing-file error")
		} else if !strings.Contains(err.Error(), "open file") {
			t.Fatalf("openNoFollow() error = %v, want wrapped open error", err)
		}
	})
}
