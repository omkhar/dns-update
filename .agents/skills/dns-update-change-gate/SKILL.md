---
name: dns-update-change-gate
description: Validate a change for correctness, safety, and reviewability before merge.
---
This document uses ASD-STE100 Simplified Technical English.

1. Restate the behavior change.
2. Keep the edit in scope.
3. Keep the pull request easy to review.
4. Run `go run ./cmd/agentdocgen` after a change in `docs/agents/**`.
5. Run the smallest tests that prove the change.
6. Review correctness, idiomatic code, performance, public wording, documentation, and release effects.
7. Run `go test ./...`, `go vet ./...`, and `go build ./...` for a normal code change.
