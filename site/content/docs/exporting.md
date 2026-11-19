---
title: Exporting
weight: 6
---

{{< shot name="export" caption="The Export menu." >}}

**Export ▾** in the toolbar, or the **File** menu, saves your edits and
exports the presentation. Each format starts in the presentation's folder,
named after its `output` setting or the folder.

## PDF

**PDF…** (⌘E) prints one page per slide, at the slide's size: 96 pixels to
the inch, so an A4 deck prints on A4. Text stays text, so you can select and
search it, and links work.

## PowerPoint

PowerPoint files open in PowerPoint, Keynote, Google Slides and LibreOffice.
Both kinds carry each slide's notes as its speaker notes, and keep the
slide's size: 16:9 is PowerPoint's widescreen size, and A4 is A4.

**PowerPoint, editable…** (⇧⌘E) makes slides you can change. InkStudio reads
each slide as it is laid out, and rebuilds it from PowerPoint's own parts:

- headings, paragraphs and code become **text boxes**, with their fonts,
  sizes, colours, bold, italics and links;
- lists become **bulleted and numbered lists**;
- boxes, callouts and the lines of tables become **shapes**;
- images, diagrams, logos and gradients become **pictures**;
- numbers that a template adds, such as an agenda's, become text.

**PowerPoint, pictures…** makes each slide one picture, at twice its size,
exactly as it looks in InkStudio, but you cannot change it.

### What to know about editable slides

- Fonts are named, not included in the file. A computer without the
  template's fonts, such as IBM Plex Sans in the default template, shows
  others, and lines can break in other places.
- Each block of a slide is its own text box, so a longer paragraph does not
  push the ones below it down.
- Each line of a code block is its own text box.

## How InkStudio draws exports

InkStudio exports with the WebKit that macOS draws web pages with, the same
engine as its preview, so an export looks like the preview, and nothing else
has to be installed. The `inkpress` command prints PDFs with Chrome, so a
PDF from InkStudio and one from `inkpress pdf` can differ in small ways.
