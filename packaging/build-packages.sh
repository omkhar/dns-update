#!/bin/sh
set -eu

# shellcheck source=packaging/lib.sh
. "$(dirname "$0")/lib.sh"

script_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
repo_root=$(repo_root "$0")

run_native_tests "$repo_root"

PACKAGING_SKIP_NATIVE_TESTS=1 "$script_dir/build-deb.sh" "$@"
PACKAGING_SKIP_NATIVE_TESTS=1 "$script_dir/build-rpm.sh" "$@"
