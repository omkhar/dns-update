---
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
4. Regenerate the agent docs with `go run ./cmd/agentdocgen` whenever `docs/agents/**` changes.
5. Run the smallest tests that prove the change, then widen coverage only if the risk justifies it. Normal code changes still require `go test ./...`, `go vet ./...`, and `go build ./...`.
6. Review the change for correctness, idiomatic Go or shell, performance, and public-repo-safe wording.
7. If behavior, security, scheduler, packaging, or release-facing behavior changed, update the related docs in the same change.
8. Skip release and packaging checks unless the change actually touches those paths.

## Stop conditions

- Split the change if review becomes hard.
- Call out security, correctness, or generated-file drift before merge.
