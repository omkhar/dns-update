# Packaging

This document uses ASD-STE100 Simplified Technical English.

This repository ships native packaging metadata for:

- Debian-family `.deb` packages
- RPM-family `.rpm` packages
- macOS release archives
- Windows release archives

Linux is the only platform with native OS packages in this repository today.
macOS and Windows ship as signed release archives that bundle the binary,
examples, changelog, license, and native scheduler helpers under `deploy/`.

Both package layouts install:

- `/usr/bin/dns-update`
- the `dns-update(1)` man page under the distro-standard `man1` path
- `/etc/dns-update/dns-update.env`
- `/etc/dns-update/config.example.json` as a shipped sample. The default
  systemd service does not load this file
- `/etc/dns-update/cloudflare.token.example` as a shipped placeholder token file
- hardened systemd units at the distro-standard unit path

Packaged binaries are intentionally not UPX-packed. That keeps the installed
service compatible with the hardened unit settings, including
`MemoryDenyWriteExecute=yes`.

The package intentionally does not install a live `/etc/dns-update/config.json`
or `/etc/dns-update/cloudflare.token`. Create those files before enabling the
timer.

Copy `/etc/dns-update/config.example.json` to `/etc/dns-update/config.json`
and `/etc/dns-update/cloudflare.token.example` to
`/etc/dns-update/cloudflare.token` when bootstrapping a host. The default
systemd service does not read the sample files directly. Replace placeholders
such as `CLOUDFLARE_ZONE_ID` and `CLOUDFLARE_TOKEN`.

For a direct binary run, edit `/etc/dns-update/config.json`.
Set `provider.cloudflare.api_token_file` to `/etc/dns-update/cloudflare.token`.
You can instead export
`DNS_UPDATE_PROVIDER_CLOUDFLARE_API_TOKEN_FILE=/etc/dns-update/cloudflare.token`.
The packaged systemd unit overrides only that field.

`-force-push` is CLI-only.
The packaged `dns-update.env` file has no persistent control for it.
Use it for a one-time refresh or in a custom unit override.

At runtime the packaged service uses `LoadCredential=` to materialize
`/etc/dns-update/cloudflare.token` into a private systemd credential directory.
That runtime file is systemd-managed and may appear with a read-only mode such
as `0400` or `0440`.

Default package targets:

- `amd64`
- `rpi32` for Raspberry Pi OS / Linux ARMv7 (`armhf` / `armv7hl`)
- `rpi64` for Raspberry Pi OS / Linux ARM64 (`arm64` / `aarch64`)

## Supported helper interface

Use no target argument to build all default package targets.
Use `amd64`, `rpi32`, or `rpi64` to select package targets.
`build-packages.sh` passes its target arguments to both package builders.

The package builders accept these common controls:

| Variable | Function |
| --- | --- |
| `DNS_UPDATE_RELEASE_GOFLAGS` | Replace the release Go build flags. |
| `DNS_UPDATE_RELEASE_LDFLAGS` | Replace the release Go linker flags. |
| `GOTOOLCHAIN` | Replace the Go toolchain selection for native tests. |
| `PACKAGING_SKIP_BUILDDEPS=1` | Skip native package build-dependency checks. |
| `PACKAGING_SKIP_NATIVE_TESTS=1` | Skip the native Go test run. |
| `PACKAGING_SKIP_SIGN=1` | Skip package blob signing. |
| `SOURCE_DATE_EPOCH` | Set the reproducible source timestamp. |

`build-deb.sh` also accepts these controls:

| Variable | Function |
| --- | --- |
| `DEB_BUILD_OPTIONS` | Add Debian build options. |
| `DEB_OUTPUT_DIR` | Replace the Debian output directory. |
| `DEB_RELEASE` | Replace the package release. |
| `DEB_VERSION` | Replace the package version. |
| `PACKAGING_FORCE_DIRECT_DEB=1` | Use direct `dpkg-deb` assembly. |

`build-rpm.sh` also accepts these controls:

