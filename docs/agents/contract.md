---
kind: contract
slug: dns-update
title: dns-update repo contract
summary: Keep changes simple, correct, tested, secure, public-repo-safe, and in sync with generated docs; keep PRs human-reviewable, and run release or packaging checks only when the change touches those paths.
---
- Keep changes in scope, preserve unrelated work, and keep PRs human-reviewable; CI rejects changes above `35` changed files or `3000` total added plus deleted lines.
- Regenerate agent docs from `docs/agents/**`, never hand-edit generated projections, and prefer the smallest correct idiomatic change with focused tests; use public-repo-safe wording only, and update docs plus release or packaging checks only when those behaviors or paths are touched; normal code changes still require `go test ./...`, `go vet ./...`, and `go build ./...`.
