#!/bin/sh
# gaal-lite installer.
#
# Downloads the latest release of gaal from GitHub Releases, verifies its
# SHA-256 checksum against the release's SHA256SUMS file, and installs it
# to $INSTALL_DIR (default $HOME/.local/bin).
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/gmg-inc/gaal-lite/main/scripts/install.sh | sh
#   curl -fsSL ... | VERSION=v0.1.2 sh
#   curl -fsSL ... | INSTALL_DIR=/usr/local/bin sh
#
# Environment:
#   VERSION               Release tag to install (default: latest).
#   INSTALL_DIR           Install directory (default: $HOME/.local/bin).
#   GAAL_INSTALL_DEBUG    Set to 1 to enable verbose tracing.
#
# All changes to this file require review per .github/CODEOWNERS.

set -eu

# --- test-only hooks ---------------------------------------------------------
# These env vars are undocumented and exist only so the Go integration test
# at internal/installscript/install_test.go can point the script at a local
# httptest server instead of api.github.com / github.com. Do not rely on
# them outside tests.
: "${GAAL_INSTALL_BASE_URL:=}"

if [ "${GAAL_INSTALL_DEBUG:-}" = "1" ]; then
  set -x
fi

# --- constants ---------------------------------------------------------------
REPO="gmg-inc/gaal-lite"
BIN_NAME="gaal"
DEFAULT_INSTALL_DIR="$HOME/.local/bin"

# --- logging helpers ---------------------------------------------------------
log()  { printf '%s\n' "$*"; }
warn() { printf 'warning: %s\n' "$*" >&2; }
die()  { printf 'error: %s\n' "$*" >&2; exit 1; }

# --- arg parsing -------------------------------------------------------------
usage() {
  cat <<'EOF'
gaal-lite installer

Usage:
  curl -fsSL https://raw.githubusercontent.com/gmg-inc/gaal-lite/main/scripts/install.sh | sh
  curl -fsSL https://raw.githubusercontent.com/gmg-inc/gaal-lite/main/scripts/install.sh | VERSION=v0.1.2 sh

Environment:
  VERSION              Release tag to install (default: latest from GitHub Releases)
  INSTALL_DIR          Install directory (default: $HOME/.local/bin)
  GAAL_INSTALL_DEBUG   Set to 1 for verbose tracing

Examples:
  # Install latest to ~/.local/bin
  curl -fsSL <URL> | sh

  # Pin to v0.1.2
  curl -fsSL <URL> | VERSION=v0.1.2 sh

  # System-wide install (will prompt for sudo on the final move)
  curl -fsSL <URL> | INSTALL_DIR=/usr/local/bin sh

Report issues: https://github.com/gmg-inc/gaal-lite/issues
EOF
}

for arg in "$@"; do
  case "$arg" in
    -h|--help) usage; exit 0 ;;
    *) die "unknown argument: $arg (see --help)" ;;
  esac
done

# --- platform detection ------------------------------------------------------
detect_os() {
  uname_s=$(uname -s 2>/dev/null || echo unknown)
  case "$uname_s" in
    Linux)  echo linux ;;
    Darwin) echo darwin ;;
    *)      die "unsupported operating system: $uname_s (supported: Linux, Darwin)" ;;
  esac
}

detect_arch() {
  uname_m=$(uname -m 2>/dev/null || echo unknown)
  case "$uname_m" in
    x86_64|amd64)  echo amd64 ;;
    aarch64|arm64) echo arm64 ;;
    *)             die "unsupported architecture: $uname_m (supported: amd64, arm64)" ;;
  esac
}

OS=$(detect_os)
ARCH=$(detect_arch)

