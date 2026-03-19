#!/bin/sh
set -eu

usage() {
	cat <<'EOF' >&2
usage: ./deploy/launchd/install-launchd-job.sh \
  [--label LABEL] \
  [--binary PATH] \
  [--config PATH] \
  [--token PATH] \
  [--interval SECONDS] \
  [--plist PATH] \
  [--log PATH] \
  [--timeout DURATION] \
  [--validate-config]
EOF
	exit 2
}

label=com.dns-update
binary=/usr/local/bin/dns-update
config=/usr/local/etc/dns-update/config.json
token=/usr/local/etc/dns-update/cloudflare.token
interval=300
plist=/Library/LaunchDaemons/com.dns-update.plist
log=/var/log/dns-update.log
timeout=2m
validate_config=0

while [ "$#" -gt 0 ]; do
	case "$1" in
	--label)
		shift
		[ "$#" -gt 0 ] || usage
		label=$1
		;;
	--binary)
		shift
		[ "$#" -gt 0 ] || usage
		binary=$1
		;;
	--config)
		shift
		[ "$#" -gt 0 ] || usage
		config=$1
		;;
	--token)
		shift
		[ "$#" -gt 0 ] || usage
		token=$1
		;;
	--interval)
		shift
		[ "$#" -gt 0 ] || usage
		interval=$1
		;;
	--plist)
		shift
		[ "$#" -gt 0 ] || usage
		plist=$1
		;;
	--log)
		shift
		[ "$#" -gt 0 ] || usage
		log=$1
		;;
	--timeout)
		shift
		[ "$#" -gt 0 ] || usage
		timeout=$1
		;;
	--validate-config)
		validate_config=1
		;;
	*)
		usage
		;;
	esac
	shift
done

case "$interval" in
'' | *[!0-9]*)
	echo "interval must be a positive integer number of seconds" >&2
	exit 2
	;;
0)
	echo "interval must be greater than zero" >&2
	exit 2
	;;
esac

if [ "$(id -u)" -ne 0 ]; then
	echo "install-launchd-job.sh must run as root" >&2
	exit 1
fi

mkdir -p "$(dirname "$plist")" "$(dirname "$log")"

cat >"$plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "https://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>$label</string>
  <key>ProgramArguments</key>
  <array>
    <string>$binary</string>
EOF

if [ "$validate_config" = 1 ]; then
	cat >>"$plist" <<'EOF'
    <string>-validate-config</string>
EOF
fi

cat >>"$plist" <<EOF
    <string>-config</string>
    <string>$config</string>
  </array>
  <key>EnvironmentVariables</key>
  <dict>
    <key>DNS_UPDATE_PROVIDER_CLOUDFLARE_API_TOKEN_FILE</key>
    <string>$token</string>
    <key>DNS_UPDATE_TIMEOUT</key>
    <string>$timeout</string>
  </dict>
  <key>ProcessType</key>
  <string>Background</string>
  <key>RunAtLoad</key>
  <true/>
  <key>StartInterval</key>
  <integer>$interval</integer>
  <key>StandardOutPath</key>
  <string>$log</string>
  <key>StandardErrorPath</key>
  <string>$log</string>
</dict>
</plist>
EOF

chmod 0644 "$plist"

if command -v plutil >/dev/null 2>&1; then
	plutil -lint "$plist" >/dev/null
fi

launchctl bootout "system/$label" >/dev/null 2>&1 || true
launchctl bootstrap system "$plist"
