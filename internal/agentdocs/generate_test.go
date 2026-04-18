package agentdocs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteAndCheck(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "docs", "agents", "contract.md"), strings.TrimLeft(`---
kind: contract
slug: dns-update
title: dns-update repo contract
summary: Keep changes simple, correct, tested, secure, public-repo-safe, and in sync with generated docs; keep PRs human-reviewable, and run release or packaging checks only when the change touches those paths.
---

# dns-update repository contract

This repository keeps its agent-facing docs small, tracked, and generated from docs/agents.

## Priorities

1. Simplicity
2. Correctness
`, "\n"))
	mustWriteFile(t, filepath.Join(root, "docs", "agents", "skills", "dns-update-change-gate.md"), strings.TrimLeft(`---
kind: skill
slug: dns-update-change-gate
title: dns-update change gate
summary: Validate a change for correctness, safety, and reviewability before merge.
---

# dns-update change gate

Use this playbook before a change lands.

## Playbook

1. Read the diff and restate the behavior change in one sentence.
2. Confirm the edit stays inside the requested scope and keeps the PR small enough to review cleanly.
3. Decide whether the change touches behavior, docs, security, or release/package paths.
4. Regenerate the agent docs with go run ./cmd/agentdocgen whenever docs/agents/** changes.
5. Run the smallest tests that prove the change, then widen coverage only if the risk justifies it.
6. Review the change for correctness, idiomatic Go or shell, performance, and public-repo-safe wording.
7. If behavior, security, or release-facing behavior changed, update the related docs in the same change.
8. Skip release and packaging checks unless the change actually touches those paths.

## Stop conditions

- Split the change if review becomes hard.
- Call out security, correctness, or generated-file drift before merge.
`, "\n"))
	mustWriteFile(t, filepath.Join(root, "docs", "agents", "skills", "dns-update-release-gate.md"), strings.TrimLeft(`---
kind: skill
slug: dns-update-release-gate
title: dns-update release gate
summary: Validate release-ready changes, generated docs, and package-impacting paths when relevant.
---

# dns-update release gate

Use this playbook when a change is headed for release or merge.

## Playbook

1. Confirm the canonical source and all generated projections are in sync.
2. Verify the change is still small enough to review without guesswork.
3. Regenerate the agent docs with go run ./cmd/agentdocgen whenever docs/agents/** changes.
4. Run the relevant tests for the touched code paths.
5. Run release or packaging checks only when the change touches release, packaging, deploy, scheduler, or install paths.
6. Leave the tree ready for review, tag, or release handoff.

## Stop conditions

- Pause if a release check fails or if the change needs follow-up outside the current scope.
`, "\n"))

	mismatches, err := Check(root)
	if err != ErrOutOfDate {
		t.Fatalf("Check() error = %v, want %v", err, ErrOutOfDate)
	}
	if len(mismatches) != 9 {
		t.Fatalf("Check() mismatches = %d, want 9", len(mismatches))
	}

	if err := Write(root); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	mismatches, err = Check(root)
	if err != nil {
		t.Fatalf("Check() error = %v, want nil", err)
	}
	if len(mismatches) != 0 {
		t.Fatalf("Check() mismatches = %d, want 0", len(mismatches))
	}

	want := []string{
		"AGENTS.md",
		"CLAUDE.md",
		"GEMINI.md",
		filepath.Join(".agents", "skills", "dns-update-change-gate", "SKILL.md"),
		filepath.Join(".claude", "skills", "dns-update-change-gate", "SKILL.md"),
		filepath.Join(".gemini", "commands", "dns-update", "change-gate.toml"),
		filepath.Join(".agents", "skills", "dns-update-release-gate", "SKILL.md"),
		filepath.Join(".claude", "skills", "dns-update-release-gate", "SKILL.md"),
		filepath.Join(".gemini", "commands", "dns-update", "release-gate.toml"),
	}
	for _, rel := range want {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("generated file %s missing: %v", rel, err)
		}
	}

	agentsRoot, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		t.Fatalf("ReadFile(AGENTS.md) = %v", err)
	}
	if !strings.Contains(string(agentsRoot), "`$dns-update-change-gate`") {
		t.Fatalf("AGENTS.md = %q, want Codex skill reference", string(agentsRoot))
	}

	claudeRoot, err := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("ReadFile(CLAUDE.md) = %v", err)
	}
	if !strings.Contains(string(claudeRoot), "`/dns-update-change-gate`") {
		t.Fatalf("CLAUDE.md = %q, want Claude skill reference", string(claudeRoot))
	}

	geminiRoot, err := os.ReadFile(filepath.Join(root, "GEMINI.md"))
	if err != nil {
		t.Fatalf("ReadFile(GEMINI.md) = %v", err)
	}
	if !strings.Contains(string(geminiRoot), "`/dns-update:change-gate`") {
		t.Fatalf("GEMINI.md = %q, want Gemini command reference", string(geminiRoot))
	}

	gemini, err := os.ReadFile(filepath.Join(root, ".gemini", "commands", "dns-update", "change-gate.toml"))
	if err != nil {
		t.Fatalf("ReadFile(gemini) = %v", err)
	}
	if !strings.Contains(string(gemini), "prompt = '''") {
		t.Fatalf("Gemini output = %q, want TOML prompt", string(gemini))
	}
	if !strings.Contains(string(gemini), "description = \"Validate a change for correctness, safety, and reviewability before merge.\"") {
		t.Fatalf("Gemini output = %q, want description", string(gemini))
	}
	if !strings.Contains(string(gemini), "# dns-update change gate") {
		t.Fatalf("Gemini output = %q, want prompt body", string(gemini))
	}

	skillDoc, err := os.ReadFile(filepath.Join(root, ".agents", "skills", "dns-update-change-gate", "SKILL.md"))
	if err != nil {
		t.Fatalf("ReadFile(skill) = %v", err)
	}
	if !strings.Contains(string(skillDoc), "name: dns-update-change-gate") {
		t.Fatalf("generated skill missing frontmatter:\n%s", skillDoc)
	}

	stalePath := filepath.Join(root, ".gemini", "commands", "dns-update-change-gate.toml")
	mustWriteFile(t, stalePath, "stale\n")
	if err := Write(root); err != nil {
		t.Fatalf("Write() error with stale file = %v", err)
	}
	if _, err := os.Stat(stalePath); !os.IsNotExist(err) {
		t.Fatalf("stale file %s still present; stat error = %v", stalePath, err)
	}
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
