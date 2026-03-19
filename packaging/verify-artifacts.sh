#!/bin/sh
set -eu

usage() {
	cat >&2 <<'EOF'
usage:
  verify-artifacts.sh <artifact>...

Verification modes:
  keyless (default):
    set SIGSTORE_CERTIFICATE_IDENTITY and SIGSTORE_OIDC_ISSUER
  key-based:
    set COSIGN_KEY to a public key, KMS URI, or other supported cosign verifier
EOF
	exit 2
}

if [ "$#" -eq 0 ]; then
	usage
fi

if [ -n "${COSIGN_KEY:-}" ]; then
	mode=key
else
	mode=keyless
fi

if [ "$mode" = keyless ]; then
	if [ -z "${SIGSTORE_CERTIFICATE_IDENTITY:-}" ] || [ -z "${SIGSTORE_OIDC_ISSUER:-}" ]; then
		echo "SIGSTORE_CERTIFICATE_IDENTITY and SIGSTORE_OIDC_ISSUER are required for keyless verification" >&2
		exit 1
	fi
fi

if ! command -v cosign >/dev/null 2>&1; then
	echo "missing required command: cosign" >&2
	exit 1
fi

for artifact in "$@"; do
	bundle_path=$artifact.sigstore.json
	if [ ! -f "$bundle_path" ]; then
		echo "missing bundle for $artifact: $bundle_path" >&2
		exit 1
	fi

	if [ "$mode" = key ]; then
		cosign verify-blob "$artifact" --bundle "$bundle_path" --key "$COSIGN_KEY"
		continue
	fi

	cosign verify-blob "$artifact" \
		--bundle "$bundle_path" \
		--certificate-identity "$SIGSTORE_CERTIFICATE_IDENTITY" \
		--certificate-oidc-issuer "$SIGSTORE_OIDC_ISSUER"
done
