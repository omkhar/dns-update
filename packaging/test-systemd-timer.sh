#!/bin/sh
set -eu

script_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
repo_root=$(CDPATH='' cd -- "$script_dir/.." && pwd)
. "$script_dir/lib.sh"

usage() {
	cat <<'EOF' >&2
usage: ./packaging/test-systemd-timer.sh --label LABEL --image IMAGE --family debian|ubuntu|fedora
EOF
	exit 2
}

label=
image=
family=

while [ "$#" -gt 0 ]; do
	case "$1" in
	--label)
		shift
		[ "$#" -gt 0 ] || usage
		label=$1
		;;
	--image)
		shift
		[ "$#" -gt 0 ] || usage
		image=$1
		;;
	--family)
		shift
		[ "$#" -gt 0 ] || usage
		family=$1
		;;
	*)
		usage
		;;
	esac
	shift
done

[ -n "$label" ] || usage
[ -n "$image" ] || usage
[ -n "$family" ] || usage

case "$family" in
debian | ubuntu)
	install_packages='apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends systemd dbus && apt-get clean && rm -rf /var/lib/apt/lists/*'
	;;
fedora)
	install_packages='dnf install -y systemd dbus-daemon procps-ng && dnf clean all'
	;;
*)
	echo "unsupported family: $family" >&2
	exit 2
	;;
esac

case "$(uname -m)" in
x86_64 | amd64)
	target=amd64
	goarch=amd64
	;;
aarch64 | arm64)
	target=rpi64
	goarch=arm64
	;;
*)
	echo "unsupported host architecture: $(uname -m)" >&2
	exit 1
	;;
esac

command -v docker >/dev/null 2>&1 || {
	echo "missing required command: docker" >&2
	exit 1
}

mode=${PACKAGING_SYSTEMD_TEST_MODE:-auto}
use_package=0
case "$mode" in
package)
	use_package=1
	;;
raw)
	use_package=0
	;;
auto)
	case "$family" in
	debian | ubuntu)
		if command -v dpkg-deb >/dev/null 2>&1; then
			use_package=1
		fi
		;;
	fedora)
		if command -v rpmbuild >/dev/null 2>&1; then
			use_package=1
		fi
		;;
	esac
	;;
*)
	echo "unsupported PACKAGING_SYSTEMD_TEST_MODE: $mode" >&2
	exit 2
	;;
esac

mkdir -p "$repo_root/out"
tmpdir=$(mktemp -d "$repo_root/out/systemd-test.XXXXXX")
image_tag="dns-update-systemd-$label-$$"
container_name="dns-update-systemd-$label-$$"

cleanup() {
	docker rm -f "$container_name" >/dev/null 2>&1 || true
	docker rmi -f "$image_tag" >/dev/null 2>&1 || true
	rm -rf "$tmpdir"
}
trap cleanup EXIT INT TERM

case "$family" in
debian | ubuntu)
	package_ext=deb
	package_install_cmd='dpkg -i /fixtures/dns-update-package.deb'
	;;
fedora)
	package_ext=rpm
	package_install_cmd='rpm -Uvh --replacepkgs --nosignature /fixtures/dns-update-package.rpm'
	;;
esac

if [ "$use_package" -eq 1 ]; then
	case "$family" in
	debian | ubuntu)
		PACKAGING_FORCE_DIRECT_DEB=1 \
		PACKAGING_SKIP_NATIVE_TESTS=1 \
		PACKAGING_SKIP_SIGN=1 \
			"$repo_root/packaging/build-deb.sh" "$target"
		set -- "$repo_root"/out/packages/deb/"$target"/*.deb
		;;
	fedora)
		PACKAGING_LINUX_MACROS=1 \
		PACKAGING_SKIP_BUILDDEPS=1 \
		PACKAGING_SKIP_NATIVE_TESTS=1 \
		PACKAGING_SKIP_SIGN=1 \
			"$repo_root/packaging/build-rpm.sh" "$target"
		set -- "$repo_root"/out/packages/rpm/"$target"/*.rpm
		;;
	esac

	[ -e "$1" ] || {
		echo "expected package artifact for $family $target, found none" >&2
		exit 1
	}
	[ "$#" -eq 1 ] || {
		echo "expected one package artifact for $family $target, found $#" >&2
		exit 1
	}
	cp "$1" "$tmpdir/dns-update-package.$package_ext"
else
	GOOS=linux GOARCH="$goarch" CGO_ENABLED=0 go build -o "$tmpdir/dns-update" "$repo_root/cmd/dns-update"
	cp "$repo_root/deploy/systemd/dns-update.service" "$tmpdir/dns-update.service"
	cp "$repo_root/deploy/systemd/dns-update.timer" "$tmpdir/dns-update.timer"
	cp "$repo_root/deploy/systemd/dns-update.env" "$tmpdir/dns-update.env"
	# Shorten the calendar cadence so the integration test can observe a real
	# timer-fired recovery quickly while still exercising OnCalendar semantics.
	sed 's/^OnCalendar=.*/OnCalendar=*-*-* *:*:0\/15/' \
		"$tmpdir/dns-update.timer" > "$tmpdir/dns-update.timer.tmp"
	mv "$tmpdir/dns-update.timer.tmp" "$tmpdir/dns-update.timer"
fi

cat >"$tmpdir/config.json" <<'EOF'
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
      "api_token_file": "/etc/dns-update/cloudflare.token"
    }
  }
}
EOF

printf '%s\n' dummy-token >"$tmpdir/cloudflare.token"

