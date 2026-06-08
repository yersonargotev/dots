#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Install the dots CLI from a checksum-verified GitHub Release Artifact, then delegate setup to dots install.

Environment:
  DOTS_VERSION           Release tag to install, for example v0.1.0. Required.
  DOTS_RELEASE_BASE_URL  Release asset base URL. Defaults to https://github.com/yersonargotev/dots/releases/download.
  DOTS_INSTALL_DIR       Directory where the dots command is installed. Defaults to ~/.local/bin.
  DOTS_SOURCE_ROOT       Optional development checkout/Installed Repository override passed to dots install.
  DOTS_OS                Test override for platform OS detection.
  DOTS_ARCH              Test override for platform architecture detection.
USAGE
}

fail() {
  echo "dots bootstrap: $*" >&2
  exit 1
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

version="${DOTS_VERSION:-}"
release_base_url="${DOTS_RELEASE_BASE_URL:-https://github.com/yersonargotev/dots/releases/download}"
install_dir="${DOTS_INSTALL_DIR:-$HOME/.local/bin}"
source_root="${DOTS_SOURCE_ROOT:-}"

if [[ -z "$version" ]]; then
  fail "DOTS_VERSION is required, for example DOTS_VERSION=v0.1.0"
fi

case "${DOTS_OS:-$(uname -s)}" in
  Darwin|darwin) goos="darwin" ;;
  Linux|linux) goos="linux" ;;
  *) fail "unsupported operating system: ${DOTS_OS:-$(uname -s)}" ;;
esac

case "${DOTS_ARCH:-$(uname -m)}" in
  x86_64|amd64) goarch="amd64" ;;
  arm64|aarch64) goarch="arm64" ;;
  *) fail "unsupported architecture: ${DOTS_ARCH:-$(uname -m)}" ;;
esac

artifact="dots_${version}_${goos}_${goarch}"
asset_base="${release_base_url%/}/${version}"

download() {
  local url="$1"
  local output="$2"
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$url" -o "$output"
  elif command -v wget >/dev/null 2>&1; then
    wget -qO "$output" "$url"
  else
    fail "curl or wget is required to download release artifacts"
  fi
}

sha256_file() {
  local file="$1"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$file" | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$file" | awk '{print $1}'
  else
    fail "sha256sum or shasum is required for checksum verification"
  fi
}

tmp_dir="$(mktemp -d)"
cleanup() {
  rm -rf "$tmp_dir"
}
trap cleanup EXIT

checksums_path="$tmp_dir/checksums.txt"
artifact_path="$tmp_dir/$artifact"

echo "Downloading checksums for $version"
download "$asset_base/checksums.txt" "$checksums_path"

echo "Downloading $artifact"
download "$asset_base/$artifact" "$artifact_path"

expected_checksum="$(awk -v artifact="$artifact" '$2 == artifact { print $1 }' "$checksums_path")"
if [[ -z "$expected_checksum" ]]; then
  fail "checksums.txt does not contain an entry for $artifact"
fi

actual_checksum="$(sha256_file "$artifact_path")"
if [[ "$actual_checksum" != "$expected_checksum" ]]; then
  fail "checksum mismatch for $artifact"
fi

mkdir -p "$install_dir"
install_path="$install_dir/dots"
install -m 0755 "$artifact_path" "$install_path"

echo "Installed dots to $install_path"

args=(install)
if [[ -n "$source_root" ]]; then
  args+=(--source-root "$source_root")
fi

exec "$install_path" "${args[@]}"
