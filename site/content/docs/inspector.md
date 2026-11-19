---
title: The inspector
weight: 4
---

The inspector, on the right in both views, changes the slide and the
presentation without editing their text by hand. ⌥⌘I hides or shows it.

Every change goes into the file as an edit, as if you typed it: it changes
the one line, keeps your comments, and saves like any other edit. ⌘Z in the
file undoes it.

## Slide

The layout, theme and template of the slide you are on. Each list has a
**Default** or **Deck** choice, which takes the key out of the slide's
frontmatter, so the slide follows the deck.

## Presentation

- **Title**, **Author** and **Date**, which templates show in footers and
  on the title slide.
- The **Template** and **Theme** of the whole deck.
- The **Page size**: 16:9, 16:10, 4:3, square, A4 or US Letter, in either
  orientation, or a custom shape, such as `21:9`.

Paper sizes set the slide's width to the paper's width at 96 pixels to the
inch (1123 pixels for A4 landscape), so the PDF prints at the paper's size.
inkpress templates size things to the slide, so a deck changes shape
without other changes.

## Templates

Every template in the presentation's `.templates/` folder, with its themes
as colour swatches. Click a theme to use it for the presentation. **Edit
themes…** opens the [theme editor](../themes), **Export…** saves a template
as an `.inktheme` file, and **Import…** adds one.
