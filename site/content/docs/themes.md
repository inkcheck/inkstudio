---
title: Themes
weight: 5
---

A template pack has layouts, which place things on a slide, and themes,
which set its colours. A theme is a CSS file of custom properties, such as
`--bg`, `--fg` and `--accent`, that the template's styles use. See
[Templates](https://inkcheck.github.io/inkpress/docs/templates/) in the
inkpress documentation for how they are made.

## The theme editor

{{< shot name="themes" caption="The theme editor, on the light theme of the default template." >}}

Click **Edit themes…** on a template in the inspector. The theme editor
lists the theme's custom properties under the comments that group them in
its CSS, with a colour picker for each colour. A change saves at once and
shows in the preview; ⌘Z in the theme's CSS file undoes it.

Above the properties, the editor shows how well the text stands out from
its background: the contrast of the text, the muted text, and the section
slides' text, graded **AAA**, **AA**, **AA large** or **Fails** by the
WCAG rules.

To change a copy and keep the original, type a name and click
**Duplicate**. **Edit CSS** opens the theme's file in the editor, for what
the editor does not show.

If the slide you are on uses another theme, the editor says so, and offers
to use the theme you are editing for the presentation.

## .inktheme files

An `.inktheme` file is a template pack in a zip file, to share it. In the
inspector, **Export…** on a template saves one, and **Import…** (also
**File ▸ Import Theme…**) adds one to the presentation. Double-clicking an
`.inktheme` file imports it into the presentation that is open.

The template is named after the file: `Ocean.inktheme` becomes the `Ocean`
template. InkStudio asks before it replaces a template of the same name.

To make one by hand, zip a template's folder, with `template.toml` at the
top:

```sh
cd .templates/ocean && zip -r ../../Ocean.inktheme .
```

InkStudio checks a theme before it adds it: it refuses files that would
land outside the template's folder, links, more than 2000 files or 200 MB,
and zips without a `template.toml`, and it checks that the template loads.
