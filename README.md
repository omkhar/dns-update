# dns-update

This document uses ASD-STE100 Simplified Technical English.

`dns-update` is a Go service that keeps one hostname's `A` and `AAAA` records
aligned with the host's current egress IPv4/IPv6 addresses.

The current implementation uses the Cloudflare DNS Records API.
The internal provider interface permits more provider implementations.

The project supplies release and deployment files for these platforms:

- Linux ships native `.deb` and `.rpm` packages plus systemd units.
- macOS ships release archives plus a native `launchd` helper.
- Windows ships release archives plus a native Task Scheduler helper.

Linux packages also install the `dns-update(1)` man page.
Its source is `docs/dns-update.1`.

Read [`docs/FUNCTIONS.md`](docs/FUNCTIONS.md) for the complete supported interface.
Read [`docs/DOCUMENTATION.md`](docs/DOCUMENTATION.md) for the documentation policy.

## Actions

Current GitHub Actions workflow status:

- [CI](https://github.com/omkhar/dns-update/actions/workflows/ci.yml): [![CI](https://github.com/omkhar/dns-update/actions/workflows/ci.yml/badge.svg)](https://github.com/omkhar/dns-update/actions/workflows/ci.yml)
- [CodeQL](https://github.com/omkhar/dns-update/actions/workflows/codeql.yml): [![CodeQL](https://github.com/omkhar/dns-update/actions/workflows/codeql.yml/badge.svg)](https://github.com/omkhar/dns-update/actions/workflows/codeql.yml)
- [Dependabot Updates](https://github.com/omkhar/dns-update/actions/workflows/dynamic/dependabot/dependabot-updates): [![Dependabot Updates](https://github.com/omkhar/dns-update/actions/workflows/dynamic/dependabot/dependabot-updates/badge.svg)](https://github.com/omkhar/dns-update/actions/workflows/dynamic/dependabot/dependabot-updates)
- [Dependency Graph](https://github.com/omkhar/dns-update/actions/workflows/dynamic/dependabot/update-graph): [![Dependency Graph](https://github.com/omkhar/dns-update/actions/workflows/dynamic/dependabot/update-graph/badge.svg)](https://github.com/omkhar/dns-update/actions/workflows/dynamic/dependabot/update-graph)
- [Dependency Review](https://github.com/omkhar/dns-update/actions/workflows/dependency-review.yml): [![Dependency Review](https://github.com/omkhar/dns-update/actions/workflows/dependency-review.yml/badge.svg)](https://github.com/omkhar/dns-update/actions/workflows/dependency-review.yml)
- [OSV Scanner](https://github.com/omkhar/dns-update/actions/workflows/osv-scanner.yml): [![OSV Scanner](https://github.com/omkhar/dns-update/actions/workflows/osv-scanner.yml/badge.svg)](https://github.com/omkhar/dns-update/actions/workflows/osv-scanner.yml)
- [Nightly](https://github.com/omkhar/dns-update/actions/workflows/nightly.yml): [![Nightly](https://github.com/omkhar/dns-update/actions/workflows/nightly.yml/badge.svg)](https://github.com/omkhar/dns-update/actions/workflows/nightly.yml)
- [Package Validation](https://github.com/omkhar/dns-update/actions/workflows/package-validation.yml): [![Package Validation](https://github.com/omkhar/dns-update/actions/workflows/package-validation.yml/badge.svg)](https://github.com/omkhar/dns-update/actions/workflows/package-validation.yml)
- [Release](https://github.com/omkhar/dns-update/actions/workflows/release.yml): [![Release](https://github.com/omkhar/dns-update/actions/workflows/release.yml/badge.svg)](https://github.com/omkhar/dns-update/actions/workflows/release.yml)
- [Scheduler Integration](https://github.com/omkhar/dns-update/actions/workflows/scheduler-integration.yml): [![Scheduler Integration](https://github.com/omkhar/dns-update/actions/workflows/scheduler-integration.yml/badge.svg)](https://github.com/omkhar/dns-update/actions/workflows/scheduler-integration.yml)
- [Scorecard](https://github.com/omkhar/dns-update/actions/workflows/scorecard.yml): [![Scorecard](https://github.com/omkhar/dns-update/actions/workflows/scorecard.yml/badge.svg)](https://github.com/omkhar/dns-update/actions/workflows/scorecard.yml)
- [Systemd Integration](https://github.com/omkhar/dns-update/actions/workflows/systemd-integration.yml): [![Systemd Integration](https://github.com/omkhar/dns-update/actions/workflows/systemd-integration.yml/badge.svg)](https://github.com/omkhar/dns-update/actions/workflows/systemd-integration.yml)
- [zizmor](https://github.com/omkhar/dns-update/actions/workflows/zizmor.yml): [![zizmor](https://github.com/omkhar/dns-update/actions/workflows/zizmor.yml/badge.svg)](https://github.com/omkhar/dns-update/actions/workflows/zizmor.yml)

## Behavior

On each run, the service:

1. Fetches probe responses from:
   - `probe.ipv4_url` (default `https://4.ip.omsab.net/`)
   - `probe.ipv6_url` (default `https://6.ip.omsab.net/`)
2. Parses responses in `ip=...` format.
3. Validates returned addresses by family:
   - IPv4 probe must yield a valid IPv4 or `ip=none`
   - IPv6 probe must yield a valid IPv6 or `ip=none`
   - If one probe family fails, aborts the run by default
   - If `probe.allow_partial_failure` is `true`, logs a warning and reconciles
     only the family that succeeded. The failed family stays unchanged
   - If both probe families fail, aborts the run
   - Only explicit `ip=none` removes that record family
4. Reads the current provider-side records for `record.name`.
5. Compares desired vs current DNS state:
   - If DNS already matches, the service exits without an update unless you use
     `-force-push`.
   - If you use `-force-push`, the service refreshes each existing address
     record that matches an observed address.
   - Normal reconciliation creates a missing observed record even when you use
     `-force-push`.
   - If DNS differs, the service applies only the required record operations.
   - If you use `-delete`, the service skips egress probing. It deletes only
     the selected managed record families for `record.name`.
6. Re-reads provider state and verifies the final result.
7. Retries transient probe and provider failures with bounded exponential
   backoff, jitter, and hard attempt/delay limits.

## Operational Assumption

`dns-update` assumes it is the sole writer for the managed hostname in
`record.name`.

- If another controller, script, or human can update the same name concurrently,
  the outcome is effectively last-writer-wins between reconciliations.
- The post-apply verification step detects divergence after mutation, but it
  does not provide a provider-side compare-and-swap or distributed lock.
- Keep one owner for the managed hostname, even if the wider DNS zone has other
  automation.

## Configuration

The app reads JSON config with this schema:

- `record.name` (required): FQDN.
- `record.zone` (required): FQDN. `record.name` must be either
  this exact zone apex or a true subdomain within it.
- `record.ttl_seconds` (required): positive integer TTL for created records. For
  Cloudflare this must be `1` (automatic) or between `30` and `86400`.
- `probe.ipv4_url` (optional): defaults to `https://4.ip.omsab.net/`. Overrides
  must keep this host or use a loopback or `localhost` test endpoint.
- `probe.ipv6_url` (optional): defaults to `https://6.ip.omsab.net/`. Overrides
  must keep this host or use a loopback or `localhost` test endpoint.
- `probe.timeout` (optional): Go duration string, defaults to `10s`.
- `probe.allow_insecure_http` (optional): defaults to `false`. The app accepts
  HTTP probe URLs only for loopback or `localhost` test endpoints.
- `probe.allow_partial_failure` (optional): defaults to `false`. When you enable
  it, a failed single-family probe leaves that record family untouched. The app
  reconciles the successful family.
- `provider.type` (required): currently `cloudflare`.
- `provider.timeout` (optional): Go duration string, defaults to `10s`.
- `provider.cloudflare.zone_id` (required): Cloudflare zone ID for the managed
  zone.
- `provider.cloudflare.api_token_file` (required): file that contains only the
  Cloudflare API token.
- `provider.cloudflare.base_url` (optional): defaults to
  `https://api.cloudflare.com/client/v4/`. The app accepts the default
  Cloudflare API host or loopback or `localhost` test endpoints.
- `provider.cloudflare.proxied` (optional): sets Cloudflare proxying for the
  managed A/AAAA records. Defaults to `false`.

See `config.example.json` for a complete sample.
The sample shows the full schema.
Replace its placeholder values and token-file path for your deployment.

## Configuration Sources

The app reads record, probe, and provider settings from the JSON config file.
Runtime options have a separate small override surface.

Runtime settings:

- `-config` or `DNS_UPDATE_CONFIG`
- `-delete` on the command line deletes managed records.
  It does not reconcile to observed egress IPs. Bare `-delete` deletes both `A` and
  `AAAA`. The command also accepts `-delete=a`, `-delete=aaaa`, and `-delete=both`
- `-dry-run` or `DNS_UPDATE_DRY_RUN`
- `-force-push` on the command line to refresh existing address records that
  match observed addresses
- `-verbose` or `DNS_UPDATE_VERBOSE`
- `-timeout` or `DNS_UPDATE_TIMEOUT`
- `DNS_UPDATE_PROVIDER_CLOUDFLARE_API_TOKEN_FILE` to override only
  `provider.cloudflare.api_token_file`, which is primarily useful for systemd
  credentials

CLI-only introspection settings:

- `-version` or `--version` prints the binary version and exits.
  It does not load config.
- `-h`, `--h`, `-help`, or `--help` prints flag help and exits.
  It does not load config.
- `-validate-config` loads and validates the assembled configuration, prints
  `config is valid`, and exits. It does not contact Cloudflare
- `-print-effective-config` loads and validates the assembled configuration,
  prints the fully assembled effective configuration as JSON, and exits.
  It does not contact Cloudflare
- `-validate-config` and `-print-effective-config` are mutually exclusive
- Both introspection modes still validate local provider prerequisites such as
  the Cloudflare token-file path

Record and provider settings otherwise come from JSON config file fields.

Behavior notes:

- If you use `-config` or `DNS_UPDATE_CONFIG`, the app requires that path. The
  path must exist.
- If you do not use either setting, the app first looks for `config.json` in
  the current working directory. It then looks for
  `/etc/dns-update/config.json`.
- Built-in defaults still apply for optional unset values such as probe URLs,
  timeouts, and the Cloudflare base URL.
- `-delete` is intentionally CLI-only. There is no config-file or environment
  variable equivalent for destructive record deletion.
- `-delete` is compatible with `-dry-run`.
- `-delete` and `-force-push` are mutually exclusive.

## Security Notes

- The codebase keeps the dependency surface intentionally small and prefers
  reviewed packages over broad frameworks.
- Do not put secrets in config.
- Store the Cloudflare API token in a separate file.
- On Unix-like systems, restrict the token file permissions (for example
  `chmod 600`).
- On Unix-like systems, keep the token file in a non-writable directory.
- The app rejects a parent directory that group or other users can write.
- Windows deployments use NTFS ACLs for token-file privacy.
- The app rejects Windows token ACLs that give access to other users.
- The app rejects Windows parent-directory ACLs that give write access to other users.
- The token file must not be a symlink.
- The app rejects symlinks in deeper configured path components.
- On Unix-like systems, the app opens the token file and does not follow symlinks.
- The app validates the open file again before it reads the token.
- Use HTTPS probe URLs. Use HTTP only when the deployment requires
  `probe.allow_insecure_http`.
- Set probe URL overrides only to the shipped `4.ip.omsab.net` and
  `6.ip.omsab.net` hosts or to loopback or `localhost` test endpoints.
- If you enable `probe.allow_insecure_http`, that risk increases.
  An on-path actor can change probe responses. The app accepts HTTP only for loopback
  or `localhost` test endpoints.
- By default, any single-family probe failure aborts reconciliation.
  No record family changes after the failure.
  The `probe.allow_partial_failure` option trades that fail-closed posture for
  availability on hosts that support only one address family.
- Scope the Cloudflare token to the single zone that `dns-update` manages.
- The app filters Cloudflare record reads to the managed hostname. It does not
  list the full zone.
- `provider.cloudflare.base_url` changes where the app sends the Cloudflare token.
- The app accepts only the default API host, loopback, or `localhost`.
- Probe and provider HTTP clients use a fixed custom user-agent.
  They ignore ambient proxy environment variables.
  They use bounded retries and honor `Retry-After` when present.

## Toolchain

Build and test with a patched Go toolchain. The module requires Go `1.26.5`.
See [`docs/RUNTIME.md`](docs/RUNTIME.md) for all build and CI runtime pins.
Read [`docs/LIMITATIONS.md`](docs/LIMITATIONS.md) before you deploy a scheduled writer.

## Dependencies

Runtime dependencies are deliberately narrow:

- `github.com/cloudflare/cloudflare-go/v6` for the Cloudflare DNS API
- `github.com/google/go-cmp/cmp` is used in tests only

The current build does not use a separate `golang.org/x/time/rate` dependency.
Code in this repository controls outbound request pacing.

## Cloudflare Token Scope

Because the config requires `provider.cloudflare.zone_id`, the app does not need
to discover the zone through the Cloudflare API. For minimum privilege, create
a Cloudflare API token for only the target zone. Grant the token only DNS edit
capability for that zone.

## Build and Run

Build the binary:

```sh
go build ./cmd/dns-update
```

Print the binary version:

```sh
./dns-update --version
```

Run one reconciliation cycle:

```sh
./dns-update -config /etc/dns-update/config.json
```

The packaged layout uses `/etc/dns-update/config.json`.
The command uses this file when the current directory has no `config.json`.

Limit one reconciliation cycle with retries and backoff:

```sh
./dns-update -config /etc/dns-update/config.json -timeout 30s
```

Preview planned changes. Do not apply them:

```sh
./dns-update -config /etc/dns-update/config.json -dry-run
```

Preview deletion of both managed address-record families. Do not change DNS:

```sh
./dns-update -config /etc/dns-update/config.json -dry-run -delete
```

Delete only the managed IPv4 record family:

```sh
./dns-update -config /etc/dns-update/config.json -delete=a
```

Refresh existing address records that match the observed egress IPs:

```sh
./dns-update -config /etc/dns-update/config.json -force-push
```

Combine the two flags to preview the forced update. Do not change DNS:

```sh
./dns-update -config /etc/dns-update/config.json -dry-run -force-push
```

Validate the assembled configuration:

```sh
./dns-update -config /etc/dns-update/config.json -validate-config
```

Print the effective configuration after JSON loading plus runtime overrides:

```sh
DNS_UPDATE_PROVIDER_CLOUDFLARE_API_TOKEN_FILE=/etc/dns-update/cloudflare.token \
./dns-update -config /etc/dns-update/config.json -print-effective-config
```

For a direct binary run, update the packaged sample token path.
Set it to `/etc/dns-update/cloudflare.token`.
You can instead use the environment override shown above.

## Platform Schedulers

The binary itself runs one reconciliation cycle per invocation.
Use the native scheduler for each operating system for periodic execution:

- Linux: systemd service plus timer under `deploy/systemd/`
- macOS: `launchd` `LaunchDaemon` helper under `deploy/launchd/`
- Windows: Task Scheduler helper under `deploy/windows/`

Each release archive also includes the `deploy/` tree so the scheduler helpers
travel with the binary on non-Linux systems.

`-force-push` is intentionally not part of the default scheduler configuration.
Use it to refresh an existing address record that matches an observed address.

`-delete` is also intentionally not part of the default scheduler
configuration. It is a one-shot destructive operator action, not a steady-state
reconciliation mode.

## Linux: systemd

Example hardened systemd units live in `deploy/systemd/`.

- `deploy/systemd/dns-update.service` runs one reconciliation.
  It uses a locked-down `DynamicUser`, no ambient capabilities, and a read-only filesystem view.
  It receives the Cloudflare token as a private systemd credential.
- `deploy/systemd/dns-update.timer` starts the service at boot or enable time.
- The timer runs on five-minute clock boundaries.
- It keeps future runs queued after a skipped early start.
- `Persistent=yes` starts one missed run after downtime.
- `deploy/systemd/dns-update.env` shows how to override runtime options
  such as `DNS_UPDATE_TIMEOUT`. You do not have to edit the unit.

The service expects:

- `/usr/bin/dns-update`
- `/etc/dns-update/config.json`
- `/etc/dns-update/cloudflare.token`

`LoadCredential=` mounts the token into the service.
`DNS_UPDATE_PROVIDER_CLOUDFLARE_API_TOKEN_FILE=%d/cloudflare.token` gives the path to the binary.
The unit does not store this credential in the JSON config.
On some systems the runtime credential file may appear with a read-only mode
such as `0400` or `0440`.
This mode is normal for systemd-managed credentials.
Do not manually change modes under `/run/credentials/`.

See `deploy/systemd/README.md` for installation steps.

## macOS: launchd

Use `deploy/launchd/install-launchd-job.sh` to install a `LaunchDaemon` for
system-wide scheduled execution on macOS.

- default binary path: `/usr/local/bin/dns-update`
- default config path: `/usr/local/etc/dns-update/config.json`
- default token path: `/usr/local/etc/dns-update/cloudflare.token`
- default log path: `/var/log/dns-update.log`

Example:

```sh
sudo ./deploy/launchd/install-launchd-job.sh \
  --binary /usr/local/bin/dns-update \
  --config /usr/local/etc/dns-update/config.json \
  --token /usr/local/etc/dns-update/cloudflare.token \
  --interval 300 \
  --log /var/log/dns-update.log
```

The helper writes `/Library/LaunchDaemons/com.dns-update.plist` by default,
runs once at load with `RunAtLoad`, and repeats with `StartInterval`. See
`deploy/launchd/README.md` for the full install and update flow.

## Windows: Task Scheduler

Use `deploy/windows/register-scheduled-task.ps1` to register a recurring task
that runs `dns-update` as `SYSTEM`.

- suggested binary path: `C:\Program Files\dns-update\dns-update.exe`
- suggested config path: `C:\ProgramData\dns-update\config.json`
- suggested token path: `C:\ProgramData\dns-update\credentials\cloudflare.token`
- suggested log path: `C:\ProgramData\dns-update\dns-update.log`

Example:

```powershell
.\deploy\windows\register-scheduled-task.ps1 `
  -TaskName "dns-update" `
  -BinaryPath "C:\Program Files\dns-update\dns-update.exe" `
  -ConfigPath "C:\ProgramData\dns-update\config.json" `
  -TokenPath "C:\ProgramData\dns-update\credentials\cloudflare.token" `
  -LogPath "C:\ProgramData\dns-update\dns-update.log" `
  -IntervalMinutes 5
```

The helper uses the native `ScheduledTasks` PowerShell API and replaces any
existing task with the same name. See `deploy/windows/README.md` for install
and removal details.

## Packages

Native package metadata lives in:

- `debian/` for Debian-family builds
- `packaging/rpm/dns-update.spec` for RPM-family builds
- `deploy/systemd/` for the shared Linux systemd units and env file used by
  both manual installs and native packages

Linux package builds install:

- `/usr/bin/dns-update`
- the `dns-update(1)` man page under the distro-standard `man1` path
- `/etc/dns-update/dns-update.env`
- `/etc/dns-update/config.example.json` as a shipped sample. The default
  service does not load this file
- `/etc/dns-update/cloudflare.token.example` as a shipped placeholder token file
- distro-standard systemd units for `dns-update.service` and `dns-update.timer`

Packages intentionally omit self-unpacking compression from the binaries.
Thus, the binaries remain compatible with the hardened systemd unit and
`MemoryDenyWriteExecute=yes`.

Build helpers:

```sh
./packaging/build-deb.sh
./packaging/build-rpm.sh
./packaging/build-packages.sh
```

Those wrappers build and sign the default package targets:

- `amd64`
- `rpi32`
- `rpi64`

The build helpers write package artifacts under:

- `out/packages/deb/<target>/`
- `out/packages/rpm/<target>/`

The package helpers sign each package with `cosign sign-blob`.
They write a Sigstore bundle next to the artifact as `*.sigstore.json`.
Package builds do not embed native Debian or RPM repository signatures.

GitHub's `Release` workflow is separate from the native package scripts. It
publishes a full signed cross-platform release asset set under `out/release/`:

- Linux `.deb` packages for `amd64`, `arm64`, and `armhf`
- Linux `.rpm` packages for `x86_64`, `aarch64`, and `armv7hl`
- Linux archive builds for `amd64`, `arm64`, and `armv7`
- macOS archive builds for `amd64` and `arm64`
- Windows archive builds for `amd64` and `arm64`

Each published artifact also has an adjacent `*.sigstore.json` bundle.

Before you enable the packaged timer, create:

- `/etc/dns-update/config.json`
- `/etc/dns-update/cloudflare.token`

The packaged `/etc/dns-update/config.example.json` and
`/etc/dns-update/cloudflare.token.example` are examples only.
The packaged systemd service overrides only
`provider.cloudflare.api_token_file` and reads the live token through a
credential-backed `/etc/dns-update/cloudflare.token`.
For a direct binary run, update the sample token path.
You can also export
`DNS_UPDATE_PROVIDER_CLOUDFLARE_API_TOKEN_FILE=/etc/dns-update/cloudflare.token`.

See `packaging/README.md` for package build requirements and notes.
Use `./packaging/verify-artifacts.sh ...` to verify a package against its
adjacent Sigstore bundle.

macOS and Windows release archives also include native scheduler helpers under:

- `deploy/launchd`
- `deploy/windows`

## Release Notes

See [CHANGELOG.md](./CHANGELOG.md) for public release history.

## Quality Checks

`go test ./...` runs the normal unit and integration suite and also enforces
repository-level quality gates:

- a coverage check that fails unless total statement coverage across `./...` is
  exactly `100.0%`
- a curated mutation suite that copies the repository into temporary workspaces.
  It applies compile-preserving mutants.
  The suite requires the tests to kill each mutant.
- a generated-agent parity check that fails unless the tracked Codex, Claude,
  and Gemini projections match `docs/agents/**`
- a public-repo hygiene check that rejects tracked detritus, local checkout,
  temp, or evidence paths, and banned non-public references

Regenerate the tracked agent projections with:

```sh
go run ./cmd/agentdocgen
```

The mutation and coverage skip environment variables are only for test subprocesses.
Keep these variables unset during regular use.

GitHub Actions is split into four lanes:

- `CI` is the fast pull-request gate.
- It checks reviewability limits, YAML, Go formatting, modules, and shell code.
- It runs linters, vulnerability checks, `go vet`, `go test`, and `go build ./...`.
- `Package Validation` builds the cross-platform release archives on pull
  requests and validates package/archive payloads on `main`.
- `Nightly` runs the expensive repository-level quality gates, longer fuzzing,
  and full release-artifact reproducibility checks.
- `Release` rebuilds tagged artifacts and generates an SBOM.
- It signs the artifacts and emits attestations.
- It verifies all signatures and attestations.
- It puts the asset set in a draft GitHub release.
- It publishes only after these checks pass.

GitHub Actions additionally runs the dedicated `Systemd Integration` workflow
to validate the installed Linux timer/service flow on:

- Debian Stable
- Debian Unstable
- Ubuntu Stable
- Ubuntu Unstable
- Fedora Stable
- Fedora Unstable

GitHub Actions also runs native scheduler integration tests on `main` and the
daily schedule for:

- macOS `launchd`
- Windows Task Scheduler

Those scheduler tests validate real scheduled execution rather than only manual
service starts:

- Linux waits for a later timer-fired `dns-update.service` success after an
  initial skipped activation.
- macOS runs an install-time `-validate-config` preflight and then proves a
  later `launchd`-fired invocation runs without validation-only mode.
- Windows runs an install-time `-ValidateConfig` preflight as `SYSTEM` and
  then proves a later Task Scheduler invocation runs without validation-only
  mode.

## Package Docs

Google-style package comments live alongside the code in:

- `cmd/dns-update`
- `internal/app`
- `internal/buildinfo`
- `internal/config`
- `internal/egress`
- `internal/httpclient`
- `internal/provider`
- `internal/provider/cloudflare`
- `internal/retry`
- `internal/securefile`

Runnable examples are available in:

- `internal/config` for config loading and validation
- `internal/provider` for plan construction

## Repository Policy

For contribution flow and public-repo policy, see:

- [CONTRIBUTING.md](./CONTRIBUTING.md)
- [SECURITY.md](./SECURITY.md)
- [CODE_OF_CONDUCT.md](./CODE_OF_CONDUCT.md)
- [MAINTAINERS.md](./MAINTAINERS.md)

## Contributing

See:

- [CONTRIBUTING.md](./CONTRIBUTING.md)
- [CODE_OF_CONDUCT.md](./CODE_OF_CONDUCT.md)
- [SECURITY.md](./SECURITY.md)

## License

The Apache License 2.0 applies to this repository.
See [LICENSE](./LICENSE).
