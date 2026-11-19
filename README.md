# InkStudio

InkStudio is a Mac app for [inkpress](https://github.com/inkcheck/inkpress)
presentations: folders of Markdown slides, one file per slide. It edits them
with a live preview, presents them, changes their layouts and themes, and
exports them to PDF and to PowerPoint, as editable slides or as pictures.

**Documentation and downloads: https://inkcheck.github.io/inkstudio/**

```sh
brew install --cask inkcheck/tap/inkstudio
```

## Building

Building needs Go, Node and pnpm.

```sh
make run                       # build and run; reopens the last presentation
make run PACK=~/Talks/review   # open one
make test
```

InkStudio is a [Wails](https://wails.io) v3 app: Go for the backend, which
uses inkpress's packages (`github.com/inkcheck/inkpress` v0.3.0 or later),
and the system webview for the interface. To work on both at once, point it
at a local inkpress: `go work init . ../inkpress`.

Run with `INKSTUDIO_DEBUG=1` to log every request the backend serves;
JavaScript errors in the app are always logged to the terminal.

`make web` serves the same interface to a normal browser, on inkpress's
sample deck or `PACK=…`, and on macOS `make snap` saves a snapshot of it as
WebKit draws it. Check interface changes in WebKit: the app runs in it, and
it differs from Chrome (CSS `zoom`, for one, which is why the thumbnails
use transforms).

## The site

`site/` is the documentation site, a Hugo site with the Hextra theme,
styled like inkpress's. A push to `main` that changes it publishes it to
GitHub Pages.

```sh
make serve          # http://localhost:1418/inkstudio/
make screenshots    # retake the screenshots, light and dark (macOS)
```

The screenshots are of the interface on inkpress's sample deck, taken in
WebKit by `scripts/screenshots.sh`; retake and commit them after changing
the interface.

## Packaging and releasing

```sh
make install          # build, sign, notarise and copy InkStudio.app to /Applications
make dmg              # build/InkStudio-<version>.dmg, notarised, to share
make release-check    # check everything a release needs
make release          # publish the tagged HEAD: GitHub release and Homebrew cask
```

Released versions install with Homebrew:

```sh
brew install --cask inkcheck/tap/inkstudio
```

`make release` builds and notarises the DMG from the tagged commit,
attaches it to a GitHub release, and writes `Casks/inkstudio.rb` (from
`packaging/inkstudio.rb`) to a clone of the tap at `../homebrew-tap`, or
`TAP_DIR`, and pushes it. The cask installs the notarised app as it is,
and inkline with it, for diagrams.

`make app` builds `build/InkStudio.app`, one binary for Apple silicon and
Intel, ad-hoc signed for this Mac. `make notarize`, `make dmg` and `make
install` sign it with the first "Developer ID Application" certificate in
the keychain, with the hardened runtime, have Apple notarise it with the
`inkcheck-notary` notarytool profile (inkpress's `make notary-setup` makes
it), and staple the ticket. `MACOS_SIGN_IDENTITY` and `MACOS_NOTARY_PROFILE`
override them, as for inkpress. The version comes from the latest `vX.Y.Z`
tag, or 0.1.0.

The app registers `.inkdeck` and `.inktheme` files, so a double-click opens
a shared presentation or imports a theme, and it opens a folder dropped on
its Dock icon.
`make icon` redraws the icon from `tools/make-icon/icon.swift`.

## Exporting with WebKit

The Mac app exports with the WebKit it draws with: the PDF, the
PowerPoint pictures and the editable PowerPoint come from the same engine
as the preview, so they match it, and no browser has to be installed. It
loads the deck into a WebKit view that is never on screen
(`platform/webkit`), prints each slide as a PDF page and joins them, and
takes pictures of slides and their parts. WebKit draws CSS masks as solid
boxes in PDFs, so masked elements, such as the default template's logo,
are printed as pictures of themselves.

`inkpress pdf` prints with Chrome, so its PDFs and InkStudio's can differ
in small ways. The browser build (`make web`) and any build without
WebKit export with an installed Chrome, Chromium, Edge, Brave or Vivaldi,
as inkpress does; `studio.Engine` picks the engine. Windows and Linux
can add their system engines (WebView2 and WebKitGTK) behind the same
interface.

## How it works

```
main.go              Wails app, menu, window, last-opened pack
internal/studio/     the backend: pack files, rendering, watching, exports, HTTP API
platform/pptx/       a PowerPoint writer, for any program: text boxes, shapes and pictures, and notes
platform/slidedoc/   the slide model it writes: what the editable export reads from the browser
platform/webkit/     an offscreen WebKit view: pictures and PDFs of pages, on macOS
frontend/            Vite + TypeScript + CodeMirror 6
```

The frontend talks to the backend over plain HTTP on the Wails asset server,
and the backend pushes file changes as Wails events:

| Route | |
|---|---|
| `GET /api/project` | the open pack and its files |
| `POST /api/open`, `POST /api/new` | ask for a folder, then open it |
| `GET`/`PUT /api/file?path=` | read or save a text file of the pack |
| `POST /api/render` | render with unsaved text, `{"overlay": {path: text}}`; returns each slide's frontmatter, notes, layout and theme |
| `POST /api/export/{pdf,pptx,pptx-pictures}` | ask where to save, then export the saved pack; slide images send `export:progress` events |
| `POST /api/theme/props`, `POST /api/edit/theme` | a theme's custom properties, and setting them in its CSS text |
| `POST /api/theme/duplicate` | copy a theme to a new name |
| `GET /api/templates` | the template packs, their layouts, themes and theme colours |
| `POST /api/settings/parse` | settings text to the fields the inspector shows |
| `POST /api/edit/settings`, `POST /api/edit/frontmatter` | apply `{key, value}` changes to text, keeping the rest |
| `POST /api/theme/import`, `POST /api/theme/export` | ask for a file, then import or export an `.inktheme` |
| `POST /api/reveal` | show a file in the Finder or Explorer |
| `GET /preview/<n>.html` | a rendered deck |
| `GET /@fs/<path>` | a file of the pack: images, fonts, CSS |

inkpress writes `file:` URLs, which a page in the webview cannot load, so
the preview rewrites them to `/@fs/`, and only files inside the pack are
served. WebKit won't navigate a frame to the app's own `wails://` scheme,
so the app fetches the rendered page and gives it to the frame as
`srcdoc`; the frame shares the app's origin, so its images and fonts load
through the backend. The frame shows one slide, scaled to fit, and the app
calls the page's `studioShow(n)` to move between slides. A new render loads in a
hidden frame and replaces the old one when it is ready. The presenter's
thumbnail strip is the same page in strip mode: every slide, zoomed to the
strip's width, posting clicks and keys to the app.
