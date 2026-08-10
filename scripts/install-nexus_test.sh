#!/usr/bin/env bash
set -euo pipefail

REPO_DIR="$(cd "$(dirname "$0")/.." && pwd)"
TEST_DIR="$(mktemp -d)"
trap 'rm -rf "$TEST_DIR"' EXIT

FIXTURE_ROOT="$TEST_DIR/releases"
VERSION="v0.4.0"
ARCHIVE="devbox_${VERSION}_darwin_arm64.tar.gz"
mkdir -p "$FIXTURE_ROOT/latest" "$FIXTURE_ROOT/$VERSION" "$TEST_DIR/payload" "$TEST_DIR/bin"

printf '%s\n' "$VERSION" > "$FIXTURE_ROOT/latest/version.txt"
cat > "$TEST_DIR/payload/devbox" <<'EOF'
#!/usr/bin/env bash
echo "devbox test binary"
EOF
chmod +x "$TEST_DIR/payload/devbox"
tar -czf "$FIXTURE_ROOT/$VERSION/$ARCHIVE" -C "$TEST_DIR/payload" devbox
(
    cd "$FIXTURE_ROOT/$VERSION"
    shasum -a 256 "$ARCHIVE" > checksums.txt
)

cat > "$TEST_DIR/bin/uname" <<'EOF'
#!/usr/bin/env bash
case "${1:-}" in
    -s) echo Darwin ;;
    -m) echo arm64 ;;
    *) exit 1 ;;
esac
EOF

cat > "$TEST_DIR/bin/curl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

output=""
url=""
while (($#)); do
    case "$1" in
        -o)
            output="$2"
            shift 2
            ;;
        -*)
            shift
            ;;
        *)
            url="$1"
            shift
            ;;
    esac
done

relative="${url#"$DEVBOX_RELEASE_BASE_URL/"}"
source_file="$FIXTURE_ROOT/$relative"
if [[ -n "$output" ]]; then
    cp "$source_file" "$output"
else
    cat "$source_file"
fi
EOF
chmod +x "$TEST_DIR/bin/uname" "$TEST_DIR/bin/curl"

INSTALL_DIR="$TEST_DIR/install" \
DEVBOX_RELEASE_BASE_URL="https://example.invalid/devbox" \
FIXTURE_ROOT="$FIXTURE_ROOT" \
PATH="$TEST_DIR/bin:$PATH" \
bash "$REPO_DIR/scripts/install-nexus.sh"

test -x "$TEST_DIR/install/devbox"
test "$("$TEST_DIR/install/devbox")" = "devbox test binary"

echo "install-nexus smoke test passed"