| Variable | Function |
| --- | --- |
| `PACKAGING_LINUX_MACROS=1` | Use Linux paths and GNU tools on macOS. |
| `RPM_CANONICAL_TOPDIR_ROOT` | Replace the stable RPM build root. |
| `RPM_OUTPUT_DIR` | Replace the RPM output directory. |
| `RPM_RELEASE` | Replace the package release. |
| `RPM_TOPDIR_ROOT` | Replace the RPM work directory. |
| `RPM_VERSION` | Replace the package version. |

`build-release-assets.sh` accepts these controls:

| Variable | Default | Function |
| --- | --- | --- |
| `RELEASE_OUTPUT_DIR` | `out/release` | Replace the asset output directory. |
| `RELEASE_SKIP_ARCHIVES` | `0` | Set `1` to skip archives. |
| `RELEASE_SKIP_PACKAGES` | `0` | Set `1` to skip native packages. |
| `RELEASE_TMP_DIR` | `out/release-tmp` | Replace the work directory. |
| `RELEASE_VERSION` | package metadata | Replace the release version. |
| `RELEASE_WRITE_CHECKSUMS` | `1` | Set `0` to skip `checksums.txt`. |

`build-remote-container.sh` accepts these command options:

| Option | Default | Function |
| --- | --- | --- |
| `--host HOST` | required | Select the SSH Docker host. |
| `--mode MODE` | `release-assets` | Use `release-assets` or `reproducibility`. |
| `--output-dir DIR` | generated local path | Replace the local result path. |
| `--image IMAGE` | module Go image | Replace the direct base image. |
| `--bootstrap-image IMAGE` | unset | Install Go in this cached Debian image. |
| `--image-tag TAG` | generated | Replace the remote image tag. |
| `--keep-remote-root` | disabled | Keep the remote work directory. |
| `--rebuild-image` | disabled | Build the image without layer cache. |
| `-h` or `--help` | disabled | Print usage and exit with status 2. |

The remote helper accepts these environment variables:

| Variable | Function |
| --- | --- |
| `REMOTE_BUILD_HOST` | Set the SSH Docker host. |
| `REMOTE_BUILD_MODE` | Set `release-assets` or `reproducibility`. |
| `REMOTE_BUILD_IMAGE` | Set the direct base image. |
| `REMOTE_BUILD_BOOTSTRAP_IMAGE` | Set the cached Debian bootstrap image. |
| `REMOTE_BUILD_TAG` | Set the remote image tag. |
| `REMOTE_BUILD_REBUILD_IMAGE=1` | Build the image without layer cache. |
| `REMOTE_BUILD_LOCAL_CONFIG` | Select the optional local defaults file. |

The local defaults file accepts only the six build-value variables.
It does not accept `REMOTE_BUILD_LOCAL_CONFIG`.

`check-release-reproducibility.sh` accepts no arguments.
It builds two asset sets and compares their `checksums.txt` files.
It sets direct package defaults that callers can override.

`verify-artifacts.sh` accepts one or more artifact paths.
Use `COSIGN_KEY` for key-based verification.
For keyless verification, set `SIGSTORE_CERTIFICATE_IDENTITY` and `SIGSTORE_OIDC_ISSUER`.

`verify-release-assets.sh` accepts one or more release artifact paths.
It checks the expected payload for archives, Debian packages, and RPM packages.
It ignores adjacent Sigstore bundles, SPDX files, and checksum files.
It rejects an unsupported artifact suffix.

The build helpers write artifacts under:

- `out/packages/deb/<target>/`
- `out/packages/rpm/<target>/`
- `out/release/` for signed release assets and unsigned local release-asset
  builds

`./packaging/build-packages.sh` runs the native test suite once, then invokes
both package builders with `PACKAGING_SKIP_NATIVE_TESTS=1` so the package loops
do not rerun the same native tests.

GitHub Actions runs package creation in two places:

- `Package Validation` builds the cross-platform release archives on pull
  requests and validates package/archive payloads on `main` pushes without
  publishing or signing them.
