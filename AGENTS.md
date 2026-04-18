<!-- Generated from docs/agents/contract.md; do not edit directly. -->

# dns-update repo contract

Keep changes simple, correct, tested, secure, public-repo-safe, and in sync with generated docs; keep PRs human-reviewable, and run release or packaging checks only when the change touches those paths.

- Keep changes within the requested scope, preserve unrelated work already present in the tree, and keep PRs small enough for a human reviewer to reason about the full change.
- Pull request CI rejects changes above `35` changed files or `3000` total added plus deleted lines.
- Keep generated outputs in sync with the canonical source files and never hand-edit generated projections.
- Prefer the smallest correct change that is still easy to review, using idiomatic Go and shell with focused tests that validate behavior and protect correctness, security, and performance.
- Use public-repo-safe wording only. Do not commit private analysis, evidence bundles, local absolute paths, usernames, or references to non-public repositories or workflows.
- Update docs when behavior, security, scheduler, packaging, or release-facing behavior changes, and run `go test ./...`, `go vet ./...`, and `go build ./...` for normal code changes.
- Run release and packaging checks only when the change touches release, packaging, deploy, scheduler, or install paths.

## Repo Skills

Invoke these repo skills when the task matches:

- `$dns-update-change-gate`: Validate a change for correctness, safety, and reviewability before merge.
- `$dns-update-release-gate`: Validate release-ready changes, generated docs, and package-impacting paths when relevant.
