#!/bin/sh
set -eu

. "$(dirname "$0")/lib.sh"

repo_root=$(repo_root "$0")
dist_dir=${1:-"$repo_root/dist"}

set -- \
	"$dist_dir"/checksums.txt \
	"$dist_dir"/dns-update_*.tar.gz \
	"$dist_dir"/dns-update_*.zip \
	"$repo_root"/out/packages/deb/*/*.deb \
	"$repo_root"/out/packages/rpm/*/*.rpm

for artifact in "$@"; do
	if [ -f "$artifact" ]; then
		printf '%s\n' "$artifact"
	fi
done