- The tag-driven `Release` workflow rebuilds the same package and archive formats.
- It generates an SPDX SBOM and GitHub artifact attestations.
- It signs and verifies the files with Sigstore.
- It puts the assets in a draft release.
- It then publishes the payload and the `*.sigstore.json` bundles.

To rebuild an already tagged release from the GitHub-hosted builder, run the
`Release` workflow manually and pass the existing tag plus
`rebuild_existing_release=true`. For example:

```sh
gh workflow run release.yml --ref main \
  -f release_tag=v1.4.4 \
  -f rebuild_existing_release=true
```

That manual rebuild path checks out the requested tag before building. Prefer a
new release tag when you need tag-aligned provenance for a public reissue.
An older tag rebuild does not change GitHub's Latest release.
It changes this label only when that tag is still the newest version.

Build the full unsigned local release asset set with:

```sh
./packaging/build-release-assets.sh
```

That produces:

- Linux `.deb` packages for `amd64`, `arm64`, and `armhf`
- Linux `.rpm` packages for `x86_64`, `aarch64`, and `armv7hl`
- Linux archives for `amd64`, `arm64`, and `armv7`
- macOS archives for `amd64` and `arm64`
- Windows archives for `amd64` and `arm64`

Build the same Linux packaging and cross-platform archive set on a remote Linux
Docker host inside a dedicated container with:

```sh
./packaging/build-remote-container.sh --host builder@example-build-host
```

That wrapper:

- streams a fresh source snapshot to the remote host
- builds a dedicated remote image tag for that run, reusing Docker layer cache
  unless you pass `--rebuild-image`
- runs the build as the remote login UID/GID so bind-mounted outputs remain
  removable by that shared account
- carries `SOURCE_DATE_EPOCH` into the container so release timestamps stay
  stable
- copies the remote `out/` tree back under `out/remote/<run-id>/` locally

If you keep local defaults in `packaging/build-remote-container.local.env`,
that file now accepts only literal `KEY=VALUE` entries for:

- `REMOTE_BUILD_HOST`
- `REMOTE_BUILD_MODE`
- `REMOTE_BUILD_IMAGE`
- `REMOTE_BUILD_BOOTSTRAP_IMAGE`
- `REMOTE_BUILD_TAG`
- `REMOTE_BUILD_REBUILD_IMAGE`

The wrapper no longer shell-sources that file.
It rejects shell syntax, command substitutions, and unrelated environment keys.

Use the remote wrapper for the release-asset and reproducibility lanes.
It intentionally does not run `packaging/test-systemd-timer.sh`, because that
integration test already drives privileged Docker containers against the remote
host daemon.

The remote Docker daemon can fail to pull the default Go image.
In that case, use a cached Debian-based bootstrap image.
The wrapper installs the exact Go toolchain and package tools.

```sh
./packaging/build-remote-container.sh --host builder@example-build-host \
  --bootstrap-image node:22-trixie-slim
```

Check that two consecutive full release-asset builds are reproducible with:

```sh
./packaging/check-release-reproducibility.sh
```

That check uses the trusted release package path.
Install local `dpkg-deb`, `rpmbuild`, `zip`, and `unzip` tools.

Run the same reproducibility check inside the remote container wrapper with:

```sh
./packaging/build-remote-container.sh --host builder@example-build-host --mode reproducibility
```

## Debian build

Requirements:

- `dpkg-buildpackage` plus `debhelper-compat (= 13)` for the native Debian
  packaging path, or `dpkg-deb` for the direct fallback path
- `cosign`
- `golang-any`

Build:

```sh
./packaging/build-deb.sh
```

Build one target explicitly:

```sh
./packaging/build-deb.sh amd64
./packaging/build-deb.sh rpi32
./packaging/build-deb.sh rpi64
```

The Debian wrapper runs `go test ./...` once natively, then cross-builds each
package with `DEB_BUILD_OPTIONS=nocheck`.

When `dh` is unavailable, the wrapper uses direct `dpkg-deb` assembly.
This path produces the same installed payload.
The native Debian path also emits `.buildinfo` and `.changes` files.

