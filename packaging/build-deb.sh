#!/bin/sh
set -eu

# shellcheck source=packaging/lib.sh
. "$(dirname "$0")/lib.sh"

repo_root=$(repo_root "$0")
version=${DEB_VERSION:-$(package_version "$repo_root")}
release=${DEB_RELEASE:-$(package_release "$repo_root")}
targets=$(resolve_targets "$@")
output_root=${DEB_OUTPUT_DIR:-"$repo_root/out/packages/deb"}
release_goflags=${DNS_UPDATE_RELEASE_GOFLAGS:-$(release_goflags)}
release_ldflags=${DNS_UPDATE_RELEASE_LDFLAGS:-$(release_ldflags)}
source_epoch=$(source_date_epoch "$repo_root")

build_with_debhelper() {
	target=$1
	deb_arch=$2
	target_dir=$3
	parent_dir=$4

	find "$parent_dir" -maxdepth 1 -type f \
		\( -name "dns-update_${version}-${release}_${deb_arch}.deb" \
		-o -name "dns-update_${version}-${release}_${deb_arch}.buildinfo" \
		-o -name "dns-update_${version}-${release}_${deb_arch}.changes" \) \
		-exec rm -f {} +

	(
		cd "$repo_root"
		env DNS_UPDATE_RELEASE_GOFLAGS="$release_goflags" \
			DNS_UPDATE_RELEASE_LDFLAGS="$release_ldflags" \
			SOURCE_DATE_EPOCH="$source_epoch" \
			DEB_BUILD_OPTIONS="nocheck ${DEB_BUILD_OPTIONS:-}" \
			dpkg-buildpackage -us -uc -b -a"$deb_arch" \
			${PACKAGING_SKIP_BUILDDEPS:+-d}
	)

	deb_file=$parent_dir/dns-update_${version}-${release}_${deb_arch}.deb
	buildinfo_file=$parent_dir/dns-update_${version}-${release}_${deb_arch}.buildinfo
	changes_file=$parent_dir/dns-update_${version}-${release}_${deb_arch}.changes

	if [ ! -f "$deb_file" ]; then
		echo "expected Debian artifact missing: $deb_file" >&2
		exit 1
	fi

	mv "$deb_file" "$target_dir/"
	if [ -f "$buildinfo_file" ]; then
		mv "$buildinfo_file" "$target_dir/"
	fi
	if [ -f "$changes_file" ]; then
		mv "$changes_file" "$target_dir/"
	fi

	sign_artifacts "$target_dir/$(basename "$deb_file")"
}

