#!/bin/sh
# gaal installer.
#
# Downloads the latest release of gaal from GitHub Releases, verifies its
# SHA-256 checksum against the release's SHA256SUMS file, and installs it
# to $INSTALL_DIR (default $HOME/.local/bin).
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/getgaal/gaal/main/scripts/install.sh | sh
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
# These env vars are undocumented and exist only for the Go integration test
# at internal/installscript/install_test.go. Do not rely on them outside tests.
#   GAAL_INSTALL_BASE_URL      — reroute HTTP calls to a local httptest server
#   GAAL_INSTALL_ARCH_OVERRIDE — force detect_arch to see a fake `uname -m`
: "${GAAL_INSTALL_BASE_URL:=}"

if [ "${GAAL_INSTALL_DEBUG:-}" = "1" ]; then
  set -x
fi

# --- constants ---------------------------------------------------------------
REPO="getgaal/gaal"
BIN_NAME="gaal"
DEFAULT_INSTALL_DIR="$HOME/.local/bin"

# --- logging helpers ---------------------------------------------------------
log()  { printf '%s\n' "$*"; }
warn() { printf 'warning: %s\n' "$*" >&2; }
die()  { printf 'error: %s\n' "$*" >&2; exit 1; }

# --- arg parsing -------------------------------------------------------------
usage() {
  cat <<'EOF'
gaal installer

Usage:
  curl -fsSL https://raw.githubusercontent.com/getgaal/gaal/main/scripts/install.sh | sh
  curl -fsSL https://raw.githubusercontent.com/getgaal/gaal/main/scripts/install.sh | VERSION=v0.1.2 sh

Environment:
  VERSION              Release tag to install (default: latest from GitHub Releases)
  INSTALL_DIR          Install directory (default: $HOME/.local/bin)
  GAAL_INSTALL_DEBUG   Set to 1 for verbose tracing

Examples:
  # Install latest to ~/.local/bin
  curl -fsSL <URL> | sh

  # Pin to v0.1.2
  curl -fsSL <URL> | VERSION=v0.1.2 sh

  # System-wide install (will prompt for sudo on install)
  curl -fsSL <URL> | INSTALL_DIR=/usr/local/bin sh

Report issues: https://github.com/getgaal/gaal/issues
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
  # GAAL_INSTALL_ARCH_OVERRIDE is an undocumented test-only hook (see the
  # "test-only hooks" block near the top of this file) that lets the
  # integration test at internal/installscript/install_test.go force an
  # unsupported arch error without running on an unsupported host.
  if [ -n "${GAAL_INSTALL_ARCH_OVERRIDE:-}" ]; then
    uname_m="$GAAL_INSTALL_ARCH_OVERRIDE"
  else
    uname_m=$(uname -m 2>/dev/null || echo unknown)
  fi
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

# --- already-installed short-circuit -----------------------------------------
# If the binary already exists at the target path and reports the version we
# were about to install, exit 0 without touching anything. Dev builds (e.g.
# "v0.1.0-58-g324271c-dirty") never match a clean tag, so they always upgrade.
if [ -x "$INSTALL_DIR/$BIN_NAME" ]; then
  installed_version=$("$INSTALL_DIR/$BIN_NAME" version 2>/dev/null | awk '{print $2}' || echo "")
  if [ -n "$installed_version" ] && [ "$installed_version" = "$VERSION_TO_INSTALL" ]; then
    log "$BIN_NAME $installed_version already installed — nothing to do"
    exit 0
  fi
fi

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
target="$INSTALL_DIR/$BIN_NAME"
use_sudo=""

# Pick the directory whose writability decides whether we need sudo. If the
# install dir already exists, check it directly; otherwise check the nearest
# existing ancestor (so "$HOME/.local/bin" when ~/.local/bin doesn't exist
# yet still resolves to ~/.local or ~).
probe_dir="$INSTALL_DIR"
while [ ! -d "$probe_dir" ]; do
  parent=$(dirname "$probe_dir")
  if [ "$parent" = "$probe_dir" ]; then
    break
  fi
  probe_dir="$parent"
done

if [ ! -w "$probe_dir" ]; then
  if command -v sudo >/dev/null 2>&1; then
    log "note: $probe_dir is not writable; using sudo for install"
    use_sudo="sudo"
  else
    die "$probe_dir is not writable and sudo is not available"
  fi
fi

$use_sudo mkdir -p "$INSTALL_DIR" || die "failed to create $INSTALL_DIR"
$use_sudo mv "$tmpdir/$bin_asset" "$target" || die "failed to move binary to $target"
$use_sudo chmod +x "$target" || die "failed to chmod $target"

if [ "$OS" = "darwin" ]; then
  # Strip the quarantine xattr so Gatekeeper doesn't prompt on direct launch.
  # Darwin release binaries are already signed and notarized, so stripping
  # the flag is purely a UX refinement. The 2>/dev/null || true swallows
  # the error when the attribute is not set (e.g. fresh temp files from
  # mktemp never get quarantined in the first place).
  $use_sudo xattr -d com.apple.quarantine "$target" 2>/dev/null || true
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
