#!/bin/sh
# Updates the Homebrew formula in Marcone-Santos1/homebrew-tap for a release.
# Usage: scripts/update-tap.sh v0.1.0   (needs the GitHub CLI, logged in)
set -eu

tag=${1:?usage: $0 vX.Y.Z}
version=${tag#v}
repo=Marcone-Santos1/argus
tap=Marcone-Santos1/homebrew-tap

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

gh release download "$tag" --repo "$repo" --pattern checksums.txt --dir "$work"
sum() { awk -v f="argus_${version}_$1.tar.gz" '$2 == f { print $1 }' "$work/checksums.txt"; }
url() { echo "https://github.com/$repo/releases/download/$tag/argus_${version}_$1.tar.gz"; }

gh repo clone "$tap" "$work/tap" -- --quiet
# Commit with the GitHub no-reply address, never the personal e-mail.
login=$(gh api user -q .login)
git -C "$work/tap" config user.email "$(gh api user -q .id)+$login@users.noreply.github.com"
git -C "$work/tap" config user.name "$(gh api user -q '.name // .login')"
mkdir -p "$work/tap/Formula"
cat > "$work/tap/Formula/argus-panel.rb" <<EOF
class ArgusPanel < Formula
  desc "Terminal panel for running many coding-agent sessions side by side on tmux"
  homepage "https://github.com/$repo"
  version "$version"
  license "MIT"

  depends_on "tmux"

  conflicts_with "argus", because: "both install an \`argus\` binary"

  on_macos do
    on_arm do
      url "$(url darwin_arm64)"
      sha256 "$(sum darwin_arm64)"
    end
    on_intel do
      url "$(url darwin_amd64)"
      sha256 "$(sum darwin_amd64)"
    end
  end

  on_linux do
    on_arm do
      url "$(url linux_arm64)"
      sha256 "$(sum linux_arm64)"
    end
    on_intel do
      url "$(url linux_amd64)"
      sha256 "$(sum linux_amd64)"
    end
  end

  def install
    bin.install "argus"
  end

  def caveats
    <<~TEXT
      Run once to add the status hooks and the tmux config:
        argus install
      Then open the panel with:
        argus
    TEXT
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/argus version")
  end
end
EOF

cd "$work/tap"
git add Formula/argus-panel.rb
if git diff --cached --quiet; then
  echo "formula already up to date"
  exit 0
fi
git commit -q -m "argus-panel $version"
git push -q
echo "updated $tap: argus-panel $version"
