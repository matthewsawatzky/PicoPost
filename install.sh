#!/bin/sh
# PicoPost install script.
#
#   curl -fsSL https://raw.githubusercontent.com/matthewsawatzky/PicoPost/main/install.sh | sh
#
# Installs the picopost binary to ~/.local/bin (or ~/bin if it exists).
# Prefers a prebuilt release binary; falls back to building from source
# with Go when no release matches your platform. The source checkout is
# left in ./picopost in the directory where you run the script.
#
# POSIX sh, works on macOS and Linux.

set -eu

REPO="matthewsawatzky/PicoPost"
VERSION="${PICOPOST_VERSION:-latest}"

say() { printf '\033[1;34m%s\033[0m\n' "$*"; }
die() { printf '\033[1;31m%s\033[0m\n' "$*" >&2; exit 1; }

# --- destination ----------------------------------------------------------

if [ -d "$HOME/bin" ]; then
  DEST="$HOME/bin"
elif [ -d "$HOME/.local/bin" ]; then
  DEST="$HOME/.local/bin"
else
  DEST="$HOME/.local/bin"
  mkdir -p "$DEST"
fi

if ! echo "$PATH" | grep -q "$DEST"; then
  say "note: $DEST is not on your PATH; add it with:"
  say "  export PATH=\"\$HOME/.local/bin:\$PATH\""
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

if [ "$VERSION" = "latest" ]; then
  # No releases yet? The API call 404s; that is fine, we build from source.
  VERSION="$($fetch "https://api.github.com/repos/$REPO/releases/latest" 2>/dev/null | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -1)"
fi

if [ -n "$VERSION" ]; then
  asset="picopost-${OS}-${ARCH}"
  url="https://github.com/$REPO/releases/download/$VERSION/$asset"
  say "downloading $url"
  tmpdir="$(mktemp -d)"
  if $fetch "$url" -o "$tmpdir/picopost" 2>/dev/null; then
    chmod +x "$tmpdir/picopost"
    mv "$tmpdir/picopost" "$DEST/picopost"
    rm -rf "$tmpdir"
    say "installed picopost $VERSION to $DEST/picopost"
    "$DEST/picopost" version
    exit 0
  fi
  rm -rf "$tmpdir"
  say "no prebuilt binary for $OS/$ARCH at $VERSION; building from source"
fi

# --- build from source ----------------------------------------------------

if ! command -v go >/dev/null 2>&1; then
  die "no release binary for $OS/$ARCH and go is not installed; install Go (https://go.dev/dl) and re-run"
fi

SRC_DIR="$PWD/picopost"
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

(
  cd "$SRC_DIR"
  go build -o "$DEST/picopost" ./cmd/picopost
)
say "installed picopost to $DEST/picopost"
say "source is at $SRC_DIR"
"$DEST/picopost" version
