# systemd deployment

These units run `dns-update` as a locked-down `Type=oneshot` service behind a
timer.

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

3. Install the unit files:

   ```sh
   install -m 0644 deploy/systemd/dns-update.service /etc/systemd/system/dns-update.service
   install -m 0644 deploy/systemd/dns-update.timer /etc/systemd/system/dns-update.timer
   ```

4. Optionally install `/etc/dns-update/dns-update.env` from
   `deploy/systemd/dns-update.env`.

5. Reload systemd and enable the timer:

   ```sh
   systemctl daemon-reload
   systemctl enable --now dns-update.timer
   ```

   The first timer run happens immediately at boot or immediately after the
   timer is enabled, then repeats every five minutes.

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

Depending on the distro/systemd combination, the runtime credential presented in
`$CREDENTIALS_DIRECTORY` may show up as `0400` or `0440`. That is still treated
as a private systemd-managed credential by `dns-update`.
