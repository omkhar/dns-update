# Changelog

All notable public releases of `dns-update` are documented in this file.

## 1.3.6 - 2026-03-22

- Refreshes the README, packaged docs, and `dns-update(1)` man page so they
  match the current CLI behavior and scheduler guidance.
- Clarifies that `-validate-config` and `-print-effective-config` still load
  and validate the assembled configuration, including the provider token-file
  path, while avoiding provider initialization and Cloudflare calls.
- Restores the missing `1.3.4` release notes in the public/package changelogs
  and updates the runtime user-agent and package metadata to `1.3.6`.

## 1.3.5 - 2026-03-22

- Hardens GitHub release publishing to stage assets on a draft release before
  publication and adds an explicit GitHub-hosted rebuild path for an existing
  tag.
- Rolls the release line forward to `1.3.5`.
- Updates the runtime user-agent and package metadata to `1.3.5`.

## 1.3.4 - 2026-03-22

- Revalidates the repository against the current stable Go release, which is
  still `1.26.1` as of 2026-03-22.
- Rolls the release line forward to `1.3.4`.
- Updates the runtime user-agent and package metadata to `1.3.4`.

## 1.3.3 - 2026-03-22

- Fixes GitHub CLI authentication in the release attestation verification
  steps.
- Reissues the `1.3.2` release line as `1.3.3` after the failed tag publish.
- Updates the runtime user-agent and package metadata to `1.3.3`.

## 1.3.2 - 2026-03-22

- Reworks GitHub Actions into fast PR, nightly, and release lanes instead of
  running the full quality stack on every pull request.
- Adds release SBOM generation, GitHub artifact attestations, signature
  verification, and release-archive reproducibility checks.
- Updates the runtime user-agent and package metadata to `1.3.2`.

## 1.3.1 - 2026-03-21

- Fixes the OSV scanner workflow YAML so the tag-driven release pipeline passes
  its lint gate again.
- Reissues the `1.3` release line as `1.3.1` after the failed `1.3.0` asset
  publish.
- Updates the runtime user-agent and package metadata to `1.3.1`.

## 1.3.0 - 2026-03-21

- Adds a CLI-only `--delete` mode that removes the managed `A`, `AAAA`, or
  both address-record families for the configured hostname. Bare `--delete`
  deletes both families and skips egress probing.
- Uses a dedicated provider delete planner and verifier so single-family
  deletion does not reconcile or rewrite untouched records.
- Updates the runtime user-agent, package metadata, and operator documentation
  for the `1.3.0` release.

## 1.2.0 - 2026-03-21

- Adds a CLI-only `--force-push` flag that refreshes matching DNS records even
  when the observed egress IPs have not changed.
- Updates the runtime user-agent and package metadata to `1.2.0`.
- Refreshes the release, packaging, and deployment documentation for the new
  flag and version bump.

## 1.1.0 - 2026-03-20

- Removes dead runtime plumbing in config loading, flag parsing, and effective
  config printing.
- Adds a repository-wide `CODEOWNERS` rule for `@omkhar` so owner review is the
  default review path for all files.
- Updates the default-branch protection policy so code-owner review is required
  while the single repository owner can still merge their own pull requests.

## 1.0.4 - 2026-03-19

- Publishes signed macOS and Windows release archives alongside the existing
  signed Linux packages and Linux tarballs.
- Validates native scheduled execution on macOS `launchd` and Windows Task
  Scheduler in GitHub Actions before release publishing.
- Clarifies the cross-platform deployment and packaging documentation for the
  shipped release asset set.

## 1.0.3 - 2026-03-19

- Stops UPX-packing packaged binaries so the shipped systemd service remains
  compatible with `MemoryDenyWriteExecute=yes` on distros such as Debian 12.
- Extends the multi-distro systemd integration test to install and exercise the
  built `.deb` and `.rpm` packages, not just a raw development binary.

## 1.0.2 - 2026-03-19

- Switches the packaged systemd timer to `OnCalendar=*:00/5` so future runs stay
  scheduled even if the first activation is skipped by unmet unit conditions.
- Extends the systemd integration test to verify the timer keeps a queued future
  run after an initial condition-check skip and later succeeds from a real
  timer-fired activation.
- Publishes signed `.deb` and `.rpm` release assets from the GitHub-hosted
  release builder in addition to the signed Linux tarballs.
- Adds a separate package-validation workflow so package creation is exercised
  before release publishing.

## 1.0.1 - 2026-03-19

- Accepts systemd-managed credential files that surface with read-only modes
  such as `0440` under `$CREDENTIALS_DIRECTORY`.
- Falls back to `/etc/dns-update/config.json` for implicit CLI runs when no
  local `config.json` is present.
- Adds a multi-distro systemd timer integration workflow covering Debian
  stable/sid, Ubuntu stable/latest, and Fedora stable/rawhide.
- Clarifies the systemd credential and packaging documentation for the runtime
  token path and release validation flow.

## 1.0 - 2026-03-18

- Initial public release.
- Reconciles Cloudflare `A` and `AAAA` records against observed egress IPs.
- Ships strict config validation, secure token-file handling, bounded retries,
  and hardened systemd deployment examples.
