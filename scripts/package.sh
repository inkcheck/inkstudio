#!/bin/sh
# Builds, signs, notarises and installs InkStudio.app, with Apple's own tools
# and keychain credentials, as inkpress's scripts/notarize.sh does: the
# Developer ID Application certificate for codesign, and a notarytool
# keychain profile (inkpress's `make notary-setup` creates it).
#
#   scripts/package.sh app        build/InkStudio.app: universal, ad-hoc signed
#   scripts/package.sh notarize   the app, signed with Developer ID, notarised, stapled
#   scripts/package.sh dmg        build/InkStudio-<version>.dmg, notarised, stapled
#   scripts/package.sh install    the notarised app, copied to /Applications
#   scripts/package.sh check      what signing and notarising need
#
# MACOS_SIGN_IDENTITY overrides the certificate (default: the first Developer
# ID Application identity); MACOS_NOTARY_PROFILE the profile (default:
# inkcheck-notary); INSTALL_DIR the install folder (default: /Applications).
set -eu
cd "$(dirname "$0")/.."

cmd=${1:-app}
app=build/InkStudio.app
profile=${MACOS_NOTARY_PROFILE:-inkcheck-notary}
version=$(git describe --tags --match 'v[0-9]*' 2>/dev/null | sed 's/^v//' || true)
# Before any tag, a development version.
version=${version:-0.0.0}
build=$(git rev-list --count HEAD 2>/dev/null || echo 1)
# The bundle version must be numbers and dots.
short=$(printf '%s' "$version" | sed 's/[^0-9.].*//')

identity() {
  printf '%s' "${MACOS_SIGN_IDENTITY:-$(security find-identity -v -p codesigning |
    sed -n 's/.*"\(Developer ID Application: [^"]*\)".*/\1/p' | head -1)}"
}

say() { printf '\033[1m%s\033[0m\n' "$*"; }

build_app() {
  say "Building InkStudio $version ($build), for Apple silicon and Intel"
  make --no-print-directory frontend >/dev/null
  export CGO_ENABLED=1 GOOS=darwin MACOSX_DEPLOYMENT_TARGET=13.0
  export CGO_CFLAGS="-mmacosx-version-min=13.0" CGO_LDFLAGS="-mmacosx-version-min=13.0"
  flags="-s -w -X main.version=$version"
  mkdir -p build
  GOARCH=arm64 CC="clang -arch arm64" go build -tags production -trimpath -ldflags "$flags" -o build/inkstudio-arm64 .
  GOARCH=amd64 CC="clang -arch x86_64" go build -tags production -trimpath -ldflags "$flags" -o build/inkstudio-amd64 .

  rm -rf "$app"
  mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources"
  lipo -create -output "$app/Contents/MacOS/InkStudio" build/inkstudio-arm64 build/inkstudio-amd64
  rm build/inkstudio-arm64 build/inkstudio-amd64
  sed -e "s/@VERSION@/$short/" -e "s/@BUILD@/$build/" packaging/Info.plist > "$app/Contents/Info.plist"
  plutil -lint -s "$app/Contents/Info.plist"
  cp packaging/inkstudio.icns "$app/Contents/Resources/inkstudio.icns"
  printf 'APPL????' > "$app/Contents/PkgInfo"
  # Ad hoc, so it runs here; `notarize` signs it properly.
  codesign --force --sign - "$app"
  say "Built $app"
}

sign() {
  id=$(identity)
  if [ -z "$id" ]; then
    echo "No Developer ID Application certificate in the keychain." >&2
    exit 1
  fi
  say "Signing $1 as $id"
  codesign --force --options runtime --timestamp --sign "$id" "$1"
  codesign --verify --strict --verbose=2 "$1"
}

