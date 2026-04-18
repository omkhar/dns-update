---
kind: skill
slug: dns-update-change-gate
title: dns-update change gate
summary: Validate a change for correctness, safety, and reviewability before merge.
---
1. Restate the behavior change, keep the edit in scope and the PR reviewable, and regenerate the agent docs with `go run ./cmd/agentdocgen` whenever `docs/agents/**` changes.
2. Run the smallest tests that prove the change, then review for correctness, idiomatic Go or shell, performance, public wording, and any docs or release impacts; normal code changes still require `go test ./...`, `go vet ./...`, and `go build ./...`.
