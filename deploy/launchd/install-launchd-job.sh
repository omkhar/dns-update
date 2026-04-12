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

xml_escape() {
	printf '%s' "$1" | sed \
		-e 's/&/\&amp;/g' \
		-e 's/</\&lt;/g' \
		-e 's/>/\&gt;/g'
}

write_plist() {
	{
		printf '%s\n' '<?xml version="1.0" encoding="UTF-8"?>'
		printf '%s\n' '<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "https://www.apple.com/DTDs/PropertyList-1.0.dtd">'
		printf '%s\n' '<plist version="1.0">'
		printf '%s\n' '<dict>'
		printf '%s\n' '  <key>Label</key>'
		printf '  <string>%s</string>\n' "$(xml_escape "$label")"
		printf '%s\n' '  <key>ProgramArguments</key>'
		printf '%s\n' '  <array>'
		printf '    <string>%s</string>\n' "$(xml_escape "$binary")"
		printf '%s\n' '    <string>-config</string>'
		printf '    <string>%s</string>\n' "$(xml_escape "$config")"
		printf '%s\n' '  </array>'
		printf '%s\n' '  <key>EnvironmentVariables</key>'
		printf '%s\n' '  <dict>'
		printf '%s\n' '    <key>DNS_UPDATE_PROVIDER_CLOUDFLARE_API_TOKEN_FILE</key>'
		printf '    <string>%s</string>\n' "$(xml_escape "$token")"
		printf '%s\n' '    <key>DNS_UPDATE_TIMEOUT</key>'
		printf '    <string>%s</string>\n' "$(xml_escape "$timeout")"
		printf '%s\n' '  </dict>'
		printf '%s\n' '  <key>ProcessType</key>'
		printf '%s\n' '  <string>Background</string>'
		printf '%s\n' '  <key>RunAtLoad</key>'
		printf '%s\n' '  <true/>'
		printf '%s\n' '  <key>StartInterval</key>'
		printf '  <integer>%s</integer>\n' "$interval"
		printf '%s\n' '  <key>StandardOutPath</key>'
		printf '  <string>%s</string>\n' "$(xml_escape "$log")"
		printf '%s\n' '  <key>StandardErrorPath</key>'
		printf '  <string>%s</string>\n' "$(xml_escape "$log")"
		printf '%s\n' '</dict>'
		printf '%s\n' '</plist>'
	} >"$plist"
}

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

if [ "$validate_config" = 1 ]; then
	{
		printf '[%s] validating config before installing launchd job\n' "$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
		DNS_UPDATE_PROVIDER_CLOUDFLARE_API_TOKEN_FILE="$token" \
		DNS_UPDATE_TIMEOUT="$timeout" \
			"$binary" -config "$config" -validate-config
	} >>"$log" 2>&1
	rm -f "$log"
fi

write_plist

chmod 0644 "$plist"

if command -v plutil >/dev/null 2>&1; then
	plutil -lint "$plist" >/dev/null
fi

launchctl bootout "system/$label" >/dev/null 2>&1 || true
launchctl bootstrap system "$plist"
