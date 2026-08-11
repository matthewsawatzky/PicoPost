#!/bin/sh
# PicoPost install script.
#
#   curl -fsSL https://raw.githubusercontent.com/matthewsawatzky/PicoPost/main/install.sh | sh
#
# Installs the picopost binary to ~/.local/bin (or ~/bin if it exists).
# Prefers a prebuilt release binary; falls back to building from source
# with Go when no release matches your platform.
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

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

if command -v curl >/dev/null 2>&1; then
  fetch="curl -fsSL"
elif command -v wget >/dev/null 2>&1; then
  fetch="wget -qO-"
else
  die "need curl or wget to download PicoPost"
fi

if [ "$VERSION" = "latest" ]; then
  release_url="https://api.github.com/repos/$REPO/releases/latest"
  VERSION="$($fetch "$release_url" | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -1)"
  [ -n "$VERSION" ] || VERSION=""
fi

if [ -n "$VERSION" ]; then
  asset="picopost-${OS}-${ARCH}"
  url="https://github.com/$REPO/releases/download/$VERSION/$asset"
  say "downloading $url"
  if $fetch "$url" -o "$tmpdir/picopost" 2>/dev/null; then
    chmod +x "$tmpdir/picopost"
    mv "$tmpdir/picopost" "$DEST/picopost"
    say "installed picopost $VERSION to $DEST/picopost"
    "$DEST/picopost" version
    exit 0
  fi
  say "no prebuilt binary for $OS/$ARCH at $VERSION; building from source"
fi

# --- build from source ----------------------------------------------------

if ! command -v go >/dev/null 2>&1; then
  die "no release binary for $OS/$ARCH and go is not installed; install Go (https://go.dev/dl) and re-run"
fi

say "building picopost from source"
git clone --depth 1 "https://github.com/$REPO.git" "$tmpdir/src" 2>/dev/null || {
  $fetch "https://github.com/$REPO/archive/refs/heads/main.tar.gz" | tar -xz -C "$tmpdir"
  mv "$tmpdir/PicoPost-main" "$tmpdir/src"
}
(
  cd "$tmpdir/src"
  go build -o "$DEST/picopost" ./cmd/picopost
)
say "installed picopost to $DEST/picopost"
"$DEST/picopost" version
