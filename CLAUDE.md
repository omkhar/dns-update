<!-- Generated from docs/agents/contract.md; do not edit directly. -->

# dns-update repo contract

Keep changes simple, correct, tested, secure, public-repo-safe, and in sync with generated docs; keep PRs human-reviewable, and run release or packaging checks only when the change touches those paths.

This repository keeps its agent-facing docs small, tracked, and generated from `docs/agents`.

## Priorities

1. Simplicity
2. Correctness
3. Clean validation
4. Appropriate tests
5. Security
6. Performance
7. Idiomatic Go and shell
8. Human-reviewable PR size and complexity

## Repository invariants

- Keep changes within the requested scope and preserve unrelated work already present in the tree.
- Keep pull requests small enough for a human reviewer to reason about the full change. Split unrelated work or large complexity jumps into separate PRs.
- Pull request CI rejects changes above `35` changed files or `3000` total added plus deleted lines.
- Keep generated outputs in sync with the canonical source files and never hand-edit generated projections.
- Prefer the smallest correct change that is still easy to review.
- Use idiomatic Go and shell, with focused tests that cleanly validate behavior.
- Protect correctness, security, and performance.
- Use public-repo-safe wording only. Do not commit private analysis, evidence bundles, local absolute paths, usernames, or references to non-public repositories or workflows.
- Update docs when behavior, security, scheduler, packaging, or release-facing behavior changes.
- `go test ./...`, `go vet ./...`, and `go build ./...` are required for normal code changes.
- Run release and packaging checks only when the change touches release, packaging, deploy, scheduler, or install paths.

## Project Skills

Invoke these project skills when the task matches:

- `/dns-update-change-gate`: Validate a change for correctness, safety, and reviewability before merge.
- `/dns-update-release-gate`: Validate release-ready changes, generated docs, and package-impacting paths when relevant.
