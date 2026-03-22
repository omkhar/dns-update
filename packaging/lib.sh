#!/bin/sh

package_targets() {
	printf '%s\n' amd64 rpi32 rpi64
}

default_targets() {
	package_targets
}

release_archive_targets() {
	cat <<'EOF'
linux amd64 - linux_amd64 tar.gz
linux arm64 - linux_arm64 tar.gz
linux arm 7 linux_armv7 tar.gz
darwin amd64 - darwin_amd64 tar.gz
darwin arm64 - darwin_arm64 tar.gz
windows amd64 - windows_amd64 zip
windows arm64 - windows_arm64 zip
EOF
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

release_tar() {
	command -v gtar 2>/dev/null || command -v tar
}

is_gnu_tar() {
	command -v "$1" >/dev/null 2>&1 || return 1
	"$1" --version 2>/dev/null | grep -qi 'gnu tar'
}

sha256_tool() {
	command -v sha256sum 2>/dev/null || command -v shasum
}

source_date_epoch() {
	repo_root=$1
	if [ -n "${SOURCE_DATE_EPOCH:-}" ]; then
		printf '%s\n' "$SOURCE_DATE_EPOCH"
		return 0
	fi

	if command -v git >/dev/null 2>&1; then
		if epoch=$(git -C "$repo_root" log -1 --format=%ct 2>/dev/null); then
			if [ -n "$epoch" ]; then
				printf '%s\n' "$epoch"
				return 0
			fi
		fi
	fi

	date +%s
}

touch_timestamp() {
	epoch=$1
	if date -u -r "$epoch" +%Y%m%d%H%M.%S >/dev/null 2>&1; then
		date -u -r "$epoch" +%Y%m%d%H%M.%S
		return 0
	fi
	date -u -d "@$epoch" +%Y%m%d%H%M.%S
}

normalize_tree_timestamps() {
	root=$1
	epoch=$2
	stamp=$(touch_timestamp "$epoch")
	find "$root" -exec touch -t "$stamp" {} +
}

write_checksums() {
	output=$1
	shift

	tool=$(sha256_tool)
	tool_name=$(basename "$tool")
	require_cmd "$tool_name"

	: > "$output"
	for artifact in "$@"; do
		append_checksum "$output" "$artifact"
	done
}

append_checksum() {
	output=$1
	artifact=$2
	tool=$(sha256_tool)
	tool_name=$(basename "$tool")
	require_cmd "$tool_name"

	case "$tool_name" in
	sha256sum)
		"$tool" "$artifact" >> "$output"
		;;
	shasum)
		"$tool" -a 256 "$artifact" >> "$output"
		;;
	*)
		echo "unsupported checksum tool: $tool_name" >&2
		exit 1
		;;
	esac
}

resolve_package_targets() {
	if [ "$#" -eq 0 ]; then
		package_targets
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

resolve_targets() {
	resolve_package_targets "$@"
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
