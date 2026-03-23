#!/bin/sh
set -eu

# shellcheck source=packaging/lib.sh
. "$(dirname "$0")/lib.sh"

repo_root=$(repo_root "$0")
tmp_root=$(mktemp -d "$repo_root/out/reproducibility.XXXXXX")
out_one=$tmp_root/one
out_two=$tmp_root/two

cleanup() {
	rm -rf "$tmp_root"
}
trap cleanup EXIT INT TERM

build_once() {
	output_dir=$1
	tmp_dir=$2
	(
		cd "$repo_root"
		PACKAGING_FORCE_DIRECT_DEB="${PACKAGING_FORCE_DIRECT_DEB:-1}" \
		PACKAGING_LINUX_MACROS="${PACKAGING_LINUX_MACROS:-1}" \
		PACKAGING_SKIP_BUILDDEPS="${PACKAGING_SKIP_BUILDDEPS:-1}" \
		PACKAGING_SKIP_SIGN=1 \
		RELEASE_SKIP_PACKAGES="${RELEASE_SKIP_PACKAGES:-0}" \
		RELEASE_OUTPUT_DIR="$output_dir" \
		RELEASE_TMP_DIR="$tmp_dir" \
			./packaging/build-release-assets.sh
	)
}

build_once "$out_one" "$tmp_root/tmp-one"
build_once "$out_two" "$tmp_root/tmp-two"

diff -u "$out_one/checksums.txt" "$out_two/checksums.txt"
