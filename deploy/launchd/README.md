# launchd deployment

Use `launchd` for native scheduled execution on macOS.
This document uses ASD-STE100 Simplified Technical English.

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
recurring job is installed. The installed `LaunchDaemon` still runs normal
reconciliation and does not keep `-validate-config` in `ProgramArguments`.

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

To replace an existing job, rerun the helper with the same label. It bootstraps
the updated plist after booting out any existing job with that label.

## Health check

Run these commands after installation or an update:

```sh
sudo launchctl print system/com.dns-update
sudo tail -n 50 /var/log/dns-update.log
```

The first command must show a loaded job and its next scheduled state.
The log must show a completed run and no new configuration, credential, probe, or provider error.

To remove the default job:

```sh
sudo launchctl bootout system/com.dns-update || true
sudo rm -f /Library/LaunchDaemons/com.dns-update.plist
```
