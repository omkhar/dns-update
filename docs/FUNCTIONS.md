# Supported functions

This document uses ASD-STE100 Simplified Technical English.

## Scope

This document defines the supported operator and maintainer interface.
It does not make internal Go helpers part of the public interface.
Packages below `internal/` can change without a compatibility promise.

ASD-STE100 Rule 1.12 permits technical names.
This document uses command names, flags, environment variables, JSON fields, paths, and product names as technical names.

## Reconcile mode

Run `dns-update` without a delete or introspection flag.
The command performs one reconciliation cycle and exits.

The command starts the IPv4 probe and the IPv6 probe concurrently.
It also reads the current Cloudflare state.
Each probe uses its specified IP network.
The IPv4 probe uses `tcp4`.
The IPv6 probe uses `tcp6`.

Each probe must return HTTP status 200.
The response must contain `ip=<address>` or `ip=none`.
The command reads at most 64 response bytes.
The command rejects redirects.
The command rejects an address from the wrong family.

The command compares each observed family with its DNS records.
It creates a missing record.
It updates one nonmatching record.
It deletes duplicate records.
It deletes all records for an explicit `ip=none` response.
It leaves an unobserved family unchanged in partial-failure mode.

The command rejects a CNAME at the managed name during reconciliation.
It does not change records at another name.
It does not change record types other than A and AAAA.

After a change, the command reads the provider state again.
It verifies the address, TTL, proxy value, record count, and CNAME state.
A verification difference makes the run fail.

## Delete mode

Use `-delete=a` to delete all A records at the managed name.
Use `-delete=aaaa` to delete all AAAA records at the managed name.
Use `-delete=both` to delete both record families.
Bare `-delete` also selects both families.
The value `true` also selects both families.
The values `false` and `none` disable delete mode.
The delete value is not case-sensitive.

Delete mode does not run the egress probes.
Delete mode leaves CNAME and other record types unchanged.
Delete mode verifies that each selected family is absent.
`-delete` and `-force-push` are mutually exclusive.

## Dry-run mode

Use `-dry-run` to print the planned operations.
Dry-run mode does not send a provider mutation.
It still loads config, validates local files, probes, and reads provider state.
You can combine `-dry-run` with reconcile, delete, or force-push mode.

## Force-push mode

Use `-force-push` to refresh a matching managed record.
The command still requires valid probe evidence.
The command updates only an observed family that has an existing address record.
It does not create a missing record through the force-only path.
Normal reconciliation still creates a missing record.

## Introspection modes

Use `-validate-config` to validate the assembled config.
The command prints `config is valid` and exits.
It does not initialize the provider.
It does not contact Cloudflare.
It still validates the local token-file path and protections.

Use `-print-effective-config` to print the assembled config as JSON.
The output includes the token-file path.
The output does not include token contents.
This mode has the same local validation behavior as `-validate-config`.

Use `-version` or `--version` to print the product version.
Version mode does not load config.

`-validate-config` and `-print-effective-config` are mutually exclusive.

## Command flags

| Flag | Default | Function |
| --- | --- | --- |
| `-config PATH` | unset | Select the JSON config file. |
| `-delete[=a\|aaaa\|both]` | unset | Select delete mode and the record families. |
| `-dry-run` | `false` | Print the plan without a provider mutation. |
| `-force-push` | `false` | Refresh matching existing address records. |
| `-print-effective-config` | `false` | Print the assembled config and exit. |
| `-timeout DURATION` | `0` | Limit one reconciliation or delete cycle. Zero disables this limit. |
| `-validate-config` | `false` | Validate the assembled config and exit. |
| `-verbose` | `false` | Enable debug logs. |
| `-version` | `false` | Print version information and exit. |

The Go flag parser also accepts the double-hyphen form.
Use `-h` or `-help` to print flag help.
Help exits with status 0.

## Runtime environment variables

| Variable | Default | Function |
| --- | --- | --- |
| `DNS_UPDATE_CONFIG` | unset | Select the JSON config file. |
| `DNS_UPDATE_DRY_RUN` | `false` | Set dry-run mode. |
| `DNS_UPDATE_PROVIDER_CLOUDFLARE_API_TOKEN_FILE` | unset | Replace only the token-file path. |
| `DNS_UPDATE_TIMEOUT` | `0` | Set the reconciliation or delete cycle timeout. |
| `DNS_UPDATE_VERBOSE` | `false` | Set verbose logging. |

An explicit command flag has precedence over its runtime environment variable.
The environment variable has precedence over the built-in runtime default.
The token-file environment variable has precedence over its JSON field.

Boolean environment values accept these true values:
`1`, `t`, `true`, `y`, and `yes`.
They accept these false values:
`0`, `f`, `false`, `n`, and `no`.
The comparison is not case-sensitive.

## Config file selection

An explicit `-config` value has first precedence.
`DNS_UPDATE_CONFIG` has second precedence.
The current-directory `config.json` has third precedence.
`/etc/dns-update/config.json` has last precedence.

An explicit empty path is an error.
An explicit missing file is an error.
The loader rejects unknown JSON fields.
The loader rejects data after the first JSON value.

## JSON config fields

