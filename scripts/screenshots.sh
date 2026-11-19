#!/bin/sh
# Takes the site's screenshots: the UI on inkpress's sample deck, as WebKit
# draws it (macOS only), in light and dark, into site/static/screenshots/.
#
#   scripts/screenshots.sh [name...]   default: all of them
set -eu
cd "$(dirname "$0")/.."

out=site/static/screenshots
addr=127.0.0.1:1417
mkdir -p "$out" bin
make --no-print-directory frontend bin/webkit-snap >/dev/null
go build -o bin/inkstudio-web ./cmd/inkstudio-web
bin/inkstudio-web -addr "$addr" -sample >/dev/null 2>&1 &
server=$!
trap 'kill $server 2>/dev/null' EXIT
sleep 1

# Each shot: a name, and the JavaScript that sets the view up.
# The sample lives in a temporary folder; show a usual one instead.
setup="document.getElementById('deck-dir').textContent = '~/Talks/inkpress';"
editor="$setup document.querySelector('[data-view=editor]').click(); document.querySelector('[data-path=\"slides/06-boxes.md\"]').click();"
shot() {
  name=$1 js=$2
  if [ -n "${only:-}" ] && ! printf ' %s ' "$only" | grep -q " $name "; then return; fi
  for mode in light dark; do
    SNAP_APPEARANCE=$mode bin/webkit-snap "http://$addr/" "$out/$name-$mode.png" 1680 1050 4 "$js" >/dev/null
    # Retina snapshots are 3360 wide; 2000 is sharp on the site and smaller.
    sips -Z 2000 "$out/$name-$mode.png" >/dev/null
    echo "$out/$name-$mode.png"
  done
}
only="$*"

shot editor "$editor"
shot presenter "$setup document.querySelector('[data-view=presenter]').click();
  setTimeout(() => document.querySelector('#strip-frame iframe').contentDocument.querySelectorAll('.studio-thumb')[1].click(), 1200);"
shot themes "$editor document.querySelector('[data-path=\"slides/04-section.md\"]').click();
  setTimeout(() => document.querySelector('#inspector .tpl-edit').click(), 800);"
shot export "$editor setTimeout(() => document.getElementById('export-btn').click(), 600);"
shot slideshow "$editor setTimeout(() => document.querySelector('[data-action=present]').click(), 600);"
