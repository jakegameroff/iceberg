#!/bin/sh
# iceberg.sh CLI installer
set -e

BASE="https://iceberg.sh"

# detect OS
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$OS" in
  linux|darwin) ;;
  *) echo "iceberg: unsupported OS: $OS" >&2; exit 1 ;;
esac

# detect arch
ARCH=$(uname -m)
case "$ARCH" in
  x86_64|amd64)  ARCH=amd64 ;;
  aarch64|arm64) ARCH=arm64 ;;
  *) echo "iceberg: unsupported arch: $ARCH" >&2; exit 1 ;;
esac

BIN="iceberg-${OS}-${ARCH}"
URL="${BASE}/dl/${BIN}"

TMP=$(mktemp)
trap 'rm -f "$TMP"' EXIT

echo "iceberg: downloading ${URL}"
if ! curl -fsSL "$URL" -o "$TMP"; then
  echo "iceberg: download failed (no build for ${OS}/${ARCH}?)" >&2
  exit 1
fi
chmod +x "$TMP"

DEST="/usr/local/bin/iceberg"
if [ -w "$(dirname "$DEST")" ]; then
  mv "$TMP" "$DEST"
else
  echo "iceberg: installing to ${DEST} (needs sudo)"
  sudo mv "$TMP" "$DEST"
fi
trap - EXIT

echo "iceberg: installed to ${DEST}"
echo "iceberg: try  ->  iceberg help"
