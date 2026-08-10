#!/usr/bin/env bash
set -euo pipefail

RELEASE_BASE_URL="${DEVBOX_RELEASE_BASE_URL:-https://nexus.mijo.services/repository/public/devbox}"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"
VERSION="${DEVBOX_VERSION:-latest}"

case "$(uname -s)" in
    Darwin) OS="darwin" ;;
    Linux) OS="linux" ;;
    *)
        echo "Error: unsupported operating system: $(uname -s)" >&2
        exit 1
        ;;
esac

case "$(uname -m)" in
    x86_64|amd64) ARCH="amd64" ;;
    arm64|aarch64) ARCH="arm64" ;;
    *)
        echo "Error: unsupported architecture: $(uname -m)" >&2
        exit 1
        ;;
esac

if [[ "$VERSION" == "latest" ]]; then
    VERSION="$(curl -fsSL "$RELEASE_BASE_URL/latest/version.txt")"
fi
if [[ ! "$VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$ ]]; then
    echo "Error: invalid release version: $VERSION" >&2
    exit 1
fi

ARCHIVE="devbox_${VERSION}_${OS}_${ARCH}.tar.gz"
TEMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TEMP_DIR"' EXIT

curl -fsSL "$RELEASE_BASE_URL/$VERSION/$ARCHIVE" -o "$TEMP_DIR/$ARCHIVE"
curl -fsSL "$RELEASE_BASE_URL/$VERSION/checksums.txt" -o "$TEMP_DIR/checksums.txt"

EXPECTED="$(awk -v archive="$ARCHIVE" '$2 == archive || $2 == "*" archive {print $1; exit}' "$TEMP_DIR/checksums.txt")"
if [[ -z "$EXPECTED" ]]; then
    echo "Error: checksum not found for $ARCHIVE" >&2
    exit 1
fi
if command -v sha256sum >/dev/null 2>&1; then
    ACTUAL="$(sha256sum "$TEMP_DIR/$ARCHIVE" | awk '{print $1}')"
else
    ACTUAL="$(shasum -a 256 "$TEMP_DIR/$ARCHIVE" | awk '{print $1}')"
fi
if [[ "$ACTUAL" != "$EXPECTED" ]]; then
    echo "Error: checksum verification failed for $ARCHIVE" >&2
    exit 1
fi

tar -xzf "$TEMP_DIR/$ARCHIVE" -C "$TEMP_DIR"
mkdir -p "$INSTALL_DIR"
install -m 0755 "$TEMP_DIR/devbox" "$INSTALL_DIR/devbox"

echo "Installed devbox $VERSION to $INSTALL_DIR/devbox"
