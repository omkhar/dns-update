package agentdocs

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadMissingContract(t *testing.T) {
	_, err := Load(t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "contract.md") {
		t.Fatalf("Load() error = %v, want missing contract error", err)
	}
}

func TestLoadRejectsSymlinkRoot(t *testing.T) {
	root := t.TempDir()
	writeCanonicalTree(t, root)
	link := filepath.Join(t.TempDir(), "repo-link")
	mustSymlink(t, root, link)

	if _, err := Load(link); err == nil || !strings.Contains(err.Error(), "must not be a symlink") {
		t.Fatalf("Load(symlink root) error = %v, want symlink-root error", err)
	}
}

func TestLoadRejectsMalformedSkill(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "docs", "agents", "contract.md"), strings.TrimLeft(`---
kind: contract
slug: dns-update
title: dns-update repo contract
summary: Keep changes simple.
---

# dns-update repo contract
`, "\n"))
	mustWriteFile(t, filepath.Join(root, "docs", "agents", "skills", "bad.md"), "not front matter\n")

	_, err := Load(root)
	if err == nil || !strings.Contains(err.Error(), "missing front matter start marker") {
		t.Fatalf("Load() error = %v, want malformed skill error", err)
	}
}

func TestLoadRejectsNonRegularCanonicalSource(t *testing.T) {
	root := t.TempDir()
	writeCanonicalTree(t, root)
	target := filepath.Join(root, "target.md")
	mustWriteFile(t, target, "target\n")
	if err := os.Remove(filepath.Join(root, "docs", "agents", "contract.md")); err != nil {
		t.Fatalf("Remove(contract.md) = %v", err)
	}
	mustSymlink(t, target, filepath.Join(root, "docs", "agents", "contract.md"))

	if _, err := Load(root); err == nil || !strings.Contains(err.Error(), "regular file") {
		t.Fatalf("Load() error = %v, want regular-file error", err)
	}
}

func TestLoadReturnsReadError(t *testing.T) {
	root := t.TempDir()
	writeCanonicalTree(t, root)
	contractPath := filepath.Join(root, "docs", "agents", "contract.md")
	injected := errors.New("read blocked")
	withReadSourceFile(t, func(path string) ([]byte, error) {
		if path == contractPath {
			return nil, injected
		}
		return os.ReadFile(path)
	})

	if _, err := Load(root); !errors.Is(err, injected) || !strings.Contains(err.Error(), "read") {
		t.Fatalf("Load() error = %v, want read error wrapping %v", err, injected)
	}
}

