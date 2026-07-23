# Runtime versions

This document uses ASD-STE100 Simplified Technical English.

ASD-STE100 Rule 1.12 permits technical names.
This document uses these technical names: Go, GitHub Actions, Git, runner,
commit SHA, container, image, tag, digest, SemVer, actionlint, govulncheck,
golangci-lint, yamllint, cosign, and ShellCheck.

Use Go 1.26.5 to build, test, and release `dns-update`.
The `go` directive in `go.mod` is the source for the Go version.

GitHub Actions uses these runner images:

- Ubuntu 24.04
- macOS 26
- Windows Server 2025

The workflows pin each external action to a full commit SHA.
The workflows also pin deterministic integration containers to multi-platform image digests.

The workflows install these tool versions:

- actionlint 1.7.12
- govulncheck 1.6.0
- golangci-lint 2.12.2
- yamllint 1.38.0
- cosign 3.1.2

Ubuntu package repositories supply ShellCheck.
The repository does not specify the ShellCheck version.

See `docs/runtime-versions.json` for the action SHAs, runner labels, container
names, container digests, and tool versions.
Repository tests compare that file with `go.mod` and all workflow files.

The scheduled systemd drift job uses floating container tags.
This job detects a new distribution image before the repository changes its deterministic pull-request pins.

Use primary upstream release pages when you update a version.
Resolve annotated Git tags to the commit that the tag identifies.
Check a container manifest before you record its multi-platform digest.
