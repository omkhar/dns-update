# Runtime versions

This document uses ASD-STE100 Simplified Technical English.

ASD-STE100 Rule 1.12 permits technical names.
This document uses these technical names: Go, GitHub Actions, Git, runner,
commit SHA, container, image, tag, digest, SemVer, actionlint, govulncheck,
golangci-lint, yamllint, cosign, and ShellCheck.

The `go` directive in `go.mod` is the source for the Go version.
It sets the minimum Go version.
CI installs the latest patch release of that Go minor version.
Each workflow job resolves the `go` directive to a SemVer tilde range before `actions/setup-go` runs.
Then `actions/setup-go` uses `check-latest` to install the newest release in that range.
Thus, a new Go patch release that fixes a standard-library vulnerability needs no repository change.
Use the latest patch release of the same Go minor version for local builds and release builds.

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

See `docs/runtime-versions.json` for the action names, runner labels, container
names, container digests, and tool versions.
Repository tests compare that file with all workflow files.
The file does not copy values that Dependabot changes.
The tests read each action commit SHA from its workflow `uses:` line.
All actions from one repository must use the same commit SHA.

The scheduled systemd drift job uses floating container tags.
This job detects a new distribution image before the repository changes its deterministic pull-request pins.

Use primary upstream release pages when you update a version.
Resolve annotated Git tags to the commit that the tag identifies.
Check a container manifest before you record its multi-platform digest.
