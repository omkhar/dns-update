package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunWritesAndChecksArtifacts(t *testing.T) {
	root := t.TempDir()
	writeCanonicalSources(t, root)

	stderr := new(bytes.Buffer)
	if got, want := run([]string{"-root", root}, stderr), 0; got != want {
		t.Fatalf("run(write) exit code = %d, want %d, stderr = %q", got, want, stderr.String())
	}

	if _, err := os.Stat(filepath.Join(root, "AGENTS.md")); err != nil {
		t.Fatalf("generated AGENTS.md missing: %v", err)
	}

	stderr.Reset()
	if got, want := run([]string{"-root", root, "-check"}, stderr), 0; got != want {
		t.Fatalf("run(check) exit code = %d, want %d, stderr = %q", got, want, stderr.String())
	}
}

func TestRunCheckFailsWhenArtifactsAreMissing(t *testing.T) {
	root := t.TempDir()
	writeCanonicalSources(t, root)

	stderr := new(bytes.Buffer)
	if got, want := run([]string{"-root", root, "-check"}, stderr), 1; got != want {
		t.Fatalf("run(check missing) exit code = %d, want %d", got, want)
	}
	if !strings.Contains(stderr.String(), "AGENTS.md is missing") {
		t.Fatalf("stderr = %q, want missing artifact summary", stderr.String())
	}
}

func TestRunRejectsUnknownFlag(t *testing.T) {
	stderr := new(bytes.Buffer)
	if got, want := run([]string{"-nope"}, stderr), 2; got != want {
		t.Fatalf("run(unknown flag) exit code = %d, want %d", got, want)
	}
}

func TestRunFailsWhenRootIsMissing(t *testing.T) {
	root := filepath.Join(t.TempDir(), "missing")

	stderr := new(bytes.Buffer)
	if got, want := run([]string{"-root", root}, stderr), 1; got != want {
		t.Fatalf("run(write missing root) exit code = %d, want %d", got, want)
	}
	if !strings.Contains(stderr.String(), "contract.md") {
		t.Fatalf("stderr = %q, want missing contract error", stderr.String())
	}

	stderr.Reset()
	if got, want := run([]string{"-root", root, "-check"}, stderr), 1; got != want {
		t.Fatalf("run(check missing root) exit code = %d, want %d", got, want)
	}
	if !strings.Contains(stderr.String(), "contract.md") {
		t.Fatalf("stderr = %q, want missing contract error", stderr.String())
	}
}

func TestMainCallsExitWithRunResult(t *testing.T) {
	root := t.TempDir()
	writeCanonicalSources(t, root)

	oldArgs := os.Args
	oldExit := exit
	defer func() {
		os.Args = oldArgs
		exit = oldExit
	}()

	var gotCode int
	exit = func(code int) {
		gotCode = code
		panic("exit")
	}
	os.Args = []string{"agentdocgen", "-root", root}

	defer func() {
		if recovered := recover(); recovered == nil {
			t.Fatal("main() did not call exit")
		}
		if got, want := gotCode, 0; got != want {
			t.Fatalf("exit code = %d, want %d", got, want)
		}
	}()

	main()
}

func writeCanonicalSources(t *testing.T, root string) {
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

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) = %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) = %v", path, err)
	}
}
