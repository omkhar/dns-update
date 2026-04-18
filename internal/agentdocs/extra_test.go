package agentdocs

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestLoadMissingContract(t *testing.T) {
	_, err := Load(t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "contract.md") {
		t.Fatalf("Load() error = %v, want missing contract error", err)
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
	if runtime.GOOS == "windows" {
		t.Skip("permission-based stale removal behavior is platform-specific")
	}

	root := t.TempDir()
	writeCanonicalTree(t, root)
	staleDir := filepath.Join(root, ".gemini", "commands")
	if err := os.MkdirAll(staleDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(staleDir) = %v", err)
	}
	staleFile := filepath.Join(staleDir, "stale.toml")
	if err := os.WriteFile(staleFile, []byte("stale\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(staleFile) = %v", err)
	}
	if err := os.Chmod(staleDir, 0o500); err != nil {
		t.Fatalf("Chmod(staleDir) = %v", err)
	}
	defer os.Chmod(staleDir, 0o755)

	err := Write(root)
	if err == nil || !strings.Contains(err.Error(), "remove stale") {
		t.Fatalf("Write() error = %v, want stale removal error", err)
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
	if runtime.GOOS == "windows" {
		t.Skip("symlink behavior is platform-specific")
	}

	root := t.TempDir()
	writeCanonicalTree(t, root)
	if err := os.MkdirAll(filepath.Join(root, ".agents", "skills"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.agents/skills) = %v", err)
	}
	blocker := filepath.Join(root, "blocker")
	if err := os.WriteFile(blocker, []byte("file\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(blocker) = %v", err)
	}
	if err := os.Symlink(blocker, filepath.Join(root, ".agents", "skills", "dns-update-change-gate")); err != nil {
		t.Fatalf("Symlink(blocker) = %v", err)
	}

	err := Write(root)
	if err == nil || !strings.Contains(err.Error(), ".agents") {
		t.Fatalf("Write() error = %v, want parent directory error", err)
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
	var b strings.Builder
	renderRootHelpers(&b, []Source{{Slug: "dns-update-change-gate", Summary: "summary"}}, rootTarget("bogus"))
	if b.Len() != 0 {
		t.Fatalf("renderRootHelpers(default target) = %q, want empty", b.String())
	}
}

func TestAppendIfRegular(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "AGENTS.md"), "content\n")

	paths := appendIfRegular(nil, root, "AGENTS.md")
	if len(paths) != 1 || paths[0] != "AGENTS.md" {
		t.Fatalf("appendIfRegular(existing) = %v", paths)
	}
	if paths := appendIfRegular(nil, root, "missing.md"); len(paths) != 0 {
		t.Fatalf("appendIfRegular(missing) = %v, want empty", paths)
	}
}

func TestManagedPathsErrors(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission-based walk errors are platform-specific")
	}

	root := t.TempDir()
	badDir := filepath.Join(root, ".agents", "skills", "blocked")
	if err := os.MkdirAll(badDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(badDir) = %v", err)
	}
	if err := os.Chmod(badDir, 0); err != nil {
		t.Fatalf("Chmod(badDir) = %v", err)
	}
	defer os.Chmod(badDir, 0o755)

	if _, err := managedPaths(root); err == nil {
		t.Fatal("managedPaths() error = nil, want walk error")
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
