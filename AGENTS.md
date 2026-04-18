- Keep changes in scope, preserve unrelated work, and keep PRs human-reviewable; CI rejects changes above `35` changed files or `3000` total added plus deleted lines.
- Regenerate agent docs from `docs/agents/**`, never hand-edit generated projections, and prefer the smallest correct idiomatic change with focused tests; normal code changes still require `go test ./...`, `go vet ./...`, and `go build ./...`.
- Use public-repo-safe wording only, and update docs plus release or packaging checks only when those behaviors or paths are touched.

## Repo Skills
- `$dns-update-change-gate`: Validate a change for correctness, safety, and reviewability before merge.
- `$dns-update-release-gate`: Validate release-ready changes, generated docs, and package-impacting paths when relevant.
