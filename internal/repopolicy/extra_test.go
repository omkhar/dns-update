package repopolicy

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
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

func TestCheckRejectsTrackedSymlinks(t *testing.T) {
	root := t.TempDir()
	gitRun(t, root, "init")

	target := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(target, []byte(joinFragments("/Us", "ers/", "alice/", "git/", "private-repo/\n")), 0o644); err != nil {
		t.Fatalf("WriteFile(target) = %v", err)
	}
	link := filepath.Join(root, "README.link")
	if err := os.Symlink(target, link); err != nil {
		if os.IsPermission(err) {
			t.Skipf("Symlink(README.link) error = %v", err)
		}
		t.Fatalf("Symlink(README.link) error = %v", err)
	}
	gitRun(t, root, "add", "README.link")

	findings, err := Check(root)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("len(findings) = %d, want 1", len(findings))
	}
	if got, want := findings[0].Path, "README.link"; got != want {
		t.Fatalf("findings[0].Path = %q, want %q", got, want)
	}
}

func TestCheckReturnsLstatErrorForTrackedFile(t *testing.T) {
	root := t.TempDir()
	gitRun(t, root, "init")
	writeTestFile(t, root, "README.md", "tracked\n")
	gitRun(t, root, "add", "README.md")

	target := filepath.Join(root, "README.md")
	injected := errors.New("lstat blocked")
	withLstatFile(t, func(path string) (os.FileInfo, error) {
		if path == target {
			return nil, injected
		}
		return os.Lstat(path)
	})

	if _, err := Check(root); !errors.Is(err, injected) {
		t.Fatalf("Check() error = %v, want %v", err, injected)
	}
}

func TestCheckSkipsTrackedFileDeletedAfterLstat(t *testing.T) {
	root := t.TempDir()
	gitRun(t, root, "init")
	writeTestFile(t, root, "README.md", "tracked\n")
	gitRun(t, root, "add", "README.md")

	target := filepath.Join(root, "README.md")
	withReadFile(t, func(path string) ([]byte, error) {
		if path == target {
			return nil, os.ErrNotExist
		}
		return os.ReadFile(path)
	})

	findings, err := Check(root)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("findings = %v, want empty", findings)
	}
}

func TestCheckReturnsReadErrorForTrackedFile(t *testing.T) {
	root := t.TempDir()
	gitRun(t, root, "init")
	writeTestFile(t, root, "README.md", "tracked\n")
	gitRun(t, root, "add", "README.md")

	target := filepath.Join(root, "README.md")
	injected := errors.New("read blocked")
	withReadFile(t, func(path string) ([]byte, error) {
		if path == target {
			return nil, injected
		}
		return os.ReadFile(path)
	})

	if _, err := Check(root); !errors.Is(err, injected) {
		t.Fatalf("Check() error = %v, want %v", err, injected)
	}
}

func TestCheckReturnsProjectFilesError(t *testing.T) {
	root := filepath.Join(t.TempDir(), "missing")
	if _, err := Check(root); err == nil {
		t.Fatal("Check() error = nil, want project files error")
	}
}

func TestCheckSkipsTrackedDirectories(t *testing.T) {
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

	findings, err := Check(root)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("findings = %v, want empty", findings)
	}
}

func TestCheckSortsFindingsByPathThenMessage(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "coverage/report.txt", "detritus\n")
	writeTestFile(t, root, "README.md", joinFragments("/Us", "ers/", "alice/", "src/", "private-repo/\n", "work", "cell", "\n"))

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

func TestCheckFindsWindowsHomePathAndKnownPrivateRepo(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "README.md", joinFragments("C", ":\\", "Us", "ers\\", "Alice Smith\\", "My Projects\\", "repo\n", "cloudflare-", "site-platform-", "ts", "\n"))

	findings, err := Check(root)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if len(findings) != 2 {
		t.Fatalf("len(findings) = %d, want 2", len(findings))
	}
}

func TestCheckFindsWindowsTempRootPath(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "README.md", joinFragments("C", ":\\", "a\n"))

	findings, err := Check(root)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("len(findings) = %d, want 1", len(findings))
	}
}

func TestCheckFindsMacPrivateTempPath(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "README.md", joinFragments("/private", "/tmp\n"))

	findings, err := Check(root)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("len(findings) = %d, want 1", len(findings))
	}
}

func TestCheckFindsMacVarFoldersPath(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "README.md", joinFragments("/var", "/folders/", "ab/cdefghijklmnopqrstuvwxyz/T/dns-update-proof.txt\n"))

	findings, err := Check(root)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("len(findings) = %d, want 1", len(findings))
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
	root := t.TempDir()
	injected := errors.New("walk blocked")
	previous := walkProjectDir
	walkProjectDir = func(path string, fn fs.WalkDirFunc) error { return fn(filepath.Join(path, "blocked"), nil, injected) }
	t.Cleanup(func() { walkProjectDir = previous })

	if _, err := projectFiles(root); !errors.Is(err, injected) {
		t.Fatalf("projectFiles() error = %v, want %v", err, injected)
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

func withLstatFile(t *testing.T, fn func(string) (os.FileInfo, error)) {
	t.Helper()

	previous := lstatFile
	lstatFile = fn
	t.Cleanup(func() {
		lstatFile = previous
	})
}

func withReadFile(t *testing.T, fn func(string) ([]byte, error)) {
	t.Helper()

	previous := readFile
	readFile = fn
	t.Cleanup(func() {
		readFile = previous
	})
}
