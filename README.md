# InkStudio

InkStudio is a desktop app for [inkpress](../inkpress) slide packs. It opens a
folder of Markdown slides, edits them with a live preview, and (in later
phases) presents and exports them. It is a [Wails](https://wails.io) v3 app:
Go for the backend, which uses inkpress as a library, and the system webview
for the UI.

```sh
make run                       # build and run; reopens the last pack
make run PACK=~/talks/review   # open a pack
make test
```

Run with `INKSTUDIO_DEBUG=1` to log every request the backend serves;
JavaScript errors in the app are always logged to the terminal.

`make web PACK=…` serves the same UI to a normal browser, and on macOS
`make snap` saves a snapshot of it as WebKit draws it. Check UI changes in
WebKit: the app runs in it, and it differs from Chrome (CSS `zoom`, for one,
draws differently, which is why the thumbnails use transforms).

Building needs Go, Node and pnpm. inkpress is found at `../inkpress` through
a `replace` in go.mod, until it publishes its library packages.

## Installing on a Mac

```sh
make install          # build, sign, notarise and copy InkStudio.app to /Applications
make dmg              # build/InkStudio-<version>.dmg, notarised, to share
make release-check    # check the certificate and notarytool profile
```

`make app` builds `build/InkStudio.app`, one binary for Apple silicon and
Intel, ad-hoc signed for this Mac. `make notarize`, `make dmg` and `make
install` sign it with the first "Developer ID Application" certificate in
the keychain, with the hardened runtime, have Apple notarise it with the
`inkcheck-notary` notarytool profile (inkpress's `make notary-setup` makes
it), and staple the ticket. `MACOS_SIGN_IDENTITY` and `MACOS_NOTARY_PROFILE`
override them, as for inkpress. The version comes from the latest `vX.Y.Z`
tag, or 0.1.0.

The app registers `.inktheme` files, so a double-click imports a theme into
the open presentation, and opens a folder dropped on its Dock icon.
`make icon` redraws the icon from `tools/make-icon/icon.swift`.

## Phase 1: editor view

- **Open** a pack (⌘O) or start a **new** one from inkpress's sample (⇧⌘N).
  InkStudio reopens the last pack on start.
- The **sidebar** lists `settings.toml`, `README.md` and the slides, with
  each slide's number in the deck. A struck-through slide has `skip: true`.
- The **editor** highlights Markdown and marks what inkpress gives meaning to:
  the frontmatter, `|||` columns, `:::` boxes, and the `!--` notes.
- The **preview** renders the deck as you type, before saving, and shows the
  slide in the editor. ‹ › or ⌥PageUp/⌥PageDown move between slides. Errors
  and warnings appear under it, and the last good preview stays up.
- Edits **save themselves** a moment after you stop typing; ⌘S saves at
  once. Changes made on disk by other editors or git are picked up.
- Diagrams need `inkline` on the PATH, as for inkpress. InkStudio adds
  `/opt/homebrew/bin`, `/usr/local/bin` and `~/go/bin` to the PATH, since an
  app started from the Finder gets a minimal one.

## Phase 2: presenter view and PDF

- **Presenter** (⌘2, or the toolbar): thumbnails on the left, the slide on
  the right, its frontmatter above and its notes below. The chips show the
  template, layout and theme the slide gets; a dashed chip is a default, not
  set in the frontmatter. Click a thumbnail, or use the arrow keys, Page
  Up/Down, Space, Home and End. ⇧⌘O hides the notes, and the bar above them
  resizes them. Back in the editor (⌘1), the slide you were on is open.
- **Export PDF** (⌘E) saves your edits, asks where to put the file
  (settings' `output`, or `<pack>.pdf`, by default) and prints it as
  `inkpress pdf` does: with an installed Chrome, Chromium, Edge, Brave or
  Vivaldi, or the browser in `INKPRESS_BROWSER`.
- **Diagrams are cached** between previews, so typing in a slide no longer
  runs inkline for every diagram in the deck. A change to any file besides
  the slides and settings, such as an icon, clears the cache.

## Phase 3: inspector and themes

- The **inspector** (⌥⌘I, or the toolbar) sits on the right in both views.
  - **Slide**: the layout, theme and template of the slide you're on. Each
    has a "Default" or "Deck" choice, which removes the key from the
    frontmatter.
  - **Presentation**: title, author, date, the template and theme, and the
    page size: 16:9, 16:10, 4:3, square, A4 and US Letter in either
    orientation, or a custom W:H. Paper sizes set the width to the paper's
    width at 96 px per inch, so the PDF prints at true size.
  - **Templates**: every template in `.templates/`, with its themes as
    colour swatches. Click a theme to use it. Each template can be exported
    as an `.inktheme`, and **Import…** (also File ▸ Import Theme…) adds one.
- The inspector edits `settings.toml` and the frontmatter as **text**: it
  changes the one line, keeps comments and order, and checks the result
  parses. The change goes into the editor like typing, so ⌘Z undoes it,
  and it saves like any other edit.

### .inktheme files

An `.inktheme` is a zip of one inkpress template pack: `template.toml`,
`style.css`, `themes/`, `layouts/`, and any `partials/`, `assets/` and
`fonts/`. `template.toml` is at the root of the zip, or in one folder at the
root (as macOS's "Compress" makes). The template is named after the file:
`Ocean.inktheme` installs as `.templates/Ocean`.

```sh
cd .templates/mytheme && zip -r ../../mytheme.inktheme .   # by hand
```

Importing refuses paths that leave the template folder, links, more than
2000 files or 200 MB, and zips with no or several `template.toml`. It
unpacks into a hidden folder, checks the template loads, and only then
moves it into place. A template with the same name is replaced only after
you confirm.

## Phase 4: PowerPoint, OpenDocument and the theme editor

- **Export ▾** in the toolbar, or the File menu, exports a **PDF** (⌘E), a
  **PowerPoint** file (⇧⌘E) or an **OpenDocument** presentation (`.odp`,
  for LibreOffice Impress and Keynote).
  - In PowerPoint and OpenDocument files each slide is a **picture**,
    rendered at twice the slide size (2560 × 1440 for 16:9), so they look
    exactly like the PDF. The slide's title is the picture's alt text, and
    its `!--` notes are the speaker notes.
  - The slide size carries over: 16:9 is PowerPoint's own widescreen size,
    and A4 is A4. PowerPoint's 56-inch limit is kept by scaling.
  - The pictures are rendered by the same installed Chromium-based browser
    the PDF uses. The status bar counts the slides as they render.
  - The text in the slides is not editable in PowerPoint. That needs a
    native export, which maps headings, lists and columns to text boxes:
    a project of its own.
- The **theme editor**: **Edit themes…** on a template in the inspector.
  - It lists the theme's CSS custom properties (`--bg`, `--fg`, `--accent`
    …) under the comments that group them, with a colour picker for each
    colour. Changes save and show in the preview as you make them, and ⌘Z
    in the CSS file undoes them.
  - It shows the WCAG contrast of the text on the background, the muted
    text, and the section slides' text, graded AAA, AA, AA large or fails.
  - **Duplicate** makes a copy of the theme to change, leaving the
    original alone. **Edit CSS** opens the file in the editor.
  - If the slide you are on uses another theme, it says so, and offers to
    use the edited theme for the presentation.

## How it works

```
main.go              Wails app, menu, window, last-opened pack
internal/studio/     the backend: pack files, rendering, watching, exports, HTTP API
internal/pptx/       a PowerPoint writer: one picture and its notes per slide
internal/odp/        the same for OpenDocument
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
| `POST /api/export/{pdf,pptx,odp}` | ask where to save, then export the saved pack; slide images send `export:progress` events |
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
