#!/usr/bin/env bash
# Install the latest metapi release binary from GitHub Releases.
#
# Usage:
#   curl -fsSL https://github.com/DeliciousBuding/metapi-go/releases/latest/download/install.sh | bash
#   # or with a specific version:
#   METAPI_VERSION=v0.9.0 bash <(curl -fsSL ...)
set -euo pipefail

REPO="DeliciousBuding/metapi-go"
VERSION="${METAPI_VERSION:-latest}"
PREFIX="${METAPI_INSTALL_PREFIX:-/usr/local}"

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
arch="$(uname -m)"
case "$arch" in
  x86_64 | amd64) arch="amd64" ;;
  aarch64 | arm64) arch="arm64" ;;
  *) echo "unsupported architecture: $arch" >&2; exit 1 ;;
esac

case "$os" in
  linux | darwin) binary="metapi-${os}-${arch}" ;;
  *) echo "unsupported OS: $os (Windows users: download metapi-windows-amd64.exe from the Release page)" >&2; exit 1 ;;
esac

if [ "$VERSION" = "latest" ]; then
  base="https://github.com/${REPO}/releases/latest/download"
else
  base="https://github.com/${REPO}/releases/download/${VERSION}"
fi
url="${base}/${binary}"

echo "Downloading ${binary} (${VERSION})..."
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
curl -fsSL "$url" -o "$tmp/metapi"

# Verify the binary against the release checksums manifest.
checksums_url="${base}/checksums.txt"
curl -fsSL "$checksums_url" -o "$tmp/checksums.txt"
expected="$(awk -v b="$binary" '$2 == b { print $1 }' "$tmp/checksums.txt")"
if [ -z "$expected" ]; then
  echo "checksums.txt did not contain an entry for ${binary}" >&2
  exit 1
fi
if command -v sha256sum >/dev/null 2>&1; then
  actual="$(sha256sum "$tmp/metapi" | awk '{print $1}')"
else
  actual="$(shasum -a 256 "$tmp/metapi" | awk '{print $1}')"
fi
if [ "$expected" != "$actual" ]; then
  echo "checksum mismatch for ${binary}: expected ${expected}, got ${actual}" >&2
  exit 1
fi

chmod +x "$tmp/metapi"

install_dir="${PREFIX}/bin"
install -d "$install_dir"
install -m 0755 "$tmp/metapi" "$install_dir/metapi"

echo "Installed metapi to ${install_dir}/metapi"
"$install_dir/metapi" --version
echo ""
echo "Next steps:"
echo "  export AUTH_TOKEN=<admin-token> PROXY_TOKEN=<proxy-token>"
echo "  metapi"
