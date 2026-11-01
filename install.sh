#!/bin/sh
# Install the latest agentsync release from GitHub.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/x0c/agentsync/main/install.sh | sh
#
# Environment:
#   AGENTSYNC_INSTALL_DIR  installation directory (default: ~/.local/bin)
#   AGENTSYNC_VERSION      version to install, e.g. 1.4.0 or v1.4.0 (default: latest)
#   AGENTSYNC_REPOSITORY   GitHub owner/name (default: x0c/agentsync)
#
# The installer verifies the downloaded archive against the release
# checksums.txt (SHA-256) before installing. It never uses sudo: if the
# installation directory is not writable it exits with an error instead of
# elevating privileges. No secrets, no telemetry.

set -eu

REPOSITORY="${AGENTSYNC_REPOSITORY:-x0c/agentsync}"
INSTALL_DIR="${AGENTSYNC_INSTALL_DIR:-$HOME/.local/bin}"
VERSION="${AGENTSYNC_VERSION:-latest}"

say() {
    printf 'agentsync: %s\n' "$*"
}

fail() {
    printf 'agentsync: error: %s\n' "$*" >&2
    exit 1
}

if [ -z "${AGENTSYNC_INSTALL_DIR:-}" ] && [ -z "${HOME:-}" ]; then
    fail "HOME is unset; set AGENTSYNC_INSTALL_DIR to choose an installation directory"
fi
INSTALL_DIR="${INSTALL_DIR%/}"

owner="${REPOSITORY%%/*}"
rest="${REPOSITORY#*/}"
case "$owner" in
    ''|*[!A-Za-z0-9_.-]*) fail "invalid repository '$REPOSITORY' (expected owner/name)" ;;
esac
case "$rest" in
    ''|*[!A-Za-z0-9_.-]*|*/*) fail "invalid repository '$REPOSITORY' (expected owner/name)" ;;
esac

if [ "$VERSION" = "latest" ]; then
    DOWNLOAD_BASE="https://github.com/${REPOSITORY}/releases/latest/download"
    TAG_LABEL="the latest release"
else
    case "$VERSION" in
        v*) TAG="$VERSION" ;;
        *) TAG="v$VERSION" ;;
    esac
    case "$TAG" in
        v[0-9]*.[0-9]*.[0-9]*) ;;
        *) fail "invalid version '$VERSION' (expected something like 1.4.0 or v1.4.0, or 'latest')" ;;
    esac
    DOWNLOAD_BASE="https://github.com/${REPOSITORY}/releases/download/${TAG}"
    TAG_LABEL="$TAG"
fi

OS="$(uname -s)"
case "$OS" in
    Darwin) GOOS=darwin ;;
    Linux) GOOS=linux ;;
    *) fail "unsupported operating system '$OS' (agentsync provides builds for macOS and Linux)" ;;
esac

ARCH="$(uname -m)"
case "$ARCH" in
    x86_64|amd64) GOARCH=amd64 ;;
    aarch64|arm64) GOARCH=arm64 ;;
    *) fail "unsupported architecture '$ARCH' (agentsync provides builds for amd64 and arm64)" ;;
esac

command -v curl >/dev/null 2>&1 || fail "required command 'curl' was not found on PATH"
command -v tar >/dev/null 2>&1 || fail "required command 'tar' was not found on PATH"

TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/agentsync-install.XXXXXX")"
cleanup() {
    rm -rf "$TMP_DIR"
}
trap cleanup EXIT INT TERM

ARCHIVE="agentsync_${GOOS}_${GOARCH}.tar.gz"
say "Downloading $TAG_LABEL for ${GOOS}/${GOARCH}..."
curl -fsSL -o "$TMP_DIR/$ARCHIVE" "$DOWNLOAD_BASE/$ARCHIVE" \
    || fail "could not download $DOWNLOAD_BASE/$ARCHIVE"
curl -fsSL -o "$TMP_DIR/checksums.txt" "$DOWNLOAD_BASE/checksums.txt" \
    || fail "could not download $DOWNLOAD_BASE/checksums.txt"

EXPECTED="$(grep -F "  $ARCHIVE" "$TMP_DIR/checksums.txt" | cut -d ' ' -f 1 || true)"
if [ -z "$EXPECTED" ]; then
    fail "release $TAG_LABEL does not publish $ARCHIVE"
fi
case "$EXPECTED" in
    *[!0-9a-fA-F]*|'') fail "the release checksum file is invalid" ;;
esac
if [ "${#EXPECTED}" -ne 64 ]; then
    fail "the release checksum file is invalid"
fi

if command -v sha256sum >/dev/null 2>&1; then
    ACTUAL="$(sha256sum "$TMP_DIR/$ARCHIVE" | cut -d ' ' -f 1)"
elif command -v shasum >/dev/null 2>&1; then
    ACTUAL="$(shasum -a 256 "$TMP_DIR/$ARCHIVE" | cut -d ' ' -f 1)"
else
    fail "neither 'sha256sum' nor 'shasum' was found on PATH (install GNU coreutils to verify the download)"
fi
if [ "$ACTUAL" != "$EXPECTED" ]; then
    fail "SHA-256 verification failed for $ARCHIVE (the download may be corrupted or tampered with)"
fi
say "SHA-256 verification passed."

tar -xzf "$TMP_DIR/$ARCHIVE" -C "$TMP_DIR" \
    || fail "could not extract $ARCHIVE"
if [ ! -f "$TMP_DIR/agentsync" ]; then
    fail "the release archive does not contain the agentsync binary"
fi

mkdir -p "$INSTALL_DIR" || fail "could not create directory $INSTALL_DIR"
if [ ! -w "$INSTALL_DIR" ]; then
    fail "directory $INSTALL_DIR is not writable (set AGENTSYNC_INSTALL_DIR to a writable directory; this installer never uses sudo)"
fi
cp "$TMP_DIR/agentsync" "$INSTALL_DIR/agentsync" \
    || fail "could not write to $INSTALL_DIR/agentsync"
chmod 755 "$INSTALL_DIR/agentsync"

say "Installed $TAG_LABEL to $INSTALL_DIR/agentsync"
case ":$PATH:" in
    *":$INSTALL_DIR:"*)
        say "Run 'agentsync --version' to verify the installation."
        ;;
    *)
        say "Run '$INSTALL_DIR/agentsync --version' to verify the installation."
        say "Note: $INSTALL_DIR is not on your PATH. Add it with:"
        say "  export PATH=\"$INSTALL_DIR:\$PATH\""
        ;;
esac