# --- version resolution ------------------------------------------------------
resolve_version() {
  if [ -n "${VERSION:-}" ]; then
    echo "$VERSION"
    return
  fi
  if [ -n "$GAAL_INSTALL_BASE_URL" ]; then
    api_url="$GAAL_INSTALL_BASE_URL/repos/$REPO/releases/latest"
  else
    api_url="https://api.github.com/repos/$REPO/releases/latest"
  fi
  body=$(curl -fsSL "$api_url") || die "failed to query latest release at $api_url"
  # Extract "tag_name":"vX.Y.Z" with grep + sed — avoids a jq dependency.
  tag=$(printf '%s\n' "$body" | grep -o '"tag_name"[[:space:]]*:[[:space:]]*"[^"]*"' | sed 's/.*"\([^"]*\)"$/\1/')
  [ -n "$tag" ] || die "could not parse tag_name from $api_url response"
  echo "$tag"
}

VERSION_TO_INSTALL=$(resolve_version)

# --- install dir resolution --------------------------------------------------
INSTALL_DIR="${INSTALL_DIR:-$DEFAULT_INSTALL_DIR}"

# --- download, verify, install -----------------------------------------------
if [ -n "$GAAL_INSTALL_BASE_URL" ]; then
  download_base="$GAAL_INSTALL_BASE_URL/releases/download/$VERSION_TO_INSTALL"
else
  download_base="https://github.com/$REPO/releases/download/$VERSION_TO_INSTALL"
fi

bin_asset="${BIN_NAME}-${OS}-${ARCH}"

pick_sha_tool() {
  if command -v sha256sum >/dev/null 2>&1; then
    echo "sha256sum"
  elif command -v shasum >/dev/null 2>&1; then
    echo "shasum -a 256"
  else
    die "neither sha256sum nor shasum found on PATH"
  fi
}

SHA_CMD=$(pick_sha_tool)

tmpdir=$(mktemp -d 2>/dev/null || mktemp -d -t gaal-install)
[ -d "$tmpdir" ] || die "failed to create temp directory"
cleanup() { rm -rf "$tmpdir"; }
trap cleanup EXIT INT TERM

log "Downloading $bin_asset ($VERSION_TO_INSTALL) ..."
curl -fsSL "$download_base/$bin_asset" -o "$tmpdir/$bin_asset" \
  || die "failed to download $download_base/$bin_asset"
curl -fsSL "$download_base/SHA256SUMS" -o "$tmpdir/SHA256SUMS" \
  || die "failed to download $download_base/SHA256SUMS"

log "Verifying SHA-256 checksum ..."
actual=$(cd "$tmpdir" && $SHA_CMD "$bin_asset" | awk '{print $1}')
expected=$(awk -v name="$bin_asset" '$2 == name || $2 == "*"name {print $1}' "$tmpdir/SHA256SUMS")
[ -n "$expected" ] || die "no checksum entry for $bin_asset in SHA256SUMS"
if [ "$actual" != "$expected" ]; then
  die "checksum mismatch for $bin_asset (expected $expected, got $actual)"
fi

# --- install -----------------------------------------------------------------
mkdir -p "$INSTALL_DIR" || die "failed to create $INSTALL_DIR"

target="$INSTALL_DIR/$BIN_NAME"
use_sudo=""
if [ ! -w "$INSTALL_DIR" ]; then
  if command -v sudo >/dev/null 2>&1; then
    log "note: $INSTALL_DIR is not writable; using sudo for the final move"
    use_sudo="sudo"
  else
    die "$INSTALL_DIR is not writable and sudo is not available"
  fi
fi

$use_sudo mv "$tmpdir/$bin_asset" "$target" || die "failed to move binary to $target"
$use_sudo chmod +x "$target" || die "failed to chmod $target"

if [ "$OS" = "darwin" ]; then
  # Strip the quarantine xattr so Gatekeeper doesn't prompt on direct launch.
  # Darwin release binaries are already signed + notarized. 2>/dev/null || true
  # swallows the error when the attribute isn't set (fresh temp files).
  xattr -d com.apple.quarantine "$target" 2>/dev/null || true
fi

log "Installed $BIN_NAME $VERSION_TO_INSTALL to $target"

# --- PATH warning ------------------------------------------------------------
case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *)
    warn "$INSTALL_DIR is not on your PATH"
    warn "add it with: export PATH=\"$INSTALL_DIR:\$PATH\""
    ;;
esac
