#!/bin/sh

default_targets() {
	printf '%s\n' amd64 rpi32 rpi64
}

require_cmd() {
	if ! command -v "$1" >/dev/null 2>&1; then
		echo "missing required command: $1" >&2
		exit 1
	fi
}

script_dir() {
	CDPATH='' cd -- "$(dirname -- "$1")" && pwd
}

repo_root() {
	CDPATH='' cd -- "$(script_dir "$1")/.." && pwd
}

parse_version_field() {
	repo_root=$1
	sed -n '1s/^dns-update (\([^)]*\)) .*/\1/p' "$repo_root/debian/changelog"
}

package_version() {
	repo_root=$1
	version_field=$(parse_version_field "$repo_root")
	printf '%s\n' "${version_field%-*}"
}

package_release() {
	repo_root=$1
	version_field=$(parse_version_field "$repo_root")
	printf '%s\n' "${version_field##*-}"
}

release_goflags() {
	printf '%s\n' "-mod=readonly -trimpath -buildvcs=false"
}

release_ldflags() {
	printf '%s\n' "-s -w -buildid="
}

resolve_targets() {
	if [ "$#" -eq 0 ]; then
		default_targets
		return 0
	fi

	for target in "$@"; do
		case "$target" in
		amd64 | rpi32 | rpi64)
			printf '%s\n' "$target"
			;;
		*)
			echo "unsupported target: $target" >&2
			echo "supported targets: amd64 rpi32 rpi64" >&2
			exit 1
			;;
		esac
	done
}

deb_arch_for_target() {
	case "$1" in
	amd64)
		printf '%s\n' amd64
		;;
	rpi32)
		printf '%s\n' armhf
		;;
	rpi64)
		printf '%s\n' arm64
		;;
	*)
		echo "unsupported Debian target: $1" >&2
		exit 1
		;;
	esac
}

rpm_target_for_target() {
	case "$1" in
	amd64)
		printf '%s\n' x86_64
		;;
	rpi32)
		printf '%s\n' armv7hl
		;;
	rpi64)
		printf '%s\n' aarch64
		;;
	*)
		echo "unsupported RPM target: $1" >&2
		exit 1
		;;
	esac
}

run_native_tests() {
	repo_root=$1
	if [ "${PACKAGING_SKIP_NATIVE_TESTS:-}" = 1 ]; then
		return 0
	fi
	(
		cd "$repo_root" || exit 1
		env GOTOOLCHAIN="${GOTOOLCHAIN:-go1.26.1+auto}" go test ./...
	)
}

prepare_clean_dir() {
	dir=$1
	rm -rf "$dir"
	mkdir -p "$dir"
}

sign_blob() {
	artifact=$1
	bundle_path=$artifact.sigstore.json

	if [ -n "${COSIGN_KEY:-}" ]; then
		cosign sign-blob --yes --key "$COSIGN_KEY" --bundle "$bundle_path" "$artifact"
		return 0
	fi

	cosign sign-blob --yes --bundle "$bundle_path" "$artifact"
}

sign_artifacts() {
	if [ "${PACKAGING_SKIP_SIGN:-}" = 1 ]; then
		return 0
	fi
	require_cmd cosign
	if [ "$#" -eq 0 ]; then
		echo "no artifacts to sign" >&2
		exit 1
	fi

	for artifact in "$@"; do
		sign_blob "$artifact"
	done
}
