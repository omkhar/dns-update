# Contributing

Thanks for contributing to this repository.

## Before You Start

- Keep changes focused. Small, reviewable pull requests are easier to merge
  safely.
- Open an issue before starting broad refactors, new providers, or behavior
  changes that affect operators.
- For security-sensitive changes, read [SECURITY.md](./SECURITY.md) first.

## Development Expectations

- Match the existing Go style and package boundaries.
- Prefer small dependencies and explicit code over broad frameworks.
- Preserve existing behavior unless the change clearly documents and tests a
  behavior update.
- Add or update tests for behavior you change.
- Keep pull requests small enough that a reviewer can reason about the full
  change without guesswork.
- Pull request CI rejects changes above 35 changed files or 2500 total added
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
- Write the pull request description so a reviewer can understand:
  - what changed
  - why it changed
  - how it was tested
  - whether there are follow-up tasks

This repository is intended to use squash merge on GitHub.

Because of that:

- You do not need a perfect public commit history.
- You should still keep commits coherent enough for review while the PR is open.
- Use a pull request title that works well as the eventual squash commit
  message.

## Commit Guidance

- Prefer imperative commit subjects.
- Keep the first line concise and specific.
- Avoid mixing unrelated docs, refactors, and behavior changes in one PR.

## Reporting Problems

- Bugs and feature requests belong in GitHub issues.
- Security vulnerabilities should follow the process in
  [SECURITY.md](./SECURITY.md), not a public bug report with exploit details.

## Licensing

By contributing to this repository, you agree that your contributions are made
under the Apache License 2.0 in [LICENSE](./LICENSE).
