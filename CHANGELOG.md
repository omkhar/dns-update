# Changelog

All notable public releases of `dns-update` are documented in this file.

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
