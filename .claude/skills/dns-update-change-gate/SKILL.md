---
name: dns-update-change-gate
description: Validate a change for correctness, safety, and reviewability before merge.
---
1. Restate the behavior change, keep the edit in scope, and keep the PR reviewable.
2. Regenerate the agent docs with `go run ./cmd/agentdocgen` whenever `docs/agents/**` changes.
3. Run the smallest tests that prove the change; normal code changes still require `go test ./...`, `go vet ./...`, and `go build ./...`.
4. Review for correctness, idiomatic Go or shell, performance, public wording, and any docs or release impacts.
