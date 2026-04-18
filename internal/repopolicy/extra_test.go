package repopolicy

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"testing"
)

func TestCheckSkipsDeletedTrackedFiles(t *testing.T) {
	root := t.TempDir()
	gitRun(t, root, "init")
	writeTestFile(t, root, "security_best_practices_report.md", "# report\n")
	gitRun(t, root, "add", "security_best_practices_report.md")
	if err := os.Remove(filepath.Join(root, "security_best_practices_report.md")); err != nil {
		t.Fatalf("Remove(deleted tracked file) = %v", err)
	}

	findings, err := Check(root)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("findings = %v, want empty", findings)
	}
}

func TestCheckUsesTrackedFilesAndIgnoresBinaryContent(t *testing.T) {
	root := t.TempDir()
	gitRun(t, root, "init")
	writeTestFile(t, root, "coverage/report.txt", "detritus\n")
	writeTestFile(t, root, "binary.dat", string([]byte{'a', 0, 'b'}))
	gitRun(t, root, "add", "coverage/report.txt", "binary.dat")

	findings, err := Check(root)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("len(findings) = %d, want 1", len(findings))
	}
	if got, want := findings[0].Path, "coverage/report.txt"; got != want {
		t.Fatalf("findings[0].Path = %q, want %q", got, want)
	}
}

func TestCheckReturnsProjectFilesError(t *testing.T) {
	root := filepath.Join(t.TempDir(), "missing")
	if _, err := Check(root); err == nil {
		t.Fatal("Check() error = nil, want project files error")
	}
}

func TestCheckReturnsReadErrorForTrackedDirectory(t *testing.T) {
	root := t.TempDir()
	gitRun(t, root, "init")
	writeTestFile(t, root, "README.md", "tracked\n")
	gitRun(t, root, "add", "README.md")
	if err := os.Remove(filepath.Join(root, "README.md")); err != nil {
		t.Fatalf("Remove(README.md) = %v", err)
	}
	if err := os.Mkdir(filepath.Join(root, "README.md"), 0o755); err != nil {
		t.Fatalf("Mkdir(README.md) = %v", err)
	}

	if _, err := Check(root); err == nil {
		t.Fatal("Check() error = nil, want read error")
	}
}

func TestCheckSortsFindingsByPathThenMessage(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "coverage/report.txt", "detritus\n")
	writeTestFile(t, root, "README.md", blockedContentRules[0].needle+blockedContentRules[2].needle)

	findings, err := Check(root)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if len(findings) != 3 {
		t.Fatalf("len(findings) = %d, want 3", len(findings))
	}
	if findings[0].Path != "README.md" || findings[1].Path != "README.md" || findings[2].Path != "coverage/report.txt" {
		t.Fatalf("findings paths = %+v", findings)
	}
	if findings[0].Message > findings[1].Message {
		t.Fatalf("same-path findings not sorted by message: %+v", findings[:2])
	}
}

func TestProjectFilesFallbackSkipsGitDirectory(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, ".git/config", "ignored\n")
	writeTestFile(t, root, "README.md", "tracked\n")
	if err := os.Symlink("README.md", filepath.Join(root, "README.link")); err != nil && !os.IsPermission(err) {
		t.Fatalf("Symlink() error = %v", err)
	}

	files, err := projectFiles(root)
	if err != nil {
		t.Fatalf("projectFiles() error = %v", err)
	}
	sort.Strings(files)
	if got, want := files, []string{"README.md"}; len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("projectFiles() = %v, want %v", got, want)
	}
}

func TestProjectFilesReturnsTrackedFilesErrorForInvalidRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "missing")
	if _, err := projectFiles(root); err == nil {
		t.Fatal("projectFiles() error = nil, want error")
	}
}

func TestProjectFilesFallbackReturnsWalkError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission-based walk errors are platform-specific")
	}

	root := t.TempDir()
	blockedDir := filepath.Join(root, "blocked")
	if err := os.MkdirAll(blockedDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(blockedDir) = %v", err)
	}
	if err := os.Chmod(blockedDir, 0); err != nil {
		t.Fatalf("Chmod(blockedDir) = %v", err)
	}
	defer os.Chmod(blockedDir, 0o755)

	if _, err := projectFiles(root); err == nil {
		t.Fatal("projectFiles() error = nil, want walk error")
	}
}

func TestHelpers(t *testing.T) {
	if !isBinary([]byte{'a', 0, 'b'}) {
		t.Fatal("isBinary() = false, want true")
	}
	if isBinary([]byte("text")) {
		t.Fatal("isBinary() = true, want false")
	}
	if got, want := normalizeNewlines("a\r\nb"), "a\nb"; got != want {
		t.Fatalf("normalizeNewlines() = %q, want %q", got, want)
	}
	if got, want := joinFragments("a", "b", "c"), "abc"; got != want {
		t.Fatalf("joinFragments() = %q, want %q", got, want)
	}
}

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v error = %v\n%s", args, err, bytes.TrimSpace(output))
	}
}