build_without_debhelper() {
	target=$1
	deb_arch=$2
	target_dir=$3

	require_cmd dpkg-deb
	require_cmd gzip
	require_cmd install

	tmpdir=$(mktemp -d)
	cleanup() {
		rm -rf "$tmpdir"
	}
	trap cleanup EXIT INT TERM

	build_dir=$tmpdir/build
	package_dir=$tmpdir/package
	doc_dir=$package_dir/usr/share/doc/dns-update
	man_dir=$package_dir/usr/share/man/man1

	mkdir -p "$build_dir" "$package_dir/DEBIAN" "$doc_dir" "$man_dir"

	install_file() {
		mode=$1
		source=$2
		dest=$3
		mkdir -p "$(dirname "$dest")"
		install -m "$mode" "$source" "$dest"
	}

	case "$target" in
	amd64)
		goos=linux
		goarch=amd64
		goarm=
		;;
	rpi32)
		goos=linux
		goarch=arm
		goarm=7
		;;
	rpi64)
		goos=linux
		goarch=arm64
		goarm=
		;;
	*)
		echo "unsupported Debian target: $target" >&2
		exit 1
		;;
	esac

	(
		cd "$repo_root"
		env CGO_ENABLED=0 \
			GOFLAGS="$release_goflags" \
			GOOS="$goos" \
			GOARCH="$goarch" \
			${goarm:+GOARM="$goarm"} \
			go build -ldflags "$release_ldflags" \
				-o "$build_dir/dns-update" \
				./cmd/dns-update
	)

	install_file 0755 "$build_dir/dns-update" \
		"$package_dir/usr/bin/dns-update"
	install_file 0644 "$repo_root/deploy/systemd/dns-update.service" \
		"$package_dir/usr/lib/systemd/system/dns-update.service"
	install_file 0644 "$repo_root/deploy/systemd/dns-update.timer" \
		"$package_dir/usr/lib/systemd/system/dns-update.timer"
	install_file 0644 "$repo_root/deploy/systemd/dns-update.env" \
		"$package_dir/etc/dns-update/dns-update.env"
	install_file 0644 "$repo_root/config.example.json" \
		"$package_dir/etc/dns-update/config.example.json"
	install_file 0600 "$repo_root/cloudflare.token.example" \
		"$package_dir/etc/dns-update/cloudflare.token.example"
	install_file 0644 "$repo_root/README.md" \
		"$doc_dir/README.md"
	install_file 0644 "$repo_root/SECURITY.md" \
		"$doc_dir/SECURITY.md"
	install_file 0644 "$repo_root/CONTRIBUTING.md" \
		"$doc_dir/CONTRIBUTING.md"
	install_file 0644 "$repo_root/packaging/README.md" \
		"$doc_dir/packaging-README.md"
	install_file 0644 "$repo_root/docs/dns-update.1" \
		"$man_dir/dns-update.1"
	install_file 0644 "$repo_root/debian/copyright" \
		"$doc_dir/copyright"
	install_file 0644 "$repo_root/debian/dns-update.lintian-overrides" \
		"$package_dir/usr/share/lintian/overrides/dns-update"
	gzip -n "$man_dir/dns-update.1"
	gzip -n -c "$repo_root/debian/changelog" > "$doc_dir/changelog.Debian.gz"

	install_file 0644 "$repo_root/debian/dns-update.conffiles" \
		"$package_dir/DEBIAN/conffiles"

	if command -v md5sum >/dev/null 2>&1; then
		(
			cd "$package_dir"
			find etc usr -type f | LC_ALL=C sort | sed 's#^./##' | xargs md5sum
		) > "$package_dir/DEBIAN/md5sums"
	fi

	installed_size=$(du -sk "$package_dir" | awk '{print $1}')
	cat > "$package_dir/DEBIAN/control" <<EOF
Package: dns-update
Version: $version-$release
Section: net
Priority: optional
Architecture: $deb_arch
Maintainer: dns-update Maintainers <opensource@dns-update.invalid>
Depends: systemd
Installed-Size: $installed_size
Description: Keep DNS A and AAAA records aligned with egress IP addresses
 dns-update probes the host's current egress IPv4 and IPv6 addresses and keeps
 one DNS hostname's A and AAAA records aligned with that state.
 .
 The package installs a hardened systemd service, a timer, a commented
 environment override file plus sample config and token placeholder files under
 /etc/dns-update.
EOF

	normalize_tree_timestamps "$package_dir" "$source_epoch"

	deb_file=$target_dir/dns-update_${version}-${release}_${deb_arch}.deb
	SOURCE_DATE_EPOCH="$source_epoch" \
		dpkg-deb --root-owner-group --build "$package_dir" "$deb_file"
	sign_artifacts "$deb_file"

	trap - EXIT INT TERM
	cleanup
}

run_native_tests "$repo_root"

parent_dir=$(CDPATH='' cd -- "$repo_root/.." && pwd)

	for target in $targets; do
		deb_arch=$(deb_arch_for_target "$target")
		target_dir=$output_root/$target
		prepare_clean_dir "$target_dir"
		if [ "${PACKAGING_FORCE_DIRECT_DEB:-}" != 1 ] && \
			command -v dpkg-buildpackage >/dev/null 2>&1 && \
			command -v dh >/dev/null 2>&1; then
			build_with_debhelper "$target" "$deb_arch" "$target_dir" "$parent_dir"
			continue
		fi

	build_without_debhelper "$target" "$deb_arch" "$target_dir"
done