func TestParseDocumentAdditionalFailures(t *testing.T) {
	tests := []struct {
		name string
		doc  string
		want string
	}{
		{
			name: "missing end marker",
			doc: strings.TrimLeft(`---
kind: contract
slug: dns-update
title: dns-update repo contract
`, "\n"),
			want: "missing front matter end marker",
		},
		{
			name: "malformed line",
			doc: strings.TrimLeft(`---
kind: contract
slug
title: dns-update repo contract
---
`, "\n"),
			want: "malformed front matter line",
		},
		{
			name: "missing kind",
			doc: strings.TrimLeft(`---
slug: dns-update
title: dns-update repo contract
---
`, "\n"),
			want: "missing kind",
		},
		{
			name: "missing slug",
			doc: strings.TrimLeft(`---
kind: contract
title: dns-update repo contract
---
`, "\n"),
			want: "missing slug",
		},
		{
			name: "invalid slug",
			doc: strings.TrimLeft(`---
kind: skill
slug: ../escape
title: dns-update change gate
summary: Validate a change.
---
`, "\n"),
			want: "invalid slug",
		},
		{
			name: "blank line in front matter",
			doc: strings.TrimLeft(`---
kind: contract

slug: dns-update
title: dns-update repo contract
---
`, "\n"),
			want: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseDocument("docs/agents/test.md", []byte(tc.doc))
			if tc.want == "" {
				if err != nil {
					t.Fatalf("ParseDocument() error = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("ParseDocument() error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestSplitSourcesFailures(t *testing.T) {
	if _, _, err := splitSources([]Source{{Kind: KindSkill, Slug: "dns-update-change-gate"}}); err == nil || !strings.Contains(err.Error(), "missing contract document") {
		t.Fatalf("splitSources(missing contract) error = %v", err)
	}

	if _, _, err := splitSources([]Source{
		{Kind: KindContract, Path: "docs/agents/contract.md"},
		{Kind: KindContract, Path: "docs/agents/other-contract.md"},
	}); err == nil || !strings.Contains(err.Error(), "multiple contract documents found") {
		t.Fatalf("splitSources(duplicate contract) error = %v", err)
	}

	if _, _, err := splitSources([]Source{{Kind: Kind("widget"), Path: "docs/agents/widget.md"}}); err == nil || !strings.Contains(err.Error(), "unsupported kind") {
		t.Fatalf("splitSources(unsupported kind) error = %v", err)
	}
}

func TestPlanRejectsDuplicateGeneratedSkillPaths(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "docs", "agents", "contract.md"), strings.TrimLeft(`---
kind: contract
slug: dns-update
title: dns-update repo contract
summary: Keep changes simple.
---

# dns-update repo contract
`, "\n"))
	duplicateSkill := strings.TrimLeft(`---
kind: skill
slug: dns-update-change-gate
title: dns-update change gate
summary: Validate a change.
---

# dns-update change gate
`, "\n")
	mustWriteFile(t, filepath.Join(root, "docs", "agents", "skills", "a.md"), duplicateSkill)
	mustWriteFile(t, filepath.Join(root, "docs", "agents", "skills", "b.md"), duplicateSkill)

	_, err := Plan(root)
	if err == nil || !strings.Contains(err.Error(), "duplicate generated path") {
		t.Fatalf("Plan() error = %v, want duplicate path error", err)
	}
}

func TestPlanAndCheckReturnLoadErrors(t *testing.T) {
	root := t.TempDir()

	if _, err := Plan(root); err == nil || !strings.Contains(err.Error(), "contract.md") {
		t.Fatalf("Plan() error = %v, want missing contract error", err)
	}
	if _, err := Check(root); err == nil || !strings.Contains(err.Error(), "contract.md") {
		t.Fatalf("Check() error = %v, want missing contract error", err)
	}
	if err := Write(root); err == nil || !strings.Contains(err.Error(), "contract.md") {
		t.Fatalf("Write() error = %v, want missing contract error", err)
	}
}

func TestPlanReturnsSplitSourceError(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "docs", "agents", "contract.md"), strings.TrimLeft(`---
kind: skill
slug: dns-update-change-gate
title: dns-update change gate
summary: Validate a change.
---

# dns-update change gate
`, "\n"))

	if _, err := Plan(root); err == nil || !strings.Contains(err.Error(), "missing contract document") {
		t.Fatalf("Plan() error = %v, want split source error", err)
	}
}

func TestWriteFailsWhenRemovingStalePathFails(t *testing.T) {
	root := t.TempDir()
	writeCanonicalTree(t, root)
	staleFile := filepath.Join(root, ".gemini", "commands", "stale.toml")
	if err := os.MkdirAll(filepath.Dir(staleFile), 0o755); err != nil {
		t.Fatalf("MkdirAll(staleFile dir) = %v", err)
	}
	if err := os.WriteFile(staleFile, []byte("stale\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(staleFile) = %v", err)
	}
	injected := errors.New("remove blocked")
	withRemovePath(t, func(path string) error {
		if path == staleFile {
			return injected
		}
		return os.Remove(path)
	})

	err := Write(root)
	if !errors.Is(err, injected) || !strings.Contains(err.Error(), "remove stale") {
		t.Fatalf("Write() error = %v, want stale removal error wrapping %v", err, injected)
	}
}

func TestWriteFailsWhenManagedPathsCannotBeListed(t *testing.T) {
	root := t.TempDir()
	writeCanonicalTree(t, root)
	if err := os.WriteFile(filepath.Join(root, ".agents"), []byte("file\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(.agents) = %v", err)
	}

	if err := Write(root); err == nil {
		t.Fatal("Write() error = nil, want managed path error")
	}
}

func TestWriteFailsWhenParentDirectoryCannotBeCreated(t *testing.T) {
	root := t.TempDir()
	writeCanonicalTree(t, root)
	blockedDir := filepath.Join(root, ".agents", "skills", "dns-update-change-gate")
	injected := errors.New("mkdir blocked")
	withMakeDirs(t, func(path string, perm os.FileMode) error {
		if path == blockedDir {
			return injected
		}
		return os.MkdirAll(path, perm)
	})

	err := Write(root)
	if !errors.Is(err, injected) {
		t.Fatalf("Write() error = %v, want wrapped %v", err, injected)
	}
}

func TestWriteFailsWhenOutputPathIsDirectory(t *testing.T) {
	root := t.TempDir()
	writeCanonicalTree(t, root)
	if err := os.Mkdir(filepath.Join(root, "AGENTS.md"), 0o755); err != nil {
		t.Fatalf("Mkdir(AGENTS.md) = %v", err)
	}

	err := Write(root)
	if err == nil || !strings.Contains(err.Error(), "AGENTS.md") {
		t.Fatalf("Write() error = %v, want file write error", err)
	}
}

func TestWriteFailsWhenExpectedOutputIsSymlink(t *testing.T) {
	root := t.TempDir()
	writeCanonicalTree(t, root)
	withLstatPath(t, func(path string) (os.FileInfo, error) {
		if path == filepath.Join(root, "AGENTS.md") {
			return fakeFileInfo{mode: os.ModeSymlink}, nil
		}
		return os.Lstat(path)
	})

	if err := Write(root); err == nil || !strings.Contains(err.Error(), "traverses symlink") {
		t.Fatalf("Write() error = %v, want symlink traversal error", err)
	}
}

func TestWriteFailsWhenManagedPathTraversesSymlink(t *testing.T) {
	root := t.TempDir()
	writeCanonicalTree(t, root)
	symlinkPath := filepath.Join(root, ".agents")
	if err := os.WriteFile(symlinkPath, []byte("stale\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(.agents) = %v", err)
	}
	removed := false
	withRemovePath(t, func(path string) error {
		if path == symlinkPath {
			removed = true
		}
		return os.Remove(path)
	})
	withLstatPath(t, func(path string) (os.FileInfo, error) {
		if path == symlinkPath && !removed {
			return fakeFileInfo{mode: os.ModeSymlink}, nil
		}
		return os.Lstat(path)
	})

	if err := Write(root); err != nil {
		t.Fatalf("Write() error = %v, want stale symlink replacement", err)
	}
	if info, err := os.Lstat(filepath.Join(root, ".agents")); err != nil {
		t.Fatalf("Lstat(.agents) = %v", err)
	} else if info.Mode()&os.ModeSymlink != 0 {
		t.Fatal(".agents remained a symlink after Write()")
	}
	if _, err := os.Stat(filepath.Join(root, ".agents", "skills", "dns-update-change-gate", "SKILL.md")); err != nil {
		t.Fatalf("generated skill missing after Write(): %v", err)
	}
}

func TestCheckReadErrorAndHelpers(t *testing.T) {
	root := t.TempDir()
	writeCanonicalTree(t, root)
	if err := Write(root); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if err := os.Remove(filepath.Join(root, "AGENTS.md")); err != nil {
		t.Fatalf("Remove(AGENTS.md) = %v", err)
	}
	if err := os.Mkdir(filepath.Join(root, "AGENTS.md"), 0o755); err != nil {
		t.Fatalf("Mkdir(AGENTS.md) = %v", err)
	}

	if _, err := Check(root); err == nil || !strings.Contains(err.Error(), "AGENTS.md") {
		t.Fatalf("Check() error = %v, want read error", err)
	}

	if got, want := FormatMismatch(Mismatch{Path: "foo", Stale: true}), "foo is stale and should be removed"; got != want {
		t.Fatalf("FormatMismatch(stale) = %q, want %q", got, want)
	}
	if got, want := FormatMismatch(Mismatch{Path: "foo", Invalid: true}), "foo is invalid and should be replaced"; got != want {
		t.Fatalf("FormatMismatch(invalid) = %q, want %q", got, want)
	}
	if got, want := FormatMismatch(Mismatch{Path: "foo", Missing: true}), "foo is missing"; got != want {
		t.Fatalf("FormatMismatch(missing) = %q, want %q", got, want)
	}
	if got, want := FormatMismatch(Mismatch{Path: "foo"}), "foo is out of date"; got != want {
		t.Fatalf("FormatMismatch(outdated) = %q, want %q", got, want)
	}
	if got := Summary(nil); got != "" {
		t.Fatalf("Summary(nil) = %q, want empty", got)
	}
	if got, want := Summary([]Mismatch{{Path: "foo", Missing: true}, {Path: "bar"}}), "foo is missing\nbar is out of date"; got != want {
		t.Fatalf("Summary(nonempty) = %q, want %q", got, want)
	}
	if got := skillInvocation("dns-update-change-gate", rootTarget("other")); got != "dns-update-change-gate" {
		t.Fatalf("skillInvocation(default) = %q", got)
	}
	if got := geminiCommandLeaf("custom-skill"); got != "custom-skill" {
		t.Fatalf("geminiCommandLeaf(non-prefixed) = %q", got)
	}
	if got := stripLeadingHeading("plain body"); got != "plain body" {
		t.Fatalf("stripLeadingHeading(no heading) = %q", got)
	}
	if got := stripLeadingHeading("# only heading"); got != "" {
		t.Fatalf("stripLeadingHeading(heading only) = %q, want empty", got)
	}
	if got := geminiCommandLeaf("dns-update-"); got != "dns-update-" {
		t.Fatalf("geminiCommandLeaf(empty suffix) = %q", got)
	}
	contract := Source{
		Path:    "docs/agents/contract.md",
		Kind:    KindContract,
		Slug:    "dns-update",
		Title:   "dns-update repo contract",
		Summary: "Keep changes simple.",
		Body:    "# dns-update repo contract\n",
	}
	outputs := RenderContract(contract, nil)
	if len(outputs) != 3 {
		t.Fatalf("RenderContract(nil skills) outputs = %d, want 3", len(outputs))
	}
	if strings.Contains(outputs[0].Content, "Repo Skills") {
		t.Fatalf("RenderContract(nil skills) should omit helper section:\n%s", outputs[0].Content)
	}
	outputs = RenderContract(contract, []Source{{Slug: "dns-update-change-gate", Summary: "summary"}})
	if !strings.Contains(outputs[0].Content, "`$dns-update-change-gate`") {
		t.Fatalf("RenderContract(codex skills) = %q, want codex invocation", outputs[0].Content)
	}
	if !strings.Contains(outputs[1].Content, "`/dns-update-change-gate`") {
		t.Fatalf("RenderContract(claude skills) = %q, want claude invocation", outputs[1].Content)
	}
	if !strings.Contains(outputs[2].Content, "`/dns-update:change-gate`") {
		t.Fatalf("RenderContract(gemini commands) = %q, want gemini invocation", outputs[2].Content)
	}
	var b strings.Builder
	renderRootHelpers(&b, nil, rootTargetCodex)
	if b.Len() != 0 {
		t.Fatalf("renderRootHelpers(nil skills) = %q, want empty", b.String())
	}
	renderRootHelpers(&b, []Source{{Slug: "dns-update-change-gate", Summary: "summary"}}, rootTarget("bogus"))
	if b.Len() != 0 {
		t.Fatalf("renderRootHelpers(default target) = %q, want empty", b.String())
	}
}

func TestCheckReturnsLstatErrorForBlockedOutput(t *testing.T) {
	root := t.TempDir()
	blockedPath := filepath.Join(root, "blocked", "file.txt")
	injected := errors.New("lstat blocked")
	withLstatPath(t, func(path string) (os.FileInfo, error) {
		if path == blockedPath {
			return nil, injected
		}
		return os.Lstat(path)
	})

	if _, _, _, err := readManagedOutput(blockedPath); !errors.Is(err, injected) || !strings.Contains(err.Error(), "read") {
		t.Fatalf("readManagedOutput() error = %v, want lstat read error wrapping %v", err, injected)
	}
}

func TestAppendIfRegular(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "AGENTS.md"), "content\n")

	paths := appendIfFile(nil, root, "AGENTS.md")
	if len(paths) != 1 || paths[0] != "AGENTS.md" {
		t.Fatalf("appendIfFile(existing) = %v", paths)
	}
	if paths := appendIfFile(nil, root, "missing.md"); len(paths) != 0 {
		t.Fatalf("appendIfFile(missing) = %v, want empty", paths)
	}
	mustSymlink(t, "AGENTS.md", filepath.Join(root, "AGENTS.link"))
	if paths := appendIfFile(nil, root, "AGENTS.link"); len(paths) != 1 || paths[0] != "AGENTS.link" {
		t.Fatalf("appendIfFile(symlink) = %v, want symlink path", paths)
	}
}