cat >"$tmpdir/integration.conf" <<'EOF'
[Service]
PermissionsStartOnly=true
DynamicUser=no
User=root
Group=root
RuntimeDirectory=dns-update-integration
RuntimeDirectoryMode=0755
Environment=CREDENTIALS_DIRECTORY=%t/dns-update-integration/credentials/dns-update.service
Environment=DNS_UPDATE_PROVIDER_CLOUDFLARE_API_TOKEN_FILE=%t/dns-update-integration/credentials/dns-update.service/cloudflare.token
ExecStartPre=/bin/sh -c 'mkdir -p "$CREDENTIALS_DIRECTORY" && chmod 0755 "$(dirname "$CREDENTIALS_DIRECTORY")" "$CREDENTIALS_DIRECTORY" && install -m 0440 /etc/dns-update/cloudflare.token "$CREDENTIALS_DIRECTORY/cloudflare.token"'
ExecStart=
ExecStart=/usr/bin/dns-update -validate-config
EOF

cat >"$tmpdir/container-check.sh" <<'EOF'
#!/bin/sh
set -eu

fixtures_dir=/fixtures

if [ "__USE_PACKAGE__" -eq 1 ]; then
__PACKAGE_INSTALL_CMD__
else
install -D -m 0644 "$fixtures_dir/dns-update.env" /etc/dns-update/dns-update.env
install -D -m 0644 "$fixtures_dir/dns-update.service" /etc/systemd/system/dns-update.service
install -D -m 0644 "$fixtures_dir/dns-update.timer" /etc/systemd/system/dns-update.timer
fi
install -D -m 0644 "$fixtures_dir/integration.conf" /etc/systemd/system/dns-update.service.d/integration.conf

if [ "__USE_PACKAGE__" -eq 1 ]; then
	sed 's/^OnCalendar=.*/OnCalendar=*-*-* *:*:0\/15/' \
		/usr/lib/systemd/system/dns-update.timer > /etc/systemd/system/dns-update.timer
fi

systemctl daemon-reload
systemctl enable dns-update.timer
systemctl start dns-update.timer

i=0
while [ "$i" -lt 30 ]; do
	if journalctl -u dns-update.service --no-pager 2>/dev/null | grep -q 'unmet condition check'; then
		break
	fi
	i=$((i + 1))
	sleep 1
done

if ! journalctl -u dns-update.service --no-pager | grep -q 'unmet condition check'; then
	systemctl status dns-update.service --no-pager || true
	journalctl -u dns-update.service --no-pager || true
	exit 1
fi

if ! systemctl is-active --quiet dns-update.timer; then
	systemctl status dns-update.timer --no-pager || true
	exit 1
fi

next_elapse=$(systemctl show dns-update.timer -p NextElapseUSecRealtime --value)
if [ -z "$next_elapse" ] || [ "$next_elapse" = "n/a" ] || [ "$next_elapse" = "0" ]; then
	systemctl status dns-update.timer --no-pager || true
	systemctl list-timers dns-update.timer --all --no-pager || true
	exit 1
fi

last_trigger_before=$(systemctl show dns-update.timer -p LastTriggerUSecMonotonic --value)
success_since=$(date '+%Y-%m-%d %H:%M:%S')

if [ "__USE_PACKAGE__" -eq 0 ]; then
install -D -m 0755 "$fixtures_dir/dns-update" /usr/bin/dns-update
fi
install -D -m 0644 "$fixtures_dir/config.json" /etc/dns-update/config.json
install -D -m 0600 "$fixtures_dir/cloudflare.token" /etc/dns-update/cloudflare.token

i=0
while [ "$i" -lt 60 ]; do
	if journalctl -u dns-update.service --since "$success_since" --no-pager 2>/dev/null | grep -q 'config is valid'; then
		break
	fi
	i=$((i + 1))
	sleep 1
done

if ! journalctl -u dns-update.service --since "$success_since" --no-pager | grep -q 'config is valid'; then
	systemctl status dns-update.service --no-pager || true
	journalctl -u dns-update.service --no-pager || true
	exit 1
fi

last_trigger_after=$(systemctl show dns-update.timer -p LastTriggerUSecMonotonic --value)
if [ -z "$last_trigger_after" ] || [ "$last_trigger_after" = "$last_trigger_before" ]; then
	systemctl status dns-update.timer --no-pager || true
	systemctl list-timers dns-update.timer --all --no-pager || true
	journalctl -u dns-update.service --no-pager || true
	exit 1
fi

if [ "$(systemctl show dns-update.service -p Result --value)" != "success" ]; then
	systemctl status dns-update.service --no-pager || true
	journalctl -u dns-update.service --no-pager || true
	exit 1
fi
EOF
sed -i.bak \
	-e "s|__USE_PACKAGE__|$use_package|g" \
	-e "s|__PACKAGE_INSTALL_CMD__|$package_install_cmd|" \
	"$tmpdir/container-check.sh"
rm -f "$tmpdir/container-check.sh.bak"
chmod 0755 "$tmpdir/container-check.sh"

cat >"$tmpdir/Dockerfile" <<EOF
FROM $image
ENV container=docker
RUN $install_packages
STOPSIGNAL SIGRTMIN+3
CMD ["/usr/lib/systemd/systemd"]
EOF

docker build -t "$image_tag" "$tmpdir" >/dev/null

docker run -d \
	--name "$container_name" \
	--privileged \
	--cgroupns=host \
	-e container=docker \
	--tmpfs /run \
	--tmpfs /run/lock \
	--tmpfs /tmp \
	-v "$tmpdir:/fixtures:ro" \
	-v /sys/fs/cgroup:/sys/fs/cgroup:rw \
	"$image_tag" >/dev/null

i=0
while [ "$i" -lt 30 ]; do
	if docker exec "$container_name" /bin/sh -lc 'systemctl list-units >/dev/null 2>&1'; then
		break
	fi
	i=$((i + 1))
	sleep 1
done

docker exec "$container_name" /bin/sh /fixtures/container-check.sh