# notarise <file>: submits a zip, dmg or pkg and waits for Apple's verdict.
notarise() {
  say "Submitting $(basename "$1") to Apple (keychain profile $profile); this takes a few minutes"
  tmp=$(mktemp -d)
  xcrun notarytool submit "$1" --keychain-profile "$profile" \
    --wait --timeout 30m --output-format json > "$tmp/result.json" || true
  status=$(plutil -extract status raw -o - "$tmp/result.json" 2>/dev/null || echo unknown)
  id=$(plutil -extract id raw -o - "$tmp/result.json" 2>/dev/null || echo "")
  if [ "$status" != Accepted ]; then
    echo "Apple returned status $status" >&2
    cat "$tmp/result.json" >&2
    [ -n "$id" ] && xcrun notarytool log "$id" --keychain-profile "$profile" >&2 || true
    rm -rf "$tmp"
    exit 1
  fi
  rm -rf "$tmp"
  say "Accepted ($id)"
}

notarise_app() {
  [ -d "$app" ] || build_app
  # A fresh build is ad-hoc signed; one that is already notarised and
  # stapled needs nothing more.
  if xcrun stapler validate -q "$app" 2>/dev/null && spctl --assess --type execute "$app" 2>/dev/null; then
    say "$app is already notarised"
    return
  fi
  sign "$app"
  zip=build/InkStudio-notarise.zip
  ditto -c -k --keepParent "$app" "$zip"
  notarise "$zip"
  rm -f "$zip"
  xcrun stapler staple "$app"
  xcrun stapler validate "$app"
  spctl --assess --type execute --verbose=2 "$app"
}

make_dmg() {
  notarise_app
  dmg=build/InkStudio-$version.dmg
  say "Making $dmg"
  stage=$(mktemp -d)
  ditto "$app" "$stage/InkStudio.app"
  ln -s /Applications "$stage/Applications"
  rm -f "$dmg"
  hdiutil create -volname "InkStudio $version" -srcfolder "$stage" -fs HFS+ -format UDZO -ov "$dmg" >/dev/null
  rm -rf "$stage"
  sign "$dmg"
  notarise "$dmg"
  xcrun stapler staple "$dmg"
  spctl --assess --type open --context context:primary-signature --verbose=2 "$dmg"
  say "Made $dmg"
}

install_app() {
  notarise_app
  dest=${INSTALL_DIR:-/Applications}
  if pgrep -xq InkStudio || pgrep -xq inkstudio; then
    say "Quitting the running InkStudio"
    osascript -e 'tell application id "io.github.inkcheck.inkstudio" to quit' 2>/dev/null || true
    sleep 1
  fi
  say "Installing to $dest/InkStudio.app"
  # The name is case-insensitive on disk: this also removes an old
  # inkstudio.app.
  rm -rf "$dest/InkStudio.app"
  ditto "$app" "$dest/InkStudio.app"
  # Tell Launch Services about the app's file types (.inktheme).
  /System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister -f "$dest/InkStudio.app" || true
  # Unregister the build copy, so the Finder opens .inktheme files with the
  # installed app.
  /System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister -u "$PWD/$app" 2>/dev/null || true
  say "Installed. Open it from Launchpad, or: open -a InkStudio [folder]"
}

check() {
  problems=0
  id=$(identity)
  if [ -n "$id" ]; then echo "  ✓ certificate: $id"; else echo "  ✗ no Developer ID Application certificate"; problems=1; fi
  if xcrun notarytool history --keychain-profile "$profile" >/dev/null 2>&1; then
    echo "  ✓ notarytool profile $profile works"
  else
    echo "  ✗ notarytool profile $profile is missing or rejected (run make notary-setup in ../inkpress)"; problems=1
  fi
  echo "  version $version, build $build"
  exit $problems
}

case $cmd in
  app) build_app ;;
  notarize | notarise) notarise_app ;;
  dmg) make_dmg ;;
  install) install_app ;;
  check) check ;;
  *) echo "usage: $0 app|notarize|dmg|install|check" >&2; exit 2 ;;
esac
