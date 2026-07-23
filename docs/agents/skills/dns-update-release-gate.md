---
kind: skill
slug: dns-update-release-gate
title: dns-update release gate
summary: Validate release-ready changes, generated docs, and paths that affect packages when relevant.
---
This document uses ASD-STE100 Simplified Technical English.

1. Keep the canonical source and generated projections synchronized.
2. Run `go run ./cmd/agentdocgen` after a change in `docs/agents/**`.
3. Run the tests for each changed path.
4. Run `go test ./...`, `go vet ./...`, and `go build ./...` for a release change.
5. Update the applicable documentation.
6. Run package checks when package, deploy, scheduler, or installer paths change.
7. Leave the tree safe for a public repository.
8. Leave the tree ready for review, a tag, or release handoff.
