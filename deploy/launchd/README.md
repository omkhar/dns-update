# launchd deployment

Use `launchd` for native scheduled execution on macOS.

The helper script in this directory writes and loads a `LaunchDaemon` that:

- runs `dns-update` as root
- uses `RunAtLoad` for an immediate startup run
- reruns at a fixed interval with `StartInterval`
- passes the Cloudflare token path and timeout through environment variables
- sends stdout and stderr to a log file

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
