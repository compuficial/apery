#!/bin/sh
set -eu

REPO="compuficial/apery"
BIN="apery"

# Detect OS
OS=$(uname -s)
case "$OS" in
  Linux)  OS=linux ;;
  Darwin) OS=macos ;;
  *) echo "Unsupported OS: $OS" >&2; exit 1 ;;
esac

# Detect arch
ARCH=$(uname -m)
case "$ARCH" in
  x86_64)        ARCH=x86_64 ;;
  aarch64|arm64) ARCH=arm64 ;;
  *) echo "Unsupported arch: $ARCH" >&2; exit 1 ;;
esac

# Resolve version: VERSION env var overrides, else fetch latest stable release
if [ -z "${VERSION:-}" ]; then
  VERSION=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
    | grep '"tag_name"' | head -1 \
    | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')
  if [ -z "$VERSION" ]; then
    echo "Could not determine latest version. Set VERSION=vX.Y.Z to install a specific release." >&2
    exit 1
  fi
fi

# Goreleaser strips the leading 'v' from filenames
VER="${VERSION#v}"

URL="https://github.com/${REPO}/releases/download/${VERSION}/${BIN}_${VER}_${OS}_${ARCH}.tar.gz"

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

echo "Downloading ${BIN} ${VERSION} (${OS}/${ARCH})..."
curl -fsSL "$URL" | tar xz -C "$TMP"

# Pick install dir without requiring sudo
if [ -w /usr/local/bin ]; then
  DEST=/usr/local/bin
else
  DEST="${HOME}/.local/bin"
  mkdir -p "$DEST"
fi

mv "$TMP/${BIN}" "${DEST}/${BIN}"
chmod +x "${DEST}/${BIN}"

echo "Installed ${BIN} ${VERSION} → ${DEST}/${BIN}"

# Warn if install dir is not in PATH
case ":${PATH}:" in
  *":${DEST}:"*) ;;
  *) echo "Warning: add ${DEST} to your PATH to use ${BIN}" ;;
esac
