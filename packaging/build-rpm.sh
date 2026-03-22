#!/bin/sh
set -eu

# shellcheck source=packaging/lib.sh
. "$(dirname "$0")/lib.sh"

repo_root=$(repo_root "$0")
version=${RPM_VERSION:-$(package_version "$repo_root")}
release=${RPM_RELEASE:-$(package_release "$repo_root")}
targets=$(resolve_targets "$@")
topdir_root=${RPM_TOPDIR_ROOT:-"$repo_root/out/rpm"}
output_root=${RPM_OUTPUT_DIR:-"$repo_root/out/packages/rpm"}
release_goflags=${DNS_UPDATE_RELEASE_GOFLAGS:-$(release_goflags)}
release_ldflags=${DNS_UPDATE_RELEASE_LDFLAGS:-$(release_ldflags)}

if [ "${PACKAGING_SKIP_SIGN:-}" != 1 ]; then
	require_cmd cosign
fi
require_cmd rpmbuild
# Prefer GNU tar (gtar on macOS) to ensure --exclude patterns anchor correctly.
TAR=$(command -v gtar 2>/dev/null || command -v tar)
require_cmd "$TAR"

run_native_tests "$repo_root"

tmpdir=$(mktemp -d)
# Go module cache files are read-only; chmod before removal to avoid errors.
cleanup() { chmod -R u+w "$tmpdir" 2>/dev/null || true; rm -rf "$tmpdir"; }
trap cleanup EXIT INT TERM

stage_root="$tmpdir/dns-update-$version"
mkdir -p "$stage_root"

$TAR -C "$repo_root" \
	--exclude='./.git' \
	--exclude='./dist' \
	--exclude='./out' \
	--exclude='./debian/.build' \
	--exclude='./debian/.debhelper' \
	--exclude='./debian/debhelper-build-stamp' \
	--exclude='./debian/dns-update' \
	--exclude='./debian/files' \
	--exclude='./debian/substvars' \
	--exclude='./debian/*.debhelper.log' \
	--exclude='./debian/*.substvars' \
	--exclude='./dns-update' \
	-cf - . | $TAR -C "$stage_root" -xf -

for target in $targets; do
	rpm_target=$(rpm_target_for_target "$target")
	rpm_build_target=$rpm_target
	topdir=$topdir_root/$target
	target_dir=$output_root/$target

	prepare_clean_dir "$topdir"
	mkdir -p \
		"$topdir/BUILD" \
		"$topdir/BUILDROOT" \
		"$topdir/RPMS" \
		"$topdir/SOURCES" \
		"$topdir/SPECS" \
		"$topdir/SRPMS"
	prepare_clean_dir "$target_dir"

	$TAR -C "$tmpdir" -czf "$topdir/SOURCES/dns-update-$version.tar.gz" "dns-update-$version"
	cp "$repo_root/packaging/rpm/dns-update.spec" "$topdir/SPECS/dns-update.spec"

	# When PACKAGING_LINUX_MACROS=1, force Linux filesystem macros and prepend
	# GNU coreutils so 'install -D' works outside of Linux.
	rpm_env_home=$HOME
	if [ "${PACKAGING_LINUX_MACROS:-}" = 1 ]; then
		# Prefer GNU coreutils gnubin when available, but keep the normal PATH on
		# Linux builders where install(1) already supports -D.
		gnu_coreutils_bin=/opt/homebrew/opt/coreutils/libexec/gnubin
		rpm_env_path=$PATH
		rpm_build_target=$rpm_target-linux
		if [ -d "$gnu_coreutils_bin" ]; then
			rpm_env_path="$gnu_coreutils_bin:$PATH"
		fi
	fi

	set -- rpmbuild -ba "$topdir/SPECS/dns-update.spec" \
		--without check \
		${PACKAGING_SKIP_BUILDDEPS:+--nodeps} \
		--target "$rpm_build_target" \
		--define "_topdir $topdir" \
		--define "pkg_version $version" \
		--define "pkg_release $release" \
		--define "release_goflags $release_goflags" \
		--define "release_ldflags $release_ldflags"
	if [ "${PACKAGING_LINUX_MACROS:-}" = 1 ]; then
		set -- "$@" \
			--define "_prefix /usr" \
			--define "_exec_prefix /usr" \
			--define "_bindir /usr/bin" \
			--define "_sysconfdir /etc" \
			--define "_unitdir /usr/lib/systemd/system" \
			--define "_datarootdir /usr/share" \
			--define "_datadir /usr/share" \
			--define "_docdir /usr/share/doc" \
			--define "_licensedir /usr/share/licenses" \
			--define "_mandir /usr/share/man"
	else
		rpm_env_path=$PATH
	fi

	HOME="$rpm_env_home" GOPATH="$(go env GOPATH)" PATH="$rpm_env_path" "$@"

	rpm_files=$(find "$topdir/RPMS" -type f -name '*.rpm' | sort)
	if [ -z "$rpm_files" ]; then
		echo "no RPM artifacts produced for target $target" >&2
		exit 1
	fi

	for artifact in $rpm_files; do
		cp "$artifact" "$target_dir/"
	done

	set -- "$target_dir"/*.rpm
	sign_artifacts "$@"
done
