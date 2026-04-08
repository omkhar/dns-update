# dns-update adversarial review and Go 1.26.2 assessment

## Executive summary

This branch updates the repository to the latest official Go patch release,
`1.26.2`, as of April 8, 2026. It also combines the pending local source
modernization work with the live remote workflow updates and closes the two
meaningful hardening gaps that remained outside the Unix happy path:

1. The Windows Task Scheduler installer now locks the Cloudflare token file to
   explicit NTFS allow rules for `SYSTEM`, local Administrators, and the user
   running the installer.
2. The optional remote bootstrap build path now verifies the downloaded Go
   toolchain tarball against the official SHA256 before extraction.

It also refreshes the stale Fedora container digests that were keeping the open
Dependabot PR red in `Systemd Integration`.

## Evidence collected

- `go.mod` now requires `go 1.26.2`.
- The host toolchain on this machine started at `go version go1.26.1 darwin/arm64`,
  and the repository validation was rerun with Go toolchain auto-resolution after
  the module bump.
- The local worktree applies the safe `go fix -diff ./...` suggestions in:
  - `internal/app/run.go`
  - `internal/config/config.go`
  - `internal/securefile/securefile.go`
  - related tests
- Packaging now derives its Go patch version from `go.mod` instead of duplicating
  it in multiple shell scripts.
- `deploy/windows/register-scheduled-task.ps1` now enforces a restrictive token
  ACL before task registration, and
  `packaging/test-windows-task-scheduler.ps1` asserts that ACL contract.
- `packaging/build-remote-container.sh` now verifies the bootstrap Go tarball
  checksum before extracting it into `/usr/local/go`.
- `.github/workflows/systemd-integration.yml` now points Fedora stable and
  rawhide at currently resolvable digests.

## Findings and disposition

### Resolved: Windows token-file privacy was helper-documented but not helper-enforced

- Previous severity: Medium
- Status: Fixed in this branch
- Evidence:
  - The Windows registration helper now disables inherited ACLs on the token
    file and replaces them with explicit allow rules.
  - The Windows scheduler integration test now verifies that inheritance is off
    and that only the expected principals retain access.
- Residual risk:
  - Direct manual Windows deployments that bypass the helper still need
    equivalent ACL discipline.

### Resolved: Remote bootstrap builds downloaded Go without integrity verification

- Previous severity: Medium
- Status: Fixed in this branch
- Evidence:
  - The bootstrap Dockerfile path now fetches the published SHA256 from
    `dl.google.com` and verifies the downloaded archive before extraction.
- Residual risk:
  - This remains a network bootstrap path; a digest-pinned builder image is
    still preferable for tightly controlled release environments.

### Resolved: Fedora systemd integration pins had gone stale

- Previous severity: Medium for CI reliability
- Status: Fixed in this branch
- Evidence:
  - The open Dependabot PR failed because both pinned Fedora image digests
    returned `not found` from Quay.
  - The workflow now uses fresh digests that resolve locally with Docker.

## Local validation

The following validation passed locally on April 8, 2026:

- `gofmt -w` on the modified Go files
- `git diff --check`
- `shellcheck -x packaging/*.sh deploy/launchd/*.sh`
- `sh -n packaging/build-remote-container.sh packaging/lib.sh deploy/launchd/*.sh`
- `go test ./...`
- `go vet ./...`
- `DNS_UPDATE_SKIP_COVERAGE_TEST=1 DNS_UPDATE_SKIP_MUTATION_TEST=1 go test -race ./...`
- `go test -run '^$' -fuzz '^FuzzNormalizeFQDN$' -fuzztime=15s ./internal/config`
- `go test -run '^$' -fuzz '^FuzzParseProbeURL$' -fuzztime=15s ./internal/config`
- `go test -run '^$' -fuzz '^FuzzParseResponse$' -fuzztime=15s ./internal/egress`
- `go build ./...`
- `PACKAGING_SKIP_SIGN=1 RELEASE_SKIP_PACKAGES=1 ./packaging/build-release-assets.sh`
- `./packaging/verify-release-assets.sh out/release/*.tar.gz out/release/*.zip`
- `./packaging/test-launchd-scheduler.sh`
- `PACKAGING_FORCE_DIRECT_DEB=1 PACKAGING_SKIP_SIGN=1 PACKAGING_SKIP_NATIVE_TESTS=1 ./packaging/build-deb.sh`
- `PACKAGING_LINUX_MACROS=1 PACKAGING_SKIP_BUILDDEPS=1 PACKAGING_SKIP_SIGN=1 PACKAGING_SKIP_NATIVE_TESTS=1 ./packaging/build-rpm.sh`
- `./packaging/verify-release-assets.sh out/packages/deb/*/*.deb out/packages/rpm/*/*.rpm`

Notes:

- The Homebrew `rpm` toolchain on macOS emitted `elfdeps` helper warnings while
  still producing valid RPMs. Payload verification passed afterward, and the
  GitHub Actions Linux environment uses the native RPM toolchain instead of the
  Homebrew one.
- Windows scheduler integration could not be executed locally on this host, but
  the PowerShell helper and test changes are narrow and covered by CI.

## Review outcome

I do not see any remaining high-confidence correctness regressions in the local
or remote diff after the fixes above. The branch is ready to raise with CI as
the final cross-platform gate.
