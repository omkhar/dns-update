# Contributing

This document uses ASD-STE100 Simplified Technical English.

Thank you for contributing to this repository.

## Before You Start

- Keep changes focused.
- Make each pull request easy to review.
- Open an issue before starting broad refactors, new providers, or behavior
  changes that affect operators.
- For security-sensitive changes, read [SECURITY.md](./SECURITY.md) first.

## Development Expectations

- Match the existing Go style and package boundaries.
- Prefer small dependencies and explicit code over broad frameworks.
- Preserve existing behavior unless the change clearly documents and tests a
  behavior update.
- Add or update tests for behavior you change.
- Keep pull requests small enough for a complete review.
- Pull request CI rejects changes above 35 changed files or 3000 total added
  plus deleted lines.
- Edit `docs/agents/**` instead of hand-editing generated agent projections.
- Regenerate agent projections with `go run ./cmd/agentdocgen` whenever
  `docs/agents/**` changes.
- Keep documentation in sync when flags, config, security posture, or
  operational behavior changes.

## Local Checks

Run the checks relevant to your change before opening a pull request.

Typical checks:

```sh
go test ./...
go vet ./...
go build ./cmd/dns-update
```

If you are changing a narrow area, run the focused package tests too.

## Branch and Pull Request Flow

- Create one topic branch per change.
- Use clear branch names such as `docs/security-policy`,
  `fix/cloudflare-timeout`, or `feat/provider-foo`.
- Keep the pull request scoped to one logical change.
- Include this information in the pull request description:
  - what changed
  - why it changed
  - how you tested it
  - whether there are follow-up tasks

Use squash merge on GitHub for this repository.

Because of that:

- You do not need a perfect public commit history.
- Keep commits coherent enough for review while the PR is open.
- Use a pull request title that works well as the eventual squash commit
  message.

## Commit Guidance

- Prefer imperative commit subjects.
- Keep the first line concise and specific.
- Avoid mixing unrelated docs, refactors, and behavior changes in one PR.

## Reporting Problems

- Bugs and feature requests belong in GitHub issues.
- Follow [SECURITY.md](./SECURITY.md) for security vulnerabilities.
- Do not put exploit details in a public bug report.

## Licensing

When you contribute to this repository, you agree to license your contributions
under the Apache License 2.0 in [LICENSE](./LICENSE).
