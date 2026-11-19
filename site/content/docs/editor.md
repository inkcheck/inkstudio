---
title: The editor
weight: 2
---

{{< shot name="editor" caption="The editor: files on the left, the slide's Markdown, its preview, and the inspector." >}}

The editor view (⌘1) has four parts, from left to right.

## Files

The sidebar lists `settings.toml`, the pack's `README.md`, and the slides in
`slides/`, in the order they run. The number beside a slide is its place in
the deck. A slide whose frontmatter has `skip: true` is struck through: it
is not in the presentation. A dot marks a file with edits that are not saved
yet.

## The Markdown editor

The editor highlights Markdown, and marks what inkpress gives meaning to:

- the **frontmatter** between `---` lines, on a tinted background;
- `|||` lines, which split a slide into **columns**;
- `:::` lines, which open and close **boxes**;
- the `!--` line, after which the **speaker notes** start, on a warm
  background.

For what these do, see
[Slides](https://inkcheck.github.io/inkpress/docs/slides/) and
[Boxes](https://inkcheck.github.io/inkpress/docs/boxes/) in the inkpress
documentation.

## The live preview

The preview shows the slide you are editing, and redraws as you type,
before you save. Move between slides with ‹ and ›, or ⌥PageUp and
⌥PageDown. ⌘P hides or shows the preview, and you can drag the line beside
it to resize it.

If a slide has a mistake, such as a layout the template does not have, the
error shows under the preview, and the last good preview stays. Warnings,
such as an image that is not found, show there too. The footer shows the
page size and its shape, such as **1280 × 720 · 16:9**.

## Saving

Edits save on their own about a second after you stop typing, and ⌘S saves
at once. Opening another presentation or quitting saves first. When another
editor or `git` changes a file, InkStudio reloads it; if you have edits to
the same file, yours are kept and saved over the change.

## Diagrams

A code block of D2 with an inkline header becomes a diagram, drawn by
[inkline](https://inkcheck.github.io/inkline/). InkStudio keeps diagrams
that did not change between previews, so typing in a slide with diagrams
stays fast.
