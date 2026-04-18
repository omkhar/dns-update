---
kind: skill
slug: dns-update-change-gate
title: dns-update change gate
summary: Validate a change for correctness, safety, and reviewability before merge.
---

1. Restate the behavior change, keep the edit in scope, and keep the PR small enough to review cleanly.
2. Decide whether the change touches behavior, docs, security, or release/package paths, and regenerate the agent docs with `go run ./cmd/agentdocgen` whenever `docs/agents/**` changes.
3. Run the smallest tests that prove the change, then widen coverage only if the risk justifies it. Normal code changes still require `go test ./...`, `go vet ./...`, and `go build ./...`.
4. Review the change for correctness, idiomatic Go or shell, performance, and public-repo-safe wording.
5. Update related docs when behavior or release-facing behavior changed, and skip release or packaging checks unless those paths are touched.
