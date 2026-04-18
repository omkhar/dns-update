package repopolicy

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckFindsBlockedTrackedArtifactPath(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "security_best_practices_report.md", "# report\n")

	findings, err := Check(root)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("len(findings) = %d, want 1", len(findings))
	}
	if findings[0].Path != "security_best_practices_report.md" {
		t.Fatalf("findings[0].Path = %q, want security_best_practices_report.md", findings[0].Path)
	}
}

func TestCheckFindsBlockedContent(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "README.md", "See "+joinFragments("/Us", "ers/", "alice/", "dev/", "private-repo/", "notes.\n"))

	findings, err := Check(root)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("len(findings) = %d, want 1", len(findings))
	}
	if findings[0].Path != "README.md" {
		t.Fatalf("findings[0].Path = %q, want README.md", findings[0].Path)
	}
}

func TestCheckFindsGitCheckoutContent(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "README.md", "See "+joinFragments("/Us", "ers/", "alice/", "git/", "private-repo/", "notes.\n"))

	findings, err := Check(root)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("len(findings) = %d, want 1", len(findings))
	}
	if findings[0].Path != "README.md" {
		t.Fatalf("findings[0].Path = %q, want README.md", findings[0].Path)
	}
}

func writeTestFile(t *testing.T, root, relativePath, content string) {
	t.Helper()

	fullPath := filepath.Join(root, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", fullPath, err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", fullPath, err)
	}
}
