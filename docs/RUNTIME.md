# Runtime versions

This document uses ASD-STE100 Simplified Technical English.

Use Go 1.26.5 to build, test, and release `dns-update`.
The `go` directive in `go.mod` is the source for the Go version.

GitHub Actions uses these fixed runner images:

- Ubuntu 24.04
- macOS 26
- Windows Server 2025

The workflows pin each external action to a full commit SHA.
The workflows also pin deterministic integration containers to multi-platform image digests.

See `docs/runtime-versions.json` for the exact action versions, SHAs, runner labels, container tags, and container digests.
Repository tests compare that file with `go.mod` and every workflow.

The scheduled systemd drift job uses floating container tags.
This job detects a new distribution image before the repository changes its deterministic pull-request pins.

Use primary upstream release pages when you update a version.
Resolve annotated Git tags to the commit that the tag identifies.
Inspect a container manifest before you record its multi-platform digest.
