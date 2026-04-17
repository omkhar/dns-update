#!/bin/sh
set -eu

repo_root=$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)
script_path=$repo_root/packaging/build-remote-container.sh

temp_dir=$(mktemp -d)
cleanup() {
	rm -rf "$temp_dir"
}
trap cleanup EXIT INT TERM HUP

fake_bin=$temp_dir/bin
mkdir -p "$fake_bin"

cat >"$fake_bin/ssh" <<'EOF'
#!/bin/sh
echo "fake ssh invoked: $*" >&2
exit 1
EOF
chmod +x "$fake_bin/ssh"

marker_path=$temp_dir/should-not-exist
config_path=$temp_dir/build-remote-container.local.env
output_log=$temp_dir/output.log
cat >"$config_path" <<'EOF'
REMOTE_BUILD_HOST='builder@example-build-host'
REMOTE_BUILD_IMAGE='golang:$(touch __MARKER_PATH__)'
EOF
sed "s#__MARKER_PATH__#$marker_path#g" "$config_path" >"$config_path.tmp"
mv "$config_path.tmp" "$config_path"

if PATH="$fake_bin:$PATH" REMOTE_BUILD_LOCAL_CONFIG="$config_path" sh "$script_path" >"$output_log" 2>&1; then
	echo "build-remote-container.sh unexpectedly succeeded with fake ssh" >&2
	exit 1
fi
grep -F "creating remote workspace on builder@example-build-host" "$output_log" >/dev/null
[ ! -e "$marker_path" ] || {
	echo "local config executed shell content while loading" >&2
	exit 1
}

bad_config_path=$temp_dir/invalid.local.env
bad_output_log=$temp_dir/bad-output.log
cat >"$bad_config_path" <<'EOF'
REMOTE_BUILD_HOST=builder@example-build-host
PATH=/tmp/not-allowed
EOF

if PATH="$fake_bin:$PATH" REMOTE_BUILD_LOCAL_CONFIG="$bad_config_path" sh "$script_path" >"$bad_output_log" 2>&1; then
	echo "build-remote-container.sh unexpectedly accepted an unsupported local config key" >&2
	exit 1
fi
grep -F "unsupported local config key" "$bad_output_log" >/dev/null
