---
kind: contract
slug: dns-update
title: dns-update repo contract
summary: Keep each change simple, correct, tested, secure, reviewable, and safe for a public repository.
---
This document uses ASD-STE100 Simplified Technical English.

- Keep each change in scope.
- Preserve unrelated work.
- Keep each pull request easy to review.
- Use no more than `35` changed files.
- Use no more than `3000` total added and deleted lines.
- Edit agent text only in `docs/agents/**`.
- Run `go run ./cmd/agentdocgen` after an agent-text change.
- Do not edit generated agent projections.
- Use the smallest correct idiomatic change.
- Add focused tests.
- Use wording that is safe for a public repository.
- Update documentation when behavior changes.
- Run release and package checks when those paths change.
- Run `go test ./...`, `go vet ./...`, and `go build ./...` for a normal code change.
