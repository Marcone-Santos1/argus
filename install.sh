#!/bin/sh
# Installs the latest argus release (macOS or Linux) without Homebrew.
#   curl -fsSL https://raw.githubusercontent.com/Marcone-Santos1/argus/main/install.sh | sh
# Options (environment):
#   ARGUS_VERSION=v0.1.0       a specific release (default: the latest)
#   ARGUS_INSTALL_DIR=~/bin    where to put the binary (default: ~/.local/bin)
set -eu

repo=Marcone-Santos1/argus
dir=${ARGUS_INSTALL_DIR:-$HOME/.local/bin}

case "$(uname -s)" in
  Darwin) os=darwin ;;
  Linux) os=linux ;;
  *) echo "argus: unsupported system: $(uname -s)" >&2; exit 1 ;;
esac
case "$(uname -m)" in
  arm64 | aarch64) arch=arm64 ;;
  x86_64 | amd64) arch=amd64 ;;
  *) echo "argus: unsupported architecture: $(uname -m)" >&2; exit 1 ;;
esac

tag=${ARGUS_VERSION:-}
if [ -z "$tag" ]; then
  # The /releases/latest page redirects to the latest tag.
  tag=$(curl -fsSLI -o /dev/null -w '%{url_effective}' "https://github.com/$repo/releases/latest")
  tag=${tag##*/}
fi
version=${tag#v}
name="argus_${version}_${os}_${arch}"
base="https://github.com/$repo/releases/download/$tag"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

echo "argus: downloading $tag ($os/$arch)"
curl -fsSL -o "$tmp/$name.tar.gz" "$base/$name.tar.gz"
curl -fsSL -o "$tmp/checksums.txt" "$base/checksums.txt"

want=$(awk -v f="$name.tar.gz" '$2 == f { print $1 }' "$tmp/checksums.txt")
if command -v sha256sum >/dev/null 2>&1; then
  got=$(sha256sum "$tmp/$name.tar.gz" | awk '{ print $1 }')
else
  got=$(shasum -a 256 "$tmp/$name.tar.gz" | awk '{ print $1 }')
fi
if [ -z "$want" ] || [ "$want" != "$got" ]; then
  echo "argus: checksum mismatch; nothing was installed" >&2
  exit 1
fi

tar -xzf "$tmp/$name.tar.gz" -C "$tmp"
mkdir -p "$dir"
install -m 0755 "$tmp/$name/argus" "$dir/argus"
echo "argus: installed $("$dir/argus" version) in $dir/argus"

if ! command -v tmux >/dev/null 2>&1; then
  echo "argus: tmux 3.4+ is required (brew install tmux / apt install tmux)"
fi
case ":$PATH:" in
  *":$dir:"*) ;;
  *) echo "argus: add $dir to your PATH" ;;
esac
echo "argus: next, run 'argus install' once, then 'argus'"
