# Packaging

This repository ships native packaging metadata for:

- Debian-family `.deb` packages
- RPM-family `.rpm` packages

Both package layouts install:

- `/usr/bin/dns-update`
- `/etc/dns-update/dns-update.env`
- `/etc/dns-update/config.example.json` as a shipped sample that is not loaded
  by the default systemd service
- `/etc/dns-update/cloudflare.token.example` as a shipped placeholder token file
- hardened systemd units at the distro-standard unit path

The package intentionally does not install a live `/etc/dns-update/config.json`
or `/etc/dns-update/cloudflare.token`. Create those files before enabling the
timer.

Copy `/etc/dns-update/config.example.json` to `/etc/dns-update/config.json`
and `/etc/dns-update/cloudflare.token.example` to
`/etc/dns-update/cloudflare.token` when bootstrapping a host. The default
systemd service does not read the sample files directly, and placeholders such
as `CLOUDFLARE_ZONE_ID` and `CLOUDFLARE_TOKEN` must be replaced.

If you want to run `dns-update` directly outside the packaged systemd unit,
either edit `/etc/dns-update/config.json` so
`provider.cloudflare.api_token_file` points at
`/etc/dns-update/cloudflare.token`, or export
`DNS_UPDATE_PROVIDER_CLOUDFLARE_API_TOKEN_FILE=/etc/dns-update/cloudflare.token`
for that invocation. The packaged systemd unit overrides only that field at
runtime.

At runtime the packaged service uses `LoadCredential=` to materialize
`/etc/dns-update/cloudflare.token` into a private systemd credential directory.
That runtime file is systemd-managed and may appear with a read-only mode such
as `0400` or `0440`.

Default package targets:

- `amd64`
- `rpi32` for Raspberry Pi OS / Linux ARMv7 (`armhf` / `armv7hl`)
- `rpi64` for Raspberry Pi OS / Linux ARM64 (`arm64` / `aarch64`)

Artifacts are written under:

- `out/packages/deb/<target>/`
- `out/packages/rpm/<target>/`

`./packaging/build-packages.sh` runs the native test suite once, then invokes
both package builders with `PACKAGING_SKIP_NATIVE_TESTS=1` so the package loops
do not rerun the same native tests.

GitHub Actions runs package creation in two places:

- `Package Validation` builds `.deb` and `.rpm` artifacts on pull requests and
  `main` pushes without publishing or signing them.
- The tag-driven `Release` workflow rebuilds the same package formats on the
  GitHub-hosted runner, signs those package files with Sigstore, and publishes
  the package files plus their `*.sigstore.json` bundles as release assets.

## Debian build

Requirements:

- `dpkg-buildpackage` plus `debhelper-compat (= 13)` for the native Debian
  packaging path, or `dpkg-deb` for the direct fallback path
- `cosign`
- `golang-any`
- `upx`

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

When `dh` is unavailable on the build host, the wrapper falls back to direct
`dpkg-deb` assembly and still produces the `.deb` artifact with the same
installed payload. The native Debian path additionally emits `.buildinfo` and
`.changes` files.

Set `PACKAGING_FORCE_DIRECT_DEB=1` to force the direct `dpkg-deb` path even
when `dh` is installed.

Release package builds use Go release-oriented flags only for the package build
step: `-mod=readonly -trimpath -buildvcs=false` plus
`-ldflags='-s -w -buildid='`. Native test and normal development builds keep
their existing defaults.

## RPM build

Requirements:

- `rpmbuild`
- `cosign`
- `golang >= 1.26.1`
- `systemd-rpm-macros`
- `upx`
- `tar` or `gtar` (GNU tar is preferred on macOS)

Build:

```sh
./packaging/build-rpm.sh
```

Override the default version and release if needed:

```sh
RPM_VERSION=1.0.2 RPM_RELEASE=1 ./packaging/build-rpm.sh
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

GitHub Actions also runs `packaging/test-systemd-timer.sh` across Debian
stable/sid, Ubuntu stable/latest, and Fedora stable/rawhide to validate the
installed timer/service flow on each distro family, including the regression
where the first activation is skipped before later timer runs are due and a
later timer-fired activation must succeed automatically.

For local runs, `packaging/test-systemd-timer.sh` requires Docker and currently
supports amd64 and arm64 hosts.

Release package builds use Go release-oriented flags only for the package build
step: `-mod=readonly -trimpath -buildvcs=false` plus
`-ldflags='-s -w -buildid='`. Native test and normal development builds keep
their existing defaults.

## Sigstore signing

Each generated `.deb` and `.rpm` is signed with `cosign sign-blob` and a
Sigstore bundle written next to the artifact as `*.sigstore.json`.

This is detached blob signing. If you inspect an RPM directly with
`rpm -qip`, the header signature field will still show `Signature: (none)`;
the attestation lives in the adjacent Sigstore bundle instead.

Default signing mode is keyless. That follows the Sigstore blob-signing flow and
requires an identity that Cosign can use.

If keyless auth is not available on the local build host, sign with a managed
key by setting `COSIGN_KEY`.

Verify an artifact with:

```sh
SIGSTORE_CERTIFICATE_IDENTITY=you@example.com \
SIGSTORE_OIDC_ISSUER=https://accounts.google.com \
./packaging/verify-artifacts.sh out/packages/deb/amd64/dns-update_1.0.2-1_amd64.deb
```

Or with a key:

```sh
COSIGN_KEY=cosign.pub \
./packaging/verify-artifacts.sh out/packages/rpm/amd64/dns-update-1.0.2-1.x86_64.rpm
```

## Maintainer metadata

The Debian and RPM metadata currently use a generic maintainer identity. Update
that metadata before publishing packages outside your own infrastructure.
