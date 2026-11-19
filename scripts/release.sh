#!/bin/sh
# Releases InkStudio: a notarised DMG on a GitHub release, and the cask in
# the Homebrew tap, so `brew install --cask inkcheck/tap/inkstudio` gets it.
#
#   scripts/release.sh check     verify the prerequisites only
#   scripts/release.sh publish   verify, then release
#
# Tag the release first: git tag -a vX.Y.Z -m 'InkStudio vX.Y.Z' && git push
# origin vX.Y.Z. TAP_DIR is a clone of inkcheck/homebrew-tap (default:
# ../homebrew-tap). Signing and notarising use the keychain, as in
# scripts/package.sh.
set -eu
cd "$(dirname "$0")/.."

mode=${1:-check}
tap=${TAP_DIR:-../homebrew-tap}
problems=0
fail() { printf '  \033[31m✗\033[0m %s\n' "$1"; problems=$((problems + 1)); }
ok() { printf '  \033[32m✓\033[0m %s\n' "$1"; }

echo "Repository"
if [ -n "$(git status --porcelain)" ]; then
  fail "working tree has uncommitted changes"
else
  ok "working tree is clean"
fi
tag=$(git describe --tags --exact-match HEAD 2>/dev/null || true)
if printf '%s' "$tag" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+$'; then
  ok "HEAD is tagged $tag"
  if git ls-remote --exit-code --tags origin "refs/tags/$tag" >/dev/null 2>&1; then
    ok "tag $tag is pushed to origin"
  else
    fail "tag $tag is not on origin (git push origin $tag)"
  fi
else
  fail "HEAD has no vX.Y.Z tag (git tag -a vX.Y.Z -m 'InkStudio vX.Y.Z')"
fi
case "$(git remote get-url origin 2>/dev/null || true)" in
  *github.com[:/]inkcheck/inkstudio*) ok "origin is github.com/inkcheck/inkstudio" ;;
  "") fail "no origin remote (expected github.com/inkcheck/inkstudio)" ;;
  *) fail "origin is $(git remote get-url origin), expected github.com/inkcheck/inkstudio" ;;
esac
# A release builds from this repository alone.
if grep -q '^replace ' go.mod; then
  fail "go.mod replaces a module with a local folder; require a released inkpress instead"
else
  ok "go.mod uses released modules"
fi

echo "Signing"
if ./scripts/package.sh check >/dev/null 2>&1; then
  ok "Developer ID certificate and notarytool profile"
else
  fail "signing is not set up (./scripts/package.sh check)"
fi

echo "GitHub and the tap"
if gh auth status >/dev/null 2>&1; then ok "gh is signed in"; else fail "gh is not signed in (gh auth login)"; fi
if [ -d "$tap/.git" ]; then
  if [ -n "$(git -C "$tap" status --porcelain)" ]; then
    fail "$tap has uncommitted changes"
  else
    git -C "$tap" fetch -q origin 2>/dev/null || true
    if [ "$(git -C "$tap" rev-parse HEAD)" = "$(git -C "$tap" rev-parse '@{u}' 2>/dev/null)" ]; then
      ok "$tap is clean and up to date"
    else
      fail "$tap is not up to date with origin (git -C $tap pull --ff-only)"
    fi
  fi
else
  fail "no tap clone at $tap (set TAP_DIR)"
fi

if [ "$problems" -gt 0 ]; then
  echo "$problems problem(s); fix them before releasing."
  exit 1
fi
echo "Ready to release $tag."
[ "$mode" = publish ] || exit 0

version=${tag#v}
dmg=build/InkStudio-$version.dmg

echo "Building, signing and notarising"
rm -rf build/InkStudio.app "$dmg"
./scripts/package.sh app
./scripts/package.sh dmg

echo "Publishing the GitHub release"
gh release create "$tag" "$dmg" --repo inkcheck/inkstudio --verify-tag \
  --title "InkStudio $version" --generate-notes

echo "Updating the cask"
sha=$(shasum -a 256 "$dmg" | cut -d' ' -f1)
mkdir -p "$tap/Casks"
sed -e "s/@VERSION@/$version/" -e "s/@SHA256@/$sha/" \
  -e '/^# The Homebrew cask, written/,/^# no quarantine/d' \
  packaging/inkstudio.rb > "$tap/Casks/inkstudio.rb"
if command -v brew >/dev/null; then
  brew style "$tap/Casks/inkstudio.rb"
fi
git -C "$tap" add Casks/inkstudio.rb
git -C "$tap" commit -q -m "inkstudio v$version"
git -C "$tap" push -q origin HEAD
echo "Released InkStudio $version: brew install --cask inkcheck/tap/inkstudio"
