#!/bin/sh
set -eu

# shellcheck source=packaging/lib.sh
. "$(dirname "$0")/lib.sh"

usage() {
	cat >&2 <<'EOF'
usage:
  build-remote-container.sh --host HOST [--mode release-assets|reproducibility]
                            [--output-dir DIR] [--image IMAGE]
                            [--bootstrap-image IMAGE] [--image-tag TAG]
                            [--keep-remote-root] [--rebuild-image]

examples:
  ./packaging/build-remote-container.sh --host builder@example-build-host
  ./packaging/build-remote-container.sh --host builder@example-build-host \
    --bootstrap-image node:22-trixie-slim
  ./packaging/build-remote-container.sh --host builder@example-build-host --mode reproducibility
EOF
	exit 2
}

repo_root=$(repo_root "$0")
local_tar=$(release_tar)
source_epoch=$(source_date_epoch "$repo_root")
go_version=$(sed -n 's/^go //p' "$repo_root/go.mod")
container_path=/usr/local/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
remote_root=
go_arch=
local_config_path=${REMOTE_BUILD_LOCAL_CONFIG:-"$repo_root/packaging/build-remote-container.local.env"}

require_cmd ssh
require_cmd "$local_tar"

if [ -f "$local_config_path" ]; then
	set -a
	# shellcheck source=/dev/null
	. "$local_config_path"
	set +a
fi

host=${REMOTE_BUILD_HOST:-}
mode=${REMOTE_BUILD_MODE:-release-assets}
output_dir=
base_image=${REMOTE_BUILD_IMAGE:-golang:1.26.1-bookworm}
bootstrap_image=${REMOTE_BUILD_BOOTSTRAP_IMAGE:-}
image_tag=${REMOTE_BUILD_TAG:-}
keep_remote_root=0
rebuild_image=${REMOTE_BUILD_REBUILD_IMAGE:-0}

