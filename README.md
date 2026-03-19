# dns-update

`dns-update` is a Go service that keeps one hostname's `A` and `AAAA` records
aligned with the host's current egress IPv4/IPv6 addresses.

The current implementation targets Cloudflare through its DNS Records API and is
structured so additional providers can be added behind the same internal
provider interface.

## Behavior

On each run, the service:

1. Fetches probe responses from:
   - `probe.ipv4_url` (default `https://4.ip.omsab.net/`)
   - `probe.ipv6_url` (default `https://6.ip.omsab.net/`)
2. Parses responses in `ip=...` format.
3. Validates returned addresses by family:
   - IPv4 probe must yield a valid IPv4 or `ip=none`
   - IPv6 probe must yield a valid IPv6 or `ip=none`
4. Reads the current provider-side records for `record.name`.
5. Compares desired vs current DNS state:
   - If already matching, exits without update.
   - If different, applies only the required record create/update/delete
     operations.
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
- `probe.allow_insecure_http` (optional): defaults to `false`. HTTP probe URLs
  are only accepted for loopback or `localhost` test endpoints.
- `provider.type` (required): currently `cloudflare`.
- `provider.timeout` (optional): Go duration string, defaults to `10s`.
- `provider.cloudflare.zone_id` (required): Cloudflare zone ID for the managed
  zone.
- `provider.cloudflare.api_token_file` (required): file containing only the
  Cloudflare API token.
- `provider.cloudflare.base_url` (optional): defaults to
  `https://api.cloudflare.com/client/v4/`. Overrides are limited to the default
  Cloudflare API host or loopback or `localhost` test endpoints.
- `provider.cloudflare.proxied` (optional): whether Cloudflare should proxy the
  managed A/AAAA records. Defaults to `false`.

See `config.example.json` for a complete sample. The shipped sample remains
loadable as-is, but placeholder values such as `CLOUDFLARE_ZONE_ID` and
`CLOUDFLARE_TOKEN` must be replaced before production use.

## Configuration Sources

The app reads record, probe, and provider settings from the JSON config file.
Runtime options have a separate small override surface.

Runtime settings:

- `-config` or `DNS_UPDATE_CONFIG`
- `-dry-run` or `DNS_UPDATE_DRY_RUN`
- `-verbose` or `DNS_UPDATE_VERBOSE`
- `-timeout` or `DNS_UPDATE_TIMEOUT`
- `DNS_UPDATE_PROVIDER_CLOUDFLARE_API_TOKEN_FILE` to override only
  `provider.cloudflare.api_token_file`, which is primarily useful for systemd
  credentials

Record and provider settings otherwise come from JSON config file fields.

Behavior notes:

- If `-config` or `DNS_UPDATE_CONFIG` is set, that path is required and must
  exist.
- If neither is set, the app falls back to `config.json`.
- Built-in defaults still apply for optional unset values such as probe URLs,
  timeouts, and the Cloudflare base URL.

## Security Notes

- The codebase keeps the dependency surface intentionally small and prefers
  reviewed packages over broad frameworks.
- No inline secrets in config; store the Cloudflare API token in a separate
  file.
- Restrict the token file permissions (for example `chmod 600`).
- Keep the token file in a non-writable directory; the app rejects token paths
  whose parent directory is writable by group or other users.
- The token file itself must not be a symlink, its direct parent directory must
  not be a symlink, and on Unix-like systems the token file is opened without
  following symlinks, then revalidated at read time.
- Use HTTPS probe URLs unless `probe.allow_insecure_http` is explicitly needed.
- Probe URL overrides are restricted to the shipped `4.ip.omsab.net` and
  `6.ip.omsab.net` hosts or loopback or `localhost` test endpoints.
- Enabling `probe.allow_insecure_http` expands that risk further by allowing
  on-path tampering of probe responses, so HTTP is restricted to loopback or
  `localhost` test endpoints.
- Scope the Cloudflare token to the single zone being managed.
- Cloudflare record reads are filtered to the managed hostname instead of
  listing the full zone.
- Overriding `provider.cloudflare.base_url` changes where the Cloudflare bearer
  token is sent, so the app accepts only the default Cloudflare API host or
  loopback or `localhost` test endpoints.
- Probe and provider HTTP clients use a fixed custom user-agent, ignore ambient
  proxy environment variables, and apply bounded retries that honor
  `Retry-After` when present.

## Toolchain

Build and test with a patched Go toolchain. The module now requires Go `1.26.1`.

## Dependencies

