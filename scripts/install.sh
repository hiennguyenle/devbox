#!/usr/bin/env bash
set -euo pipefail

INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"
REPO_DIR="$(cd "$(dirname "$0")/.." && pwd)"

# Get version from git tag, fallback to "dev"
VERSION=$(git -C "$REPO_DIR" describe --tags --always 2>/dev/null || echo "dev")

echo "Building devbox $VERSION..."
go build -ldflags "-s -w -X github.com/hien/devbox/cmd.version=$VERSION" -o "$REPO_DIR/devbox" "$REPO_DIR"

mkdir -p "$INSTALL_DIR"
cp "$REPO_DIR/devbox" "$INSTALL_DIR/devbox"
chmod +x "$INSTALL_DIR/devbox"

echo "Installed devbox $VERSION to $INSTALL_DIR/devbox"

# Check if INSTALL_DIR is in PATH
if ! echo "$PATH" | tr ':' '\n' | grep -q "^$INSTALL_DIR$"; then
    echo ""
    echo "Warning: $INSTALL_DIR is not in your PATH."
    echo "Add this to your shell profile:"
    echo "  export PATH=\"$INSTALL_DIR:\$PATH\""
fi
