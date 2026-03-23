#!/bin/sh
set -eu

script_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
repo_root=$(CDPATH='' cd -- "$script_dir/.." && pwd)

mkdir -p "$repo_root/out"
tmpdir=$(mktemp -d "$repo_root/out/launchd-test.XXXXXX")
label=com.dns-update.ci.$$
installed_plist=/Library/LaunchDaemons/$label.plist
binary=$tmpdir/dns-update
config=$tmpdir/config.json
token=$tmpdir/cloudflare.token
log=$tmpdir/dns-update.log

cleanup() {
	sudo launchctl bootout "system/$label" >/dev/null 2>&1 || true
	sudo rm -f "$installed_plist"
	rm -rf "$tmpdir"
}
trap cleanup EXIT INT TERM

go build -o "$binary" "$repo_root/cmd/dns-update"

cat >"$config" <<EOF
{
  "record": {
    "name": "host.example.com.",
    "zone": "example.com.",
    "ttl_seconds": 300
  },
  "provider": {
    "type": "cloudflare",
    "cloudflare": {
      "zone_id": "CLOUDFLARE_ZONE_ID",
      "api_token_file": "$token"
    }
  }
}
EOF

printf '%s\n' dummy-token >"$token"
chmod 0600 "$token"

sudo "$repo_root/deploy/launchd/install-launchd-job.sh" \
	--label "$label" \
	--binary "$binary" \
	--config "$config" \
	--token "$token" \
	--interval 5 \
	--plist "$installed_plist" \
	--log "$log" \
	--timeout 30s \
	--validate-config

if grep -q -- '-validate-config' "$installed_plist"; then
	cat "$installed_plist"
	exit 1
fi

cat >"$config" <<EOF
{
  "record": {
    "name": "host.example.com.",
EOF

i=0
while [ "$i" -lt 90 ]; do
	if [ -f "$log" ] && grep -q 'failed to load config: decode config: unexpected EOF' "$log"; then
		break
	fi
	i=$((i + 1))
	sleep 1
done

if [ ! -f "$log" ] || ! grep -q 'failed to load config: decode config: unexpected EOF' "$log"; then
	sudo launchctl print "system/$label" || true
	if [ -f "$log" ]; then
		cat "$log" || true
	fi
	exit 1
fi