func TestManagedPathsErrors(t *testing.T) {
	root := t.TempDir()
	badDir := filepath.Join(root, ".agents", "skills", "blocked")
	if err := os.MkdirAll(badDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(badDir) = %v", err)
	}
	injected := errors.New("walk blocked")
	managedRoot := filepath.Join(root, ".agents", "skills")
	withWalkDir(t, func(root string, fn fs.WalkDirFunc) error {
		if root == managedRoot {
			return fn(root, nil, injected)
		}
		return filepath.WalkDir(root, fn)
	})

	if _, err := managedPaths(root); !errors.Is(err, injected) {
		t.Fatalf("managedPaths() error = %v, want %v", err, injected)
	}
}

func TestCheckReportsStaleAndOutOfDateFiles(t *testing.T) {
	root := t.TempDir()
	writeCanonicalTree(t, root)
	if err := Write(root); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	mustWriteFile(t, filepath.Join(root, ".gemini", "commands", "stale.toml"), "stale\n")
	mustWriteFile(t, filepath.Join(root, "AGENTS.md"), "drifted\n")

	mismatches, err := Check(root)
	if err != ErrOutOfDate {
		t.Fatalf("Check() error = %v, want %v", err, ErrOutOfDate)
	}
	if len(mismatches) < 2 {
		t.Fatalf("len(mismatches) = %d, want at least 2", len(mismatches))
	}
}

