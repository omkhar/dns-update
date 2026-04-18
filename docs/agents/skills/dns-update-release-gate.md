---
kind: skill
slug: dns-update-release-gate
title: dns-update release gate
summary: Validate release-ready changes, generated docs, and package-impacting paths when relevant.
---

1. Confirm the canonical source and generated projections are in sync, keep the change reviewable, and regenerate the agent docs with `go run ./cmd/agentdocgen` whenever `docs/agents/**` changes.
2. Run the relevant tests for the touched code paths. Release-facing changes still require `go test ./...`, `go vet ./...`, and `go build ./...`.
3. Update docs when the change affects docs, security behavior, or release-facing behavior.
4. Run release or packaging checks only when the change touches release, packaging, deploy, scheduler, or install paths.
5. For packaging or release work, run the relevant local validation such as `./packaging/build-release-assets.sh`, `./packaging/verify-release-assets.sh out/release/*.tar.gz out/release/*.zip`, or the package builders only when those assets are in scope.
6. Confirm the wording stays public-repo-safe and there are no internal references or private paths, then leave the tree ready for review, tag, or release handoff.
