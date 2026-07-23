# launchd deployment

This document uses ASD-STE100 Simplified Technical English.

Use `launchd` for native scheduled execution on macOS.

The helper script in this directory writes and loads a `LaunchDaemon` that:

- runs `dns-update` as root
- uses `RunAtLoad` for an immediate startup run
- reruns at a fixed interval with `StartInterval`
- passes the Cloudflare token path and timeout through environment variables
- sends stdout and stderr to a log file

Suggested installed layout:

- `/usr/local/bin/dns-update`
- `/usr/local/etc/dns-update/config.json`
- `/usr/local/etc/dns-update/cloudflare.token`
- `/var/log/dns-update.log`

## Installer options

Run the installer as root.

| Option | Default | Function |
| --- | --- | --- |
| `--label LABEL` | `com.dns-update` | Set the LaunchDaemon label. |
| `--binary PATH` | `/usr/local/bin/dns-update` | Set the binary path. |
| `--config PATH` | `/usr/local/etc/dns-update/config.json` | Set the config path. |
| `--token PATH` | `/usr/local/etc/dns-update/cloudflare.token` | Set the token path. |
| `--interval SECONDS` | `300` | Set a positive run interval. |
| `--plist PATH` | `/Library/LaunchDaemons/com.dns-update.plist` | Set the plist path. |
| `--log PATH` | `/var/log/dns-update.log` | Set the combined log path. |
| `--timeout DURATION` | `2m` | Set `DNS_UPDATE_TIMEOUT`. |
| `--validate-config` | disabled | Run one config preflight before installation. |

The installer writes the plist with mode `0644`.
The installer escapes each interpolated XML value.
The installer uses `plutil` when that command is available.

Example:

```sh
sudo ./deploy/launchd/install-launchd-job.sh \
  --binary /usr/local/bin/dns-update \
  --config /usr/local/etc/dns-update/config.json \
  --token /usr/local/etc/dns-update/cloudflare.token \
  --interval 300 \
  --log /var/log/dns-update.log
```

The helper writes `/Library/LaunchDaemons/com.dns-update.plist` by default and
bootstraps it into the system launchd domain.

Pass `--validate-config` to run an immediate preflight validation before the
helper installs the recurring job. The installed `LaunchDaemon` still runs
normal reconciliation and does not keep `-validate-config` in
`ProgramArguments`.

For a fresh install:

```sh
sudo install -d -m 0755 /usr/local/etc/dns-update
sudo install -m 0755 ./dns-update /usr/local/bin/dns-update
sudo install -m 0644 ./config.json /usr/local/etc/dns-update/config.json
sudo install -m 0600 ./cloudflare.token /usr/local/etc/dns-update/cloudflare.token
sudo ./deploy/launchd/install-launchd-job.sh \
  --binary /usr/local/bin/dns-update \
  --config /usr/local/etc/dns-update/config.json \
  --token /usr/local/etc/dns-update/cloudflare.token
```

To replace an existing job, run the helper again with the same label.
The helper removes the existing job and loads the updated plist.

To remove the default job:

```sh
sudo launchctl bootout system/com.dns-update || true
sudo rm -f /Library/LaunchDaemons/com.dns-update.plist
```