func TestCheckReportsStaleSymlink(t *testing.T) {
	root := t.TempDir()
	writeCanonicalTree(t, root)
	if err := Write(root); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	stalePath := filepath.Join(root, ".agents", "skills", "stale-link")
	mustWriteFile(t, stalePath, "orphan\n")
	withLstatPath(t, func(path string) (os.FileInfo, error) {
		if path == stalePath {
			return fakeFileInfo{mode: os.ModeSymlink}, nil
		}
		return os.Lstat(path)
	})

	mismatches, err := Check(root)
	if err != ErrOutOfDate {
		t.Fatalf("Check() error = %v, want %v", err, ErrOutOfDate)
	}
	found := false
	for _, mismatch := range mismatches {
		if mismatch.Path == ".agents/skills/stale-link" && mismatch.Stale {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("Check() mismatches = %#v, want stale symlink", mismatches)
	}
}

func TestCheckRejectsExpectedSymlink(t *testing.T) {
	root := t.TempDir()
	writeCanonicalTree(t, root)
	if err := Write(root); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	withLstatPath(t, func(path string) (os.FileInfo, error) {
		if path == filepath.Join(root, "AGENTS.md") {
			return fakeFileInfo{mode: os.ModeSymlink}, nil
		}
		return os.Lstat(path)
	})

	mismatches, err := Check(root)
	if err != ErrOutOfDate {
		t.Fatalf("Check() error = %v, want %v", err, ErrOutOfDate)
	}
	for _, mismatch := range mismatches {
		if mismatch.Path == "AGENTS.md" && mismatch.Invalid {
			return
		}
	}
	t.Fatalf("Check() mismatches = %#v, want invalid AGENTS.md", mismatches)
}

func TestRenderSkillTOMLEscapesLiteralDelimiters(t *testing.T) {
	source := Source{
		Path:    "docs/agents/skills/example.md",
		Kind:    KindSkill,
		Slug:    "dns-update-example",
		Title:   "example",
		Summary: "summary",
		Body:    "# heading\nliteral ''' marker\n",
	}

	rendered := renderSkillTOML(source)
	if !strings.Contains(rendered, `prompt = "# heading\nliteral ''' marker\n"`) {
		t.Fatalf("renderSkillTOML() = %q, want escaped quoted prompt", rendered)
	}
}

func TestEnsureSafeWritePath(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "AGENTS.md"), "content\n")
	if err := ensureSafeWritePath(root, "AGENTS.md"); err != nil {
		t.Fatalf("ensureSafeWritePath(existing file) error = %v", err)
	}
	if err := ensureSafeWritePath(root, ".agents/skills/dns-update-change-gate/SKILL.md"); err != nil {
		t.Fatalf("ensureSafeWritePath(missing managed file) error = %v", err)
	}
	if err := ensureSafeWritePath(root, "../escape"); err == nil || !strings.Contains(err.Error(), "escapes repository root") {
		t.Fatalf("ensureSafeWritePath(escape) error = %v, want root escape error", err)
	}
	target := filepath.Join(root, "target.txt")
	mustWriteFile(t, target, "target\n")
	withLstatPath(t, func(path string) (os.FileInfo, error) {
		if path == filepath.Join(root, "AGENTS.md") {
			return fakeFileInfo{mode: os.ModeSymlink}, nil
		}
		return os.Lstat(path)
	})
	if err := ensureSafeWritePath(root, "AGENTS.md"); err == nil || !strings.Contains(err.Error(), "traverses symlink") {
		t.Fatalf("ensureSafeWritePath(symlink) error = %v, want symlink error", err)
	}
}

