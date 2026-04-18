---
kind: skill
slug: dns-update-release-gate
title: dns-update release gate
summary: Validate release-ready changes, generated docs, and package-impacting paths when relevant.
---
1. Keep the canonical source and generated projections in sync, regenerate the agent docs with `go run ./cmd/agentdocgen` whenever `docs/agents/**` changes, and run the relevant tests for the touched code paths; release-facing changes still require `go test ./...`, `go vet ./...`, and `go build ./...`.
2. Update docs and run release or packaging checks only when release, packaging, deploy, scheduler, or install paths are touched, then leave the tree public-repo-safe and ready for review, tag, or release handoff.
