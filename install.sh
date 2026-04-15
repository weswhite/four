#!/bin/sh
set -e

REPO="weswhite/four"

# Detect OS (WSL reports as Linux, which is correct)
OS="$(uname -s)"
case "$OS" in
  Linux*)          OS=linux ;;
  Darwin*)         OS=darwin ;;
  MINGW*|MSYS*|CYGWIN*) echo "Use WSL to run this script on Windows"; exit 1 ;;
  *)               echo "Unsupported OS: $OS"; exit 1 ;;
esac

# Detect architecture
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64)  ARCH=amd64 ;;
  aarch64) ARCH=arm64 ;;
  arm64)   ARCH=arm64 ;;
  *)       echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

# Get latest release tag
TAG="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')"
if [ -z "$TAG" ]; then
  echo "Error: could not determine latest release"
  exit 1
fi

ARCHIVE="four_${OS}_${ARCH}.tar.gz"
URL="https://github.com/${REPO}/releases/download/${TAG}/${ARCHIVE}"

echo "Downloading four ${TAG} (${OS}/${ARCH})..."
TMPDIR="$(mktemp -d)"
curl -fsSL "$URL" -o "${TMPDIR}/${ARCHIVE}"
tar -xzf "${TMPDIR}/${ARCHIVE}" -C "$TMPDIR"

# Install binary — prefer /usr/local/bin, fall back to ~/.local/bin (common on WSL)
INSTALL_DIR="/usr/local/bin"
if [ -w "$INSTALL_DIR" ]; then
  mv "${TMPDIR}/four" "${INSTALL_DIR}/four"
elif command -v sudo >/dev/null 2>&1; then
  echo "Installing to ${INSTALL_DIR} (requires sudo)..."
  sudo mv "${TMPDIR}/four" "${INSTALL_DIR}/four"
else
  INSTALL_DIR="$HOME/.local/bin"
  mkdir -p "$INSTALL_DIR"
  mv "${TMPDIR}/four" "${INSTALL_DIR}/four"
  case ":$PATH:" in
    *":$INSTALL_DIR:"*) ;;
    *) echo "Add ${INSTALL_DIR} to your PATH: export PATH=\"${INSTALL_DIR}:\$PATH\"" ;;
  esac
fi

rm -rf "$TMPDIR"
echo "four ${TAG} installed to ${INSTALL_DIR}/four"