Set `PACKAGING_FORCE_DIRECT_DEB=1` to force the direct `dpkg-deb` path even
when the host has `dh`.

Release package builds use Go release-oriented flags only for the package build
step: `-mod=readonly -trimpath -buildvcs=false` plus
`-ldflags='-s -w -buildid= -X dns-update/internal/buildinfo.Version=<version>'`.
Native test and normal development builds keep their existing defaults.

## RPM build

Requirements:

- `rpmbuild`
- `cosign`
- `golang >= 1.26.5`
- `tar` or `gtar` (prefer GNU tar on macOS)

Build:

```sh
./packaging/build-rpm.sh
```

Override the default version and release if needed:

```sh
RPM_VERSION=1.4.4 RPM_RELEASE=1 ./packaging/build-rpm.sh
```

Build both formats in one pass:

```sh
./packaging/build-packages.sh
```

Build one target explicitly:

```sh
./packaging/build-rpm.sh amd64
./packaging/build-rpm.sh rpi32
./packaging/build-rpm.sh rpi64
```

The RPM wrapper runs `go test ./...` once natively, then cross-builds each
package with `rpmbuild --without check`.

On macOS with Homebrew `rpmbuild`, set `PACKAGING_LINUX_MACROS=1` to force
Linux-style filesystem macros and prepend GNU coreutils where needed:

```sh
PACKAGING_LINUX_MACROS=1 ./packaging/build-rpm.sh
```

GitHub Actions runs `packaging/test-systemd-timer.sh` on six Linux images.
The images cover stable and unstable Debian, Ubuntu, and Fedora versions.
Each test uses the package for its distribution family.
The test skips the first service activation.
It then proves that a later timer activation succeeds.

Separate native scheduler integration jobs validate:

- `deploy/launchd/install-launchd-job.sh` on `macos-26`
- `deploy/windows/register-scheduled-task.ps1` on `windows-2025`

Those macOS and Windows jobs run an install-time config-validation preflight
and then prove a later scheduler-fired invocation uses the installed
non-validation action.

For local runs, `packaging/test-systemd-timer.sh` requires Docker and currently
supports amd64 and arm64 hosts.

Release package builds use Go release-oriented flags only for the package build
step: `-mod=readonly -trimpath -buildvcs=false` plus
`-ldflags='-s -w -buildid= -X dns-update/internal/buildinfo.Version=<version>'`.
Native test and normal development builds keep their existing defaults.

## Sigstore signing

The package helpers sign each generated `.deb` and `.rpm` with
`cosign sign-blob`. They write a Sigstore bundle next to the artifact as
`*.sigstore.json`.

This process uses detached blob signing. If you inspect an RPM directly with
`rpm -qip`, the header signature field still shows `Signature: (none)`.
The adjacent Sigstore bundle contains the attestation.

Default signing mode is keyless. That follows the Sigstore blob-signing flow and
requires an identity that Cosign can use.

If keyless auth is not available on the local build host, sign with a managed
key by setting `COSIGN_KEY`.

Verify an artifact with:

```sh
SIGSTORE_CERTIFICATE_IDENTITY=dns-update@omkhar.net \
SIGSTORE_OIDC_ISSUER=https://accounts.google.com \
./packaging/verify-artifacts.sh out/packages/deb/amd64/dns-update_1.4.4-1_amd64.deb
```

Or with a key:

```sh
COSIGN_KEY=cosign.pub \
./packaging/verify-artifacts.sh out/packages/rpm/amd64/dns-update-1.4.4-1.x86_64.rpm
```

Validate the expected payload layout of built archives and packages with:

```sh
./packaging/verify-release-assets.sh out/release/*.tar.gz out/release/*.zip
./packaging/verify-release-assets.sh out/packages/deb/amd64/*.deb
./packaging/verify-release-assets.sh out/packages/rpm/amd64/*.rpm
```

## Maintainer metadata

The Debian and RPM metadata currently use a generic maintainer identity. Update
that metadata before publishing packages outside your own infrastructure.