Runtime dependencies are deliberately narrow:

- `github.com/cloudflare/cloudflare-go/v6` for the Cloudflare DNS API
- `golang.org/x/sync/errgroup` for structured concurrency
- `github.com/google/go-cmp/cmp` is used in tests only

There is no separate `golang.org/x/time/rate` dependency in the current build;
outbound request pacing is handled by the code in this repository.

## Cloudflare Token Scope

Because the config requires `provider.cloudflare.zone_id`, the app does not need
to discover the zone through the Cloudflare API. For minimum privilege, create a
Cloudflare API token that is limited to the target zone and grants only DNS edit
capability for that zone.

## Build and Run

Build the binary:

```sh
go build ./cmd/dns-update
```

Run one reconciliation cycle:

```sh
./dns-update -config /etc/dns-update/config.json
```

Cap the entire run, including retries and backoff:

```sh
./dns-update -config /etc/dns-update/config.json -timeout 30s
```

Preview planned changes without applying them:

```sh
./dns-update -config /etc/dns-update/config.json -dry-run
```

## systemd

Example hardened systemd units live in `deploy/systemd/`.

- `deploy/systemd/dns-update.service` runs one reconciliation with a locked-down
  `DynamicUser`, no ambient capabilities, a read-only filesystem view, and a
  private systemd credential for the Cloudflare token.
- `deploy/systemd/dns-update.timer` starts the service immediately at boot and
  then reruns it every five minutes.
- `deploy/systemd/dns-update.env` shows how to override runtime options
  such as `DNS_UPDATE_TIMEOUT` without editing the unit.

The service expects:

- `/usr/bin/dns-update`
- `/etc/dns-update/config.json`
- `/etc/dns-update/cloudflare.token`

The token is mounted into the service with `LoadCredential=` and exposed to the
binary through
`DNS_UPDATE_PROVIDER_CLOUDFLARE_API_TOKEN_FILE=%d/cloudflare.token`, so the
credential never needs to be stored in the JSON config path used by the unit.
On some systems the runtime credential file may appear with a read-only mode
such as `0400` or `0440`; that is expected for systemd-managed credentials and
does not require any manual chmod under `/run/credentials/`.

See `deploy/systemd/README.md` for installation steps.

## Packages

Native package metadata lives in:

- `debian/` for Debian-family builds
- `packaging/rpm/dns-update.spec` for RPM-family builds
- `deploy/systemd/` for the shared systemd units and env file used by both
  manual installs and native packages

Package builds install:

- `/usr/bin/dns-update`
- `/etc/dns-update/dns-update.env`
- `/etc/dns-update/config.example.json` as a shipped sample that is not loaded
  by default
- `/etc/dns-update/cloudflare.token.example` as a shipped placeholder token file
- distro-standard systemd units for `dns-update.service` and `dns-update.timer`

Build helpers:

```sh
./packaging/build-deb.sh
./packaging/build-rpm.sh
./packaging/build-packages.sh
```

Those wrappers build and sign the default release targets:

- `amd64`
- `rpi32`
- `rpi64`

Before enabling the packaged timer, create:

- `/etc/dns-update/config.json`
- `/etc/dns-update/cloudflare.token`

The packaged `/etc/dns-update/config.example.json` and
`/etc/dns-update/cloudflare.token.example` are there as starting points only.
The default systemd service still reads `/etc/dns-update/config.json` and the
credential-backed `/etc/dns-update/cloudflare.token`.

See `packaging/README.md` for package build requirements and notes.

## Release Notes

See [CHANGELOG.md](./CHANGELOG.md) for public release history.

## Quality Checks

`go test ./...` runs the normal unit and integration suite and also enforces two
repository-level quality gates:

- a coverage check that fails unless total statement coverage across `./...` is
  exactly `100.0%`
- a curated mutation suite that copies the repository into temporary workspaces,
  applies compile-preserving mutants, and requires the test suite to kill each
  mutant

The mutation and coverage skip environment variables are only for the nested
subprocesses launched by those tests and normally should remain unset during
regular use.

GitHub Actions additionally runs the dedicated `Systemd Integration` workflow
to validate the installed timer/service flow on:

- Debian Stable
- Debian Sid
- Ubuntu Stable
- Ubuntu Latest
- Fedora Stable
- Fedora Rawhide

## Package Docs

Google-style package comments live alongside the code in:

- `cmd/dns-update`
- `internal/app`
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

This repository is licensed under the Apache License 2.0. See
[LICENSE](./LICENSE).
