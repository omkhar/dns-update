---
kind: skill
slug: dns-update-release-gate
title: dns-update release gate
summary: Validate release-ready changes, generated docs, and package-impacting paths when relevant.
---

# dns-update release gate

Use this playbook when a change is headed for release or merge.

## Playbook

1. Confirm the canonical source and all generated projections are in sync.
2. Verify the change is still small enough to review without guesswork.
3. Regenerate the agent docs with `go run ./cmd/agentdocgen` whenever `docs/agents/**` changes.
4. Run the relevant tests for the touched code paths. Release-facing changes still require `go test ./...`, `go vet ./...`, and `go build ./...`.
5. Check whether the change affects docs, security behavior, or release-facing behavior, and update docs when it does.
6. Run release or packaging checks only when the change touches release, packaging, deploy, scheduler, or install paths.
7. For packaging or release work, run the relevant local validation such as `./packaging/build-release-assets.sh`, `./packaging/verify-release-assets.sh out/release/*.tar.gz out/release/*.zip`, or the package builders only when those assets are in scope.
8. Confirm the wording stays public-repo-safe and there are no internal references or private paths.
9. Keep the root guidance thin and the skill outputs as step-by-step playbooks.
10. Leave the tree ready for review, tag, or release handoff.

## Stop conditions

- Pause if a release check fails or if the change needs follow-up outside the current scope.