func TestEnsureSafeWritePathReturnsLstatError(t *testing.T) {
	root := t.TempDir()
	blockedDir := filepath.Join(root, "blocked")
	injected := errors.New("lstat blocked")
	withLstatPath(t, func(path string) (os.FileInfo, error) {
		if path == blockedDir {
			return nil, injected
		}
		return os.Lstat(path)
	})

	if err := ensureSafeWritePath(root, filepath.ToSlash(filepath.Join("blocked", "file.txt"))); !errors.Is(err, injected) || !strings.Contains(err.Error(), "lstat") {
		t.Fatalf("ensureSafeWritePath(blocked) error = %v, want lstat error wrapping %v", err, injected)
	}
}

func TestRejectSymlinkRootReturnsLstatError(t *testing.T) {
	root := filepath.Join(t.TempDir(), "blocked", "repo")
	injected := errors.New("lstat blocked")
	withLstatPath(t, func(path string) (os.FileInfo, error) {
		if path == root {
			return nil, injected
		}
		return os.Lstat(path)
	})

	if err := rejectSymlinkRoot(root); !errors.Is(err, injected) {
		t.Fatalf("rejectSymlinkRoot() error = %v, want %v", err, injected)
	}
}

func TestRejectSymlinkRootAllowsMissingPath(t *testing.T) {
	if err := rejectSymlinkRoot(filepath.Join(t.TempDir(), "missing")); err != nil {
		t.Fatalf("rejectSymlinkRoot(missing) error = %v, want nil", err)
	}
}

