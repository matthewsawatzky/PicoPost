#!/bin/sh
# PicoPost install script.
#
#   curl -fsSL https://raw.githubusercontent.com/matthewsawatzky/PicoPost/main/install.sh | sh
#
# Installs the picopost binary into the current directory (./picopost).
# Prefers a prebuilt release binary; falls back to building from source
# with Go when no release matches your platform. A source build leaves
# the checkout in ./picopost-src in the directory where you run the script.
#
# Options:
#   PICOPOST_VERSION=<ver>   install a specific version (default: latest)
#   PICOPOST_SOURCE=1        always build from source
#   PICOPOST_DIR=<path>      install the binary somewhere else
#                            (e.g. ~/.local/bin to put it on your PATH)
#
# POSIX sh, works on macOS and Linux.

set -eu

REPO="matthewsawatzky/PicoPost"
VERSION="${PICOPOST_VERSION:-latest}"
FORCE_SOURCE="${PICOPOST_SOURCE:-0}"

say() { printf '\033[1;34m%s\033[0m\n' "$*"; }
die() { printf '\033[1;31m%s\033[0m\n' "$*" >&2; exit 1; }

# maybe_setup <binary> — ask whether to run the interactive setup wizard.
# Only prompts when stdin is a terminal.
maybe_setup() {
  local bin="$1"
  if [ ! -t 0 ]; then
    say "tip: run \"$bin setup\" to create a picopost.toml interactively"
    return
  fi
  printf 'Run the interactive setup wizard now to create a picopost.toml? [y/N] '
  read -r answer
  case "$answer" in
    y|Y|yes)
      "$bin" setup
      ;;
    *)
      say "ok — run \"$bin setup\" whenever you are ready"
      ;;
  esac
}

# --- destination ----------------------------------------------------------

# Default: the directory where the script is run.
DEST="${PICOPOST_DIR:-$PWD}"
mkdir -p "$DEST"

if [ "$DEST" != "$PWD" ] && ! echo "$PATH" | grep -q "$DEST"; then
  say "note: $DEST is not on your PATH; add it with:"
  say "  export PATH=\"\$DEST:\$PATH\""
fi

# --- platform -------------------------------------------------------------

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) die "unsupported architecture: $ARCH" ;;
esac

case "$OS" in
  linux|darwin) ;;
  *) die "unsupported OS: $OS" ;;
esac

# --- download release -----------------------------------------------------

if command -v curl >/dev/null 2>&1; then
  fetch="curl -fsSL"
elif command -v wget >/dev/null 2>&1; then
  fetch="wget -qO-"
else
  die "need curl or wget to download PicoPost"
fi

if [ "$VERSION" = "latest" ] && [ "$FORCE_SOURCE" != "1" ]; then
  # No releases yet? The API call 404s; that is fine, we build from source.
  VERSION="$($fetch "https://api.github.com/repos/$REPO/releases/latest" 2>/dev/null | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -1)"
fi

if [ -n "$VERSION" ] && [ "$FORCE_SOURCE" != "1" ]; then
  asset="picopost-${OS}-${ARCH}"
  url="https://github.com/$REPO/releases/download/$VERSION/$asset"
  say "installing picopost $VERSION ($OS/$ARCH)"
  say "downloading $url"
  tmpdir="$(mktemp -d)"
  if $fetch "$url" -o "$tmpdir/picopost" 2>/dev/null; then
    chmod +x "$tmpdir/picopost"
    mv "$tmpdir/picopost" "$DEST/picopost"
    rm -rf "$tmpdir"
    say "installed picopost $VERSION to $DEST/picopost"
    "$DEST/picopost" version
    maybe_setup "$DEST/picopost"
    exit 0
  fi
  rm -rf "$tmpdir"
  say "no prebuilt binary for $OS/$ARCH at $VERSION; building from source"
fi

# --- build from source ----------------------------------------------------

if ! command -v go >/dev/null 2>&1; then
  die "no release binary for $OS/$ARCH and go is not installed; install Go (https://go.dev/dl) and re-run"
fi

SRC_DIR="$PWD/picopost-src"
if [ -e "$SRC_DIR" ]; then
  die "$SRC_DIR already exists; move it away and re-run"
fi

say "building picopost from source (checkout left in $SRC_DIR)"
if command -v git >/dev/null 2>&1; then
  git clone --depth 1 "https://github.com/$REPO.git" "$SRC_DIR" 2>/dev/null || {
    rm -rf "$SRC_DIR"
    $fetch "https://github.com/$REPO/archive/refs/heads/main.tar.gz" | tar -xz -C "$PWD"
    mv "$PWD/PicoPost-main" "$SRC_DIR"
  }
else
  $fetch "https://github.com/$REPO/archive/refs/heads/main.tar.gz" | tar -xz -C "$PWD"
  mv "$PWD/PicoPost-main" "$SRC_DIR"
fi

SRC_VERSION="$(tr -d '[:space:]' < "$SRC_DIR/VERSION" 2>/dev/null || echo "dev")"
say "building picopost $SRC_VERSION from source"
(
  cd "$SRC_DIR"
  go build -ldflags "-X main.version=$SRC_VERSION" -o "$DEST/picopost" ./cmd/picopost
)
say "installed picopost $SRC_VERSION to $DEST/picopost"
say "source is at $SRC_DIR"
"$DEST/picopost" version
maybe_setup "$DEST/picopost"
