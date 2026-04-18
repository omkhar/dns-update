---
kind: skill
slug: dns-update-release-gate
title: dns-update release gate
summary: Validate release-ready changes, generated docs, and package-impacting paths when relevant.
---

1. Keep the canonical source and generated projections in sync, and regenerate the agent docs with `go run ./cmd/agentdocgen` whenever `docs/agents/**` changes.
2. Run the relevant tests for the touched code paths; release-facing changes still require `go test ./...`, `go vet ./...`, and `go build ./...`.
3. Update docs and run release or packaging checks only when release, packaging, deploy, scheduler, or install paths are touched.
4. Confirm the wording stays public-repo-safe, then leave the tree ready for review, tag, or release handoff.