func TestManagedPathsReturnsManagedRootSymlink(t *testing.T) {
	root := t.TempDir()
	withLstatPath(t, func(path string) (os.FileInfo, error) {
		if path == filepath.Join(root, ".agents") {
			return fakeFileInfo{mode: os.ModeSymlink}, nil
		}
		return os.Lstat(path)
	})

	paths, err := managedPaths(root)
	if err != nil {
		t.Fatalf("managedPaths() error = %v", err)
	}
	if len(paths) != 1 || paths[0] != ".agents" {
		t.Fatalf("managedPaths() = %v, want [.agents]", paths)
	}
}

func TestManagedPathsRejectsManagedRootFile(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".agents"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.agents) = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".agents", "skills"), []byte("file\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(.agents/skills) = %v", err)
	}

	if _, err := managedPaths(root); err == nil || !strings.Contains(err.Error(), ".agents/skills") {
		t.Fatalf("managedPaths() error = %v, want managed-root file error", err)
	}
}

func TestStatManagedRoot(t *testing.T) {
	root := t.TempDir()
	exists, err := statManagedRoot(filepath.Join(root, "missing"), "missing")
	if err != nil || exists {
		t.Fatalf("statManagedRoot(missing) = (%v, %v), want (false, nil)", exists, err)
	}

	if err := os.MkdirAll(filepath.Join(root, "dir"), 0o755); err != nil {
		t.Fatalf("MkdirAll(dir) = %v", err)
	}
	exists, err = statManagedRoot(filepath.Join(root, "dir"), "dir")
	if err != nil || !exists {
		t.Fatalf("statManagedRoot(dir) = (%v, %v), want (true, nil)", exists, err)
	}

	if err := os.WriteFile(filepath.Join(root, "file"), []byte("file\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(file) = %v", err)
	}
	if _, err := statManagedRoot(filepath.Join(root, "file"), "file"); err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("statManagedRoot(file) error = %v, want not-a-directory error", err)
	}
	blockedPath := filepath.Join(root, "blocked", "dir")
	injected := errors.New("lstat blocked")
	withLstatPath(t, func(path string) (os.FileInfo, error) {
		if path == blockedPath {
			return nil, injected
		}
		return os.Lstat(path)
	})
	if _, err := statManagedRoot(blockedPath, "blocked/dir"); !errors.Is(err, injected) {
		t.Fatalf("statManagedRoot(blocked) error = %v, want %v", err, injected)
	}
}

