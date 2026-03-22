#!/bin/sh
set -eu

usage() {
	cat >&2 <<'EOF'
usage:
  verify-release-assets.sh <artifact>...
EOF
	exit 2
}

check_archive_list() {
	root=$1
	shift
	"$@" | grep -Fx "$root/README.md" >/dev/null
	"$@" | grep -Fx "$root/CHANGELOG.md" >/dev/null
	"$@" | grep -Fx "$root/LICENSE" >/dev/null
	"$@" | grep -Fx "$root/config.example.json" >/dev/null
	"$@" | grep -Fx "$root/cloudflare.token.example" >/dev/null
	"$@" | grep -Fx "$root/deploy/systemd/dns-update.service" >/dev/null
	"$@" | grep -Fx "$root/deploy/launchd/install-launchd-job.sh" >/dev/null
	"$@" | grep -Fx "$root/deploy/windows/register-scheduled-task.ps1" >/dev/null
}

verify_tar_gz() {
	artifact=$1
	root=${artifact##*/}
	root=${root%.tar.gz}
	check_archive_list "$root" tar -tzf "$artifact"
}

verify_zip() {
	artifact=$1
	root=${artifact##*/}
	root=${root%.zip}
	check_archive_list "$root" unzip -Z1 "$artifact"
}

verify_deb() {
	artifact=$1
	command -v dpkg-deb >/dev/null 2>&1 || return 0
	dpkg-deb -c "$artifact" | grep ' ./usr/bin/dns-update$' >/dev/null
	dpkg-deb -c "$artifact" | grep ' ./etc/dns-update/dns-update.env$' >/dev/null
	dpkg-deb -c "$artifact" | grep ' ./usr/lib/systemd/system/dns-update.service$' >/dev/null
}

verify_rpm() {
	artifact=$1
	command -v rpm >/dev/null 2>&1 || return 0
	rpm -qpl --nosignature "$artifact" | grep '^/usr/bin/dns-update$' >/dev/null
	rpm -qpl --nosignature "$artifact" | grep '^/etc/dns-update/dns-update.env$' >/dev/null
	rpm -qpl --nosignature "$artifact" | grep '^/usr/lib/systemd/system/dns-update.service$' >/dev/null
}

if [ "$#" -eq 0 ]; then
	usage
fi

for artifact in "$@"; do
	case "$artifact" in
	*.tar.gz)
		verify_tar_gz "$artifact"
		;;
	*.zip)
		verify_zip "$artifact"
		;;
	*.deb)
		verify_deb "$artifact"
		;;
	*.rpm)
		verify_rpm "$artifact"
		;;
	*.sigstore.json | *.spdx.json | *checksums.txt)
		;;
	*)
		echo "unsupported release artifact: $artifact" >&2
		exit 1
		;;
	esac
done