write_remote_dockerfile() {
	if [ -n "$bootstrap_image" ]; then
		cat <<EOF
FROM $bootstrap_image
RUN apt-get update \\
 && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \\
    ca-certificates \\
    curl \\
    diffutils \\
    rpm \\
    unzip \\
    zip \\
 && rm -rf /var/lib/apt/lists/* \\
 && curl -fsSL "https://go.dev/dl/go$go_version.linux-$go_arch.tar.gz" -o /tmp/go.tgz \\
 && rm -rf /usr/local/go \\
 && tar -C /usr/local -xzf /tmp/go.tgz \\
 && rm -f /tmp/go.tgz
ENV PATH=$container_path
EOF
		return 0
	fi

	cat <<EOF
FROM $base_image
RUN apt-get update \\
 && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \\
    ca-certificates \\
    diffutils \\
    rpm \\
    unzip \\
    zip \\
 && rm -rf /var/lib/apt/lists/*
EOF
}

while [ "$#" -gt 0 ]; do
	case "$1" in
	--host)
		shift
		[ "$#" -gt 0 ] || usage
		host=$1
		;;
	--mode)
		shift
		[ "$#" -gt 0 ] || usage
		mode=$1
		;;
	--output-dir)
		shift
		[ "$#" -gt 0 ] || usage
		output_dir=$1
		;;
	--image)
		shift
		[ "$#" -gt 0 ] || usage
		base_image=$1
		;;
	--bootstrap-image)
		shift
		[ "$#" -gt 0 ] || usage
		bootstrap_image=$1
		;;
	--image-tag)
		shift
		[ "$#" -gt 0 ] || usage
		image_tag=$1
		;;
	--keep-remote-root)
		keep_remote_root=1
		;;
	--rebuild-image)
		rebuild_image=1
		;;
	-h | --help)
		usage
		;;
	*)
		echo "unsupported argument: $1" >&2
		usage
		;;
	esac
	shift
done

[ -n "$host" ] || {
	echo "missing required --host (or REMOTE_BUILD_HOST)" >&2
	usage
}

case "$mode" in
release-assets | reproducibility)
	;;
*)
	echo "unsupported mode: $mode" >&2
	usage
	;;
esac

cleanup() {
	status=$1
	if [ -n "$remote_root" ] && [ "$keep_remote_root" -ne 1 ]; then
		ssh -o BatchMode=yes "$host" "sh -s -- '$remote_root'" <<'EOF' >/dev/null 2>&1 || true
set -eu
remote_root=$1
# Go module cache content can be read-only; relax perms before cleanup.
chmod -R u+w "$remote_root" 2>/dev/null || true
rm -rf "$remote_root"
EOF
	fi
	trap - EXIT INT TERM HUP
	exit "$status"
}
trap 'cleanup $?' EXIT INT TERM HUP

run_id=dns-update-$(date -u +%Y%m%dT%H%M%SZ)-$$
if [ -z "$output_dir" ]; then
	output_dir=$repo_root/out/remote/$run_id
fi
if [ -z "$image_tag" ]; then
	image_tag=dns-update-remote-build:$run_id
fi

printf 'creating remote workspace on %s\n' "$host"
remote_root=$(
	ssh -o BatchMode=yes "$host" "set -eu
		umask 077
		command -v docker >/dev/null 2>&1 || {
			echo 'missing required command: docker' >&2
			exit 1
		}
		command -v tar >/dev/null 2>&1 || {
			echo 'missing required command: tar' >&2
			exit 1
		}
		mkdir -p \"\$HOME/.codex-builds/dns-update\"
		mktemp -d \"\$HOME/.codex-builds/dns-update/$run_id.XXXXXX\""
)

if [ -n "$bootstrap_image" ]; then
	case "$(ssh -o BatchMode=yes "$host" "uname -m")" in
	x86_64 | amd64)
		go_arch=amd64
		;;
	aarch64 | arm64)
		go_arch=arm64
		;;
	*)
		echo "unsupported remote architecture for bootstrap image: $(ssh -o BatchMode=yes "$host" "uname -m")" >&2
		exit 1
		;;
	esac
fi

ssh -o BatchMode=yes "$host" "mkdir -p '$remote_root/image' '$remote_root/src'"
write_remote_dockerfile | ssh -o BatchMode=yes "$host" "cat > '$remote_root/image/Dockerfile'"

printf 'building or reusing remote build image %s\n' "$image_tag"
ssh -o BatchMode=yes "$host" "sh -s -- '$remote_root/image' '$image_tag' '$rebuild_image'" <<'EOF'
set -eu
image_dir=$1
image_tag=$2
rebuild_image=$3

set -- docker build -t "$image_tag"
if [ "$rebuild_image" = 1 ]; then
	set -- "$@" --no-cache
fi
"$@" "$image_dir"
EOF

printf 'syncing source snapshot to %s\n' "$host"
"$local_tar" -C "$repo_root" \
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
	-cf - . | ssh -o BatchMode=yes "$host" "tar -C '$remote_root/src' -xf -"

printf 'running %s inside dedicated remote container\n' "$mode"
ssh -o BatchMode=yes "$host" "sh -s -- '$remote_root' '$image_tag' '$mode' '$source_epoch'" <<'EOF'
set -eu
remote_root=$1
image_tag=$2
mode=$3
source_epoch=$4
workspace=$remote_root/src
uid=$(id -u)
gid=$(id -g)
container_name=$(basename "$remote_root")
container_path=/usr/local/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin

mkdir -p "$workspace/out/tmp"

docker run --rm \
	--name "$container_name" \
	--user "$uid:$gid" \
	--entrypoint /bin/sh \
	-e PATH="$container_path" \
	-e HOME=/workspace/.container-home \
	-e GOPATH=/workspace/.gopath \
	-e GOMODCACHE=/workspace/.gopath/pkg/mod \
	-e GOCACHE=/workspace/.cache/go-build \
	-e TMPDIR=/workspace/out/tmp \
	-e SOURCE_DATE_EPOCH="$source_epoch" \
	-e DEB_OUTPUT_DIR=/workspace/out/packages/deb \
	-e RPM_OUTPUT_DIR=/workspace/out/packages/rpm \
	-e RPM_TOPDIR_ROOT=/workspace/out/rpm \
	-e RPM_CANONICAL_TOPDIR_ROOT=/workspace/out/rpm-canonical \
	-e RELEASE_OUTPUT_DIR=/workspace/out/release \
	-e RELEASE_TMP_DIR=/workspace/out/release-tmp \
	-e PACKAGING_LINUX_MACROS=1 \
	-e PACKAGING_FORCE_DIRECT_DEB=1 \
	-e PACKAGING_SKIP_BUILDDEPS=1 \
	-e PACKAGING_SKIP_SIGN=1 \
	-e REMOTE_BUILD_MODE="$mode" \
	-w /workspace \
	-v "$workspace:/workspace" \
	"$image_tag" \
	-c '
		set -eu
		mkdir -p "$HOME" "$GOPATH" "$GOMODCACHE" "$GOCACHE" "$TMPDIR"
		case "$REMOTE_BUILD_MODE" in
		release-assets)
			./packaging/build-release-assets.sh
			set -- out/release/*
			[ -e "$1" ] || {
				echo "no release artifacts produced" >&2
				exit 1
			}
			./packaging/verify-release-assets.sh "$@"
			;;
		reproducibility)
			./packaging/check-release-reproducibility.sh
			;;
		*)
			echo "unsupported remote build mode: $REMOTE_BUILD_MODE" >&2
			exit 1
			;;
		esac
	'
EOF

mkdir -p "$output_dir"
cat >"$output_dir/remote-build.txt" <<EOF
host=$host
mode=$mode
run_id=$run_id
remote_root=$remote_root
base_image=$base_image
bootstrap_image=$bootstrap_image
image_tag=$image_tag
source_date_epoch=$source_epoch
EOF

if [ "$mode" = release-assets ]; then
	printf 'copying remote out/ tree to %s\n' "$output_dir"
	ssh -o BatchMode=yes "$host" "sh -s -- '$remote_root/src/out'" <<'EOF' | "$local_tar" -C "$output_dir" -xf -
set -eu
out_dir=$1
cd "$out_dir"
tar -cf - .
EOF
fi

printf 'remote container build complete\n'
printf 'local output: %s\n' "$output_dir"
if [ "$keep_remote_root" -eq 1 ]; then
	printf 'remote workspace kept: %s\n' "$remote_root"
fi
