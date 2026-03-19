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

At runtime the packaged service uses `LoadCredential=` to copy
`/etc/dns-update/cloudflare.token` into a private systemd credential directory.
That runtime file is systemd-managed and may appear with a read-only mode such
as `0400` or `0440`.

Default package targets:

- `amd64`
- `rpi32` for Raspberry Pi OS / Linux ARMv7 (`armhf` / `armv7hl`)
- `rpi64` for Raspberry Pi OS / Linux ARM64 (`arm64` / `aarch64`)

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

When `dh` is unavailable on the build host, the wrapper falls back to direct
`dpkg-deb` assembly and still produces the `.deb` artifact with the same
installed payload. The native Debian path additionally emits `.buildinfo` and
`.changes` files.

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

Build:

```sh
./packaging/build-rpm.sh
```

Override the default version and release if needed:

```sh
RPM_VERSION=1.0 RPM_RELEASE=1 ./packaging/build-rpm.sh
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

GitHub Actions also runs `packaging/test-systemd-timer.sh` across Debian
stable/sid, Ubuntu stable/latest, and Fedora stable/rawhide to validate the
installed timer/service flow on each distro family.

Release package builds use Go release-oriented flags only for the package build
step: `-mod=readonly -trimpath -buildvcs=false` plus
`-ldflags='-s -w -buildid='`. Native test and normal development builds keep
their existing defaults.

## Sigstore signing

Each generated `.deb` and `.rpm` is signed with `cosign sign-blob` and a
Sigstore bundle written next to the artifact as `*.sigstore.json`.

Default signing mode is keyless. That follows the Sigstore blob-signing flow and
requires an identity that Cosign can use.

You can also sign with a managed key by setting `COSIGN_KEY`.

Verify an artifact with:

```sh
SIGSTORE_CERTIFICATE_IDENTITY=you@example.com \
SIGSTORE_OIDC_ISSUER=https://accounts.google.com \
./packaging/verify-artifacts.sh out/packages/deb/amd64/dns-update_1.0-1_amd64.deb
```

Or with a key:

```sh
COSIGN_KEY=cosign.pub \
./packaging/verify-artifacts.sh out/packages/rpm/amd64/dns-update-1.0-1.x86_64.rpm
```

## Maintainer metadata

The Debian and RPM metadata currently use a generic maintainer identity. Update
that metadata before publishing packages outside your own infrastructure.
