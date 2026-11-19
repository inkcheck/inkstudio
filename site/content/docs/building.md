---
title: Building InkStudio
weight: 9
---

InkStudio is a [Wails](https://wails.io) v3 app: Go for the backend, and
the system's web view for the interface, written in TypeScript with
[CodeMirror](https://codemirror.net). It uses inkpress as a Go library to
read and render presentations.

## Build and run

You need Go, Node.js and pnpm.

```sh
git clone https://github.com/inkcheck/inkstudio
cd inkstudio
make run                       # build and run; opens the last presentation
make run PACK=~/Talks/review   # open one
make test
```

`make web` serves the same interface to a browser, on inkpress's sample
deck, for working on it with the browser's developer tools.

## Make the app

```sh
make app       # build/InkStudio.app, for Apple silicon and Intel, for this Mac
make install   # sign, notarise, and copy it to /Applications
make dmg       # a signed, notarised disk image, to share
```

Signing needs a Developer ID Application certificate in the keychain, and
notarising a `notarytool` keychain profile; `make release-check` checks
both.

## How it fits together

| Folder | |
|---|---|
| `main.go` | the app, its window and menus |
| `internal/studio/` | the backend: files, rendering, exports, and the HTTP API the interface uses |
| `frontend/` | the interface |
| `platform/pptx/` | a PowerPoint writer, for any Go program |
| `platform/slidedoc/` | the model of an editable slide that the writer takes |
| `platform/webkit/` | an offscreen WebKit view, for pictures and PDFs on macOS |

The interface talks to the backend over HTTP on the app's own address. The
editable PowerPoint export loads a slide in WebKit, reads where the browser
put each heading, paragraph, box and picture, and writes them as PowerPoint
shapes; see the [README](https://github.com/inkcheck/inkstudio#readme) for
more.
