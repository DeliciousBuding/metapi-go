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
  url="https://github.com/${REPO}/releases/latest/download/${binary}"
else
  url="https://github.com/${REPO}/releases/download/${VERSION}/${binary}"
fi

echo "Downloading ${binary} (${VERSION})..."
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
curl -fsSL "$url" -o "$tmp/metapi"
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
