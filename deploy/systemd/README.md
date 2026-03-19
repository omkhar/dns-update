# systemd deployment

These units run `dns-update` as a locked-down `Type=oneshot` service behind a
timer. They are the Linux-specific scheduler integration; macOS and Windows use
the native helpers under `deploy/launchd` and `deploy/windows`.

## Files

- `dns-update.service`: hardened service definition
- `dns-update.timer`: periodic trigger
- `dns-update.env`: optional environment override file

## Installation

1. Install the binary:

   ```sh
   install -m 0755 dns-update /usr/bin/dns-update
   ```

2. Install the configuration and token:

   ```sh
   install -d -m 0755 /etc/dns-update
   install -m 0644 config.json /etc/dns-update/config.json
   install -m 0600 cloudflare.token /etc/dns-update/cloudflare.token
   ```

   If you start from the packaged samples, keep the same final permissions and
   ownership on the live files:

   ```sh
   install -o root -g root -m 0644 /etc/dns-update/config.example.json /etc/dns-update/config.json
   install -o root -g root -m 0600 /etc/dns-update/cloudflare.token.example /etc/dns-update/cloudflare.token
   ```

   Keep the source token at `0600`. Do not manually create or chmod files under
   `/run/credentials/`; `LoadCredential=` materializes that runtime file for the
   service on each start.

   A config copied from `config.example.json` can keep its sample
   `api_token_file` value for the packaged timer because the unit overrides only
   that field at runtime. If you plan to run `/usr/bin/dns-update` directly
   outside the unit, either change `provider.cloudflare.api_token_file` to
   `/etc/dns-update/cloudflare.token` or export
   `DNS_UPDATE_PROVIDER_CLOUDFLARE_API_TOKEN_FILE=/etc/dns-update/cloudflare.token`
   for that command.

3. Install the unit files:

   ```sh
   install -m 0644 deploy/systemd/dns-update.service /etc/systemd/system/dns-update.service
   install -m 0644 deploy/systemd/dns-update.timer /etc/systemd/system/dns-update.timer
   ```

4. Optionally install `/etc/dns-update/dns-update.env` from
   `deploy/systemd/dns-update.env`. Use it for runtime flags such as
   `DNS_UPDATE_TIMEOUT`, `DNS_UPDATE_VERBOSE`, `DNS_UPDATE_DRY_RUN`, or
   `DNS_UPDATE_CONFIG`; keep record and provider settings in the JSON config.

5. Reload systemd and enable the timer:

   ```sh
   systemctl daemon-reload
   systemctl enable --now dns-update.timer
   ```

   The first timer run happens immediately at boot or immediately after the
   timer is enabled, then repeats on five-minute clock boundaries. The
   `OnCalendar=` schedule keeps future runs queued even if an early service
   start is skipped, and `Persistent=yes` triggers one catch-up run after
   downtime.

6. Run one immediate reconciliation if you want to validate the setup before the
   next scheduled timer event:

   ```sh
   systemctl start dns-update.service
   ```

## Security model

The service uses:

- `DynamicUser=yes` so the process runs without a persistent account
- `LoadCredential=` so the Cloudflare token is exposed through a private
  credential path instead of a world-readable environment variable
- `ProtectSystem=strict`, `ProtectHome=yes`, and related isolation knobs so the
  filesystem stays read-only to the service
- an empty capability set and `NoNewPrivileges=yes`
- a restricted address-family list limited to Unix, IPv4, and IPv6 sockets

The service still needs outbound network access for DNS resolution, the probe
URLs, and the Cloudflare API. It intentionally does not get write access to the
host filesystem.

Published packages also avoid self-unpacking binary compression so the shipped
`/usr/bin/dns-update` remains compatible with `MemoryDenyWriteExecute=yes`.

Depending on the distro/systemd combination, the runtime credential presented in
`$CREDENTIALS_DIRECTORY` may show up as `0400` or `0440`. That is still treated
as a private systemd-managed credential by `dns-update`.
