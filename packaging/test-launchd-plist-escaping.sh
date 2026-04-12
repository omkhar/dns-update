#!/bin/sh
set -eu

script_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
repo_root=$(CDPATH='' cd -- "$script_dir/.." && pwd)

mkdir -p "$repo_root/out"
tmpdir=$(mktemp -d "$repo_root/out/launchd-plist-escaping.XXXXXX")
stub_dir="$tmpdir/bin"
plist="$tmpdir/com.dns-update.plist"
launchctl_log="$tmpdir/launchctl.log"

cleanup() {
	rm -rf "$tmpdir"
}
trap cleanup EXIT INT TERM

mkdir -p "$stub_dir"

cat >"$stub_dir/id" <<'EOF'
#!/bin/sh
if [ "$#" -eq 1 ] && [ "$1" = "-u" ]; then
	printf '0\n'
	exit 0
fi

exec /usr/bin/id "$@"
EOF
chmod +x "$stub_dir/id"

cat >"$stub_dir/launchctl" <<'EOF'
#!/bin/sh
printf '%s\n' "$@" >>"${DNS_UPDATE_TEST_LAUNCHCTL_LOG:?}"
exit 0
EOF
chmod +x "$stub_dir/launchctl"

label='com.dns-update</string><key>InjectedLabel</key><true/><string>'
binary="$tmpdir/bin</string><string>-delete</string><string>"
config="$tmpdir/config</string><string>-dry-run</string><string>"
token="$tmpdir/token&danger"
timeout='30s</string><key>InjectedTimeout</key><string>'
log="$tmpdir/log</string><key>InjectedLog</key><string>"

PATH="$stub_dir:$PATH" \
DNS_UPDATE_TEST_LAUNCHCTL_LOG="$launchctl_log" \
	"$repo_root/deploy/launchd/install-launchd-job.sh" \
		--label "$label" \
		--binary "$binary" \
		--config "$config" \
		--token "$token" \
		--interval 5 \
		--plist "$plist" \
		--log "$log" \
		--timeout "$timeout"

if command -v plutil >/dev/null 2>&1; then
	plutil -lint "$plist" >/dev/null
fi

program_arguments=$(
	awk '
		/<key>ProgramArguments<\/key>/ { in_array = 1; next }
		in_array && /<\/array>/ { exit }
		in_array { print }
	' "$plist"
)

environment_values=$(
	awk '
		/<key>EnvironmentVariables<\/key>/ { in_dict = 1; next }
		in_dict && /<\/dict>/ { exit }
		in_dict { print }
	' "$plist"
)

program_argument_count=$(printf '%s\n' "$program_arguments" | grep -c '<string>')
environment_value_count=$(printf '%s\n' "$environment_values" | grep -c '<string>')

[ "$program_argument_count" -eq 3 ] || {
	printf 'unexpected ProgramArguments entry count: %s\n' "$program_argument_count" >&2
	cat "$plist" >&2
	exit 1
}

[ "$environment_value_count" -eq 2 ] || {
	printf 'unexpected EnvironmentVariables value count: %s\n' "$environment_value_count" >&2
	cat "$plist" >&2
	exit 1
}

for injected in \
	'<key>InjectedLabel</key>' \
	'<key>InjectedTimeout</key>' \
	'<key>InjectedLog</key>' \
	'<string>-delete</string>' \
	'<string>-dry-run</string>'
do
	if grep -F -- "$injected" "$plist" >/dev/null; then
		printf 'unexpected injected plist fragment: %s\n' "$injected" >&2
		cat "$plist" >&2
		exit 1
	fi
done

for escaped in \
	'&lt;/string&gt;&lt;key&gt;InjectedLabel&lt;/key&gt;&lt;true/&gt;&lt;string&gt;' \
	'&lt;/string&gt;&lt;string&gt;-delete&lt;/string&gt;&lt;string&gt;' \
	'&lt;/string&gt;&lt;string&gt;-dry-run&lt;/string&gt;&lt;string&gt;' \
	'token&amp;danger' \
	'&lt;/string&gt;&lt;key&gt;InjectedTimeout&lt;/key&gt;&lt;string&gt;' \
	'&lt;/string&gt;&lt;key&gt;InjectedLog&lt;/key&gt;&lt;string&gt;'
do
	if ! grep -F -- "$escaped" "$plist" >/dev/null; then
		printf 'missing escaped plist fragment: %s\n' "$escaped" >&2
		cat "$plist" >&2
		exit 1
	fi
done
