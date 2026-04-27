#!/bin/sh
set -eu

# shellcheck source=packaging/lib.sh
. "$(dirname "$0")/lib.sh"

repo_root=$(repo_root "$0")
version=${RELEASE_VERSION:-$(package_version "$repo_root")}
out_dir=${RELEASE_OUTPUT_DIR:-"$repo_root/out/release"}
tmp_dir=${RELEASE_TMP_DIR:-"$repo_root/out/release-tmp"}
skip_archives=${RELEASE_SKIP_ARCHIVES:-0}
skip_packages=${RELEASE_SKIP_PACKAGES:-0}
write_checksums=${RELEASE_WRITE_CHECKSUMS:-1}
release_goflags=${DNS_UPDATE_RELEASE_GOFLAGS:-$(release_goflags)}
release_ldflags=${DNS_UPDATE_RELEASE_LDFLAGS:-$(release_ldflags "$repo_root" "$version")}
source_epoch=$(source_date_epoch "$repo_root")
release_tar_cmd=$(release_tar)
require_cmd "$release_tar_cmd"

prepare_clean_dir "$out_dir"
prepare_clean_dir "$tmp_dir"

cleanup() {
	rm -rf "$tmp_dir"
}
trap cleanup EXIT INT TERM

build_archive() {
	goos=$1
	goarch=$2
	goarm=$3
	suffix=$4
	archive_format=$5
	stage_name=dns-update_${version}_${suffix}
	stage_dir=$tmp_dir/$stage_name
	binary_name=dns-update

	if [ "$goos" = windows ]; then
		binary_name=$binary_name.exe
	fi

	rm -rf "$stage_dir"
	mkdir -p "$stage_dir"

	if [ "$goarm" = "-" ]; then
		(
			cd "$repo_root"
			env \
				CGO_ENABLED=0 \
				GOFLAGS="$release_goflags" \
				GOOS="$goos" \
				GOARCH="$goarch" \
				go build -ldflags "$release_ldflags" \
					-o "$stage_dir/$binary_name" \
					./cmd/dns-update
		)
	else
		(
			cd "$repo_root"
			env \
				CGO_ENABLED=0 \
				GOFLAGS="$release_goflags" \
				GOOS="$goos" \
				GOARCH="$goarch" \
				GOARM="$goarm" \
				go build -ldflags "$release_ldflags" \
					-o "$stage_dir/$binary_name" \
					./cmd/dns-update
		)
	fi

	cp "$repo_root/README.md" "$stage_dir/"
	cp "$repo_root/CHANGELOG.md" "$stage_dir/"
	cp "$repo_root/LICENSE" "$stage_dir/"
	cp "$repo_root/config.example.json" "$stage_dir/"
	cp "$repo_root/cloudflare.token.example" "$stage_dir/"
	cp -R "$repo_root/deploy" "$stage_dir/deploy"
	normalize_tree_timestamps "$stage_dir" "$source_epoch"

	case "$archive_format" in
	tar.gz)
		if is_gnu_tar "$release_tar_cmd"; then
			GZIP=-n TZ=UTC LC_ALL=C "$release_tar_cmd" \
				--sort=name \
				--owner=0 \
				--group=0 \
				--numeric-owner \
				--mtime="@$source_epoch" \
				-C "$tmp_dir" \
				-czf "$out_dir/$stage_name.tar.gz" \
				"$stage_name"
			return 0
		fi
		COPYFILE_DISABLE=1 GZIP=-n TZ=UTC LC_ALL=C "$release_tar_cmd" \
			-C "$tmp_dir" \
			-czf "$out_dir/$stage_name.tar.gz" \
			"$stage_name"
		;;
	zip)
		require_cmd zip
		(
			cd "$tmp_dir"
			find "$stage_name" -print | LC_ALL=C sort | zip -X -q "$out_dir/$stage_name.zip" -@
		)
		;;
	*)
		echo "unsupported archive format: $archive_format" >&2
		exit 1
		;;
	esac
}

if [ "$skip_archives" != 1 ]; then
	release_archive_targets | while read -r goos goarch goarm suffix archive_format; do
		[ -n "$goos" ] || continue
		build_archive "$goos" "$goarch" "$goarm" "$suffix" "$archive_format"
	done
fi

if [ "$skip_packages" != 1 ]; then
	PACKAGING_SKIP_NATIVE_TESTS=1 \
	PACKAGING_SKIP_SIGN=1 \
		"$repo_root/packaging/build-deb.sh"

	PACKAGING_SKIP_NATIVE_TESTS=1 \
	PACKAGING_SKIP_SIGN=1 \
		"$repo_root/packaging/build-rpm.sh"

	find "$repo_root/out/packages/deb" -type f -name '*.deb' -exec cp {} "$out_dir/" \;
	find "$repo_root/out/packages/rpm" -type f -name '*.rpm' -exec cp {} "$out_dir/" \;
fi

if [ "$write_checksums" = 1 ]; then
	(
		cd "$out_dir"
		artifacts=$(find . -maxdepth 1 -type f ! -name '*.sigstore.json' | sed 's#^\./##' | LC_ALL=C sort)
		if [ -z "$artifacts" ]; then
			echo "no release artifacts produced" >&2
			exit 1
		fi
		: > checksums.txt
		printf '%s\n' "$artifacts" | while IFS= read -r artifact; do
			[ -n "$artifact" ] || continue
			append_checksum checksums.txt "$artifact"
		done
	)
fi