func TestCheckFailsWhenManagedPathsCannotBeListed(t *testing.T) {
	root := t.TempDir()
	writeCanonicalTree(t, root)
	if err := os.WriteFile(filepath.Join(root, ".agents"), []byte("file\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(.agents) = %v", err)
	}

	if _, err := Check(root); err == nil {
		t.Fatal("Check() error = nil, want managed path error")
	}
}

func mustSymlink(t *testing.T, target, link string) {
	t.Helper()

	if err := os.Symlink(target, link); err != nil {
		if os.IsPermission(err) || strings.Contains(strings.ToLower(err.Error()), "privilege") {
			t.Skipf("Symlink(%s) unavailable: %v", link, err)
		}
		t.Fatalf("Symlink(%s) = %v", link, err)
	}
}

func withLstatPath(t *testing.T, fn func(string) (os.FileInfo, error)) {
	t.Helper()

	previous := lstatPath
	lstatPath = fn
	t.Cleanup(func() {
		lstatPath = previous
	})
}

func withRemovePath(t *testing.T, fn func(string) error) {
	t.Helper()

	previous := removePath
	removePath = fn
	t.Cleanup(func() {
		removePath = previous
	})
}

func withMakeDirs(t *testing.T, fn func(string, os.FileMode) error) {
	t.Helper()

	previous := makeDirs
	makeDirs = fn
	t.Cleanup(func() {
		makeDirs = previous
	})
}

func withWalkDir(t *testing.T, fn func(string, fs.WalkDirFunc) error) {
	t.Helper()

	previous := walkDir
	walkDir = fn
	t.Cleanup(func() {
		walkDir = previous
	})
}

func withReadSourceFile(t *testing.T, fn func(string) ([]byte, error)) {
	t.Helper()

	previous := readSourceFile
	readSourceFile = fn
	t.Cleanup(func() {
		readSourceFile = previous
	})
}

type fakeFileInfo struct {
	mode os.FileMode
}

func (f fakeFileInfo) Name() string       { return "" }
func (f fakeFileInfo) Size() int64        { return 0 }
func (f fakeFileInfo) Mode() os.FileMode  { return f.mode }
func (f fakeFileInfo) ModTime() time.Time { return time.Time{} }
func (f fakeFileInfo) IsDir() bool        { return f.mode.IsDir() }
func (f fakeFileInfo) Sys() any           { return nil }

func writeCanonicalTree(t *testing.T, root string) {
	t.Helper()

	mustWriteFile(t, filepath.Join(root, "docs", "agents", "contract.md"), strings.TrimLeft(`---
kind: contract
slug: dns-update
title: dns-update repo contract
summary: Keep changes simple, correct, tested, secure, public-repo-safe, and in sync with generated docs; keep PRs human-reviewable, and run release or packaging checks only when the change touches those paths.
---

# dns-update repository contract

This repository keeps its agent-facing docs small, tracked, and generated from docs/agents.
`, "\n"))
	mustWriteFile(t, filepath.Join(root, "docs", "agents", "skills", "dns-update-change-gate.md"), strings.TrimLeft(`---
kind: skill
slug: dns-update-change-gate
title: dns-update change gate
summary: Validate a change for correctness, safety, and reviewability before merge.
---

# dns-update change gate

Use this playbook before a change lands.
`, "\n"))
	mustWriteFile(t, filepath.Join(root, "docs", "agents", "skills", "dns-update-release-gate.md"), strings.TrimLeft(`---
kind: skill
slug: dns-update-release-gate
title: dns-update release gate
summary: Validate release-ready changes, generated docs, and package-impacting paths when relevant.
---

# dns-update release gate

Use this playbook when a change is headed for release or merge.
`, "\n"))
}