| Field | Default | Validation and function |
| --- | --- | --- |
| `record.name` | required | Set the managed FQDN. |
| `record.zone` | required | Set the FQDN zone. The record must be in this zone. |
| `record.ttl_seconds` | required | Set the TTL. Use 1 or 30 through 86400 for Cloudflare. |
| `probe.ipv4_url` | `https://4.ip.omsab.net/` | Set the IPv4 egress probe. |
| `probe.ipv6_url` | `https://6.ip.omsab.net/` | Set the IPv6 egress probe. |
| `probe.timeout` | `10s` | Set each probe HTTP timeout. Use a positive Go duration. |
| `probe.allow_insecure_http` | `false` | Permit HTTP only for loopback or `localhost`. |
| `probe.allow_partial_failure` | `false` | Permit one successful family to reconcile alone. |
| `provider.type` | required | Select the provider. Only `cloudflare` is valid. |
| `provider.timeout` | `10s` | Set the Cloudflare HTTP timeout. Use a positive Go duration. |
| `provider.cloudflare.zone_id` | required | Set a 32-character hexadecimal zone ID. |
| `provider.cloudflare.api_token_file` | required | Set the file that contains one API token. |
| `provider.cloudflare.base_url` | `https://api.cloudflare.com/client/v4/` | Set the Cloudflare API base URL. |
| `provider.cloudflare.proxied` | `false` | Set the Cloudflare proxy value for managed records. |

The placeholder `CLOUDFLARE_ZONE_ID` is valid for local config validation.
A relative token path is relative to the config directory.
The token-file environment override resolves a relative path from the working directory.

The loader trims and lowercases each DNS name.
It adds a final dot when it is absent.
It permits DNS letters, numbers, hyphens, underscores, and a full wildcard label.
It rejects whitespace, empty labels, long labels, and names longer than 253 bytes.

Probe URL overrides can use only the applicable default host or a loopback host.
The Cloudflare base URL can use only the default host or a loopback host.
These URLs cannot contain user information, a query, or a fragment.
The Cloudflare base URL must use HTTPS.

The token path must identify a regular file.
The path cannot contain a user-controlled symlink.
On Unix, group and other users cannot access a normal token file.
On Unix, group and other users cannot write its parent directory.
A systemd-managed credential can have mode `0440`.
The program accepts this mode only in the active systemd credential directory.
On Windows, the program checks the corresponding file and directory ACLs.
The token file can contain at most 4096 bytes.
It must contain exactly one nonempty token.

## Provider and record behavior

Cloudflare is the only provider.
The client requests records by the exact managed name.
It reads all result pages with 100 records per page.
It sends all planned mutations through one Cloudflare batch request.

The desired record state includes the address, TTL, and proxy value.
The planner keeps one matching record and deletes duplicates.
If no record matches, the planner updates the first stable record.
The planner sorts operations for stable logs and tests.

## Retry, signal, and error behavior

The command retries transient probe and provider failures.
It makes at most five attempts.
The initial delay is 500 milliseconds.
The delay uses bounded exponential backoff and jitter.
The maximum delay is 30 seconds.
The command honors valid `Retry-After` and `Retry-After-Ms` values within that limit.

Retryable HTTP status codes are 408, 425, 429, 500, 502, 503, and 504.
The command also retries eligible network and truncated-response errors.
It does not retry a redirect.

`-timeout` covers probes, provider calls, retries, and backoff.
SIGINT and SIGTERM cancel the cycle.
A cancellation or timeout makes an incomplete run fail.

The command writes normal and debug logs to standard output.
It writes command errors and flag errors to standard error.

The process uses these exit codes:

- Exit code `0` means success or requested help.
- Exit code `1` means a config, output, provider, reconciliation, or verification failure.
- Exit code `2` means a flag or runtime-option error.

## Scheduler and deployment helpers

The binary does not contain a scheduler.
Each scheduler starts a new one-cycle process.

- `deploy/launchd/install-launchd-job.sh` installs or replaces a macOS LaunchDaemon.
- `deploy/systemd/dns-update.service` defines the hardened Linux one-shot service.
- `deploy/systemd/dns-update.timer` runs the Linux service every five minutes.
- `deploy/systemd/dns-update.env` supplies optional Linux runtime overrides.
- `deploy/windows/register-scheduled-task.ps1` installs or replaces the Windows task.
- `deploy/windows/invoke-dns-update.ps1` runs the Windows command and appends its log.

Read the platform README before you use a deployment helper.
The macOS installer supports an optional validation preflight.
The Windows installer supports an optional validation preflight.
Neither preflight changes the installed recurring action.

## Package and release helpers

These scripts are supported maintainer entry points:

- `packaging/build-deb.sh` builds Debian packages for selected targets.
- `packaging/build-rpm.sh` builds RPM packages for selected targets.
- `packaging/build-packages.sh` builds both native package formats.
- `packaging/build-release-assets.sh` builds the complete release asset set.
- `packaging/build-remote-container.sh` builds assets on a remote Docker host.
- `packaging/check-release-reproducibility.sh` compares two complete asset builds.
- `packaging/verify-artifacts.sh` verifies adjacent Sigstore bundles.
- `packaging/verify-release-assets.sh` verifies package and archive payloads.

Read `packaging/README.md` for requirements, targets, options, environment controls, and signing modes.
`packaging/lib.sh` is an internal sourced library.
Files named `packaging/test-*` are internal integration tests.

## Limitations

Read `docs/LIMITATIONS.md` before deployment.
The principal operational limitations are:

- The tool supports only Cloudflare.
- The tool manages only A and AAAA records for one name.
- The tool does not provide a distributed lock.
- The tool does not contain a scheduler.
- Native packages support Linux only.
- Config validation does not validate live credentials.
- Provider and network tests use local doubles.
- A required probe failure stops reconciliation.
- The workflow contract parser accepts a restricted YAML form.
