---
title: Getting started
weight: 1
---

## Install

InkStudio runs on macOS 13 Ventura or later, on Apple silicon and Intel
Macs. Install it with [Homebrew](https://brew.sh):

```sh
brew install --cask inkcheck/tap/inkstudio
```

Or download `InkStudio-<version>.dmg` from the
[latest release](https://github.com/inkcheck/inkstudio/releases/latest),
open it, and drag InkStudio to Applications. The app is signed and
notarised by Apple, so it opens without a warning.

Homebrew also installs [inkline](https://inkcheck.github.io/inkline/),
which draws the diagrams in slides. If you install InkStudio from the disk
image, install inkline too to see diagrams:

```sh
brew install --cask inkcheck/tap/inkline
```

InkStudio needs nothing else: it draws, prints and exports slides with the
WebKit that is part of macOS.

## Open a presentation

A presentation is a folder with a `slides/` folder of Markdown files, one per
slide, as [inkpress](https://inkcheck.github.io/inkpress/docs/packs/) makes
them. To open one, do one of these:

- Click **Open…** (⌘O) and choose the folder.
- Drop the folder on InkStudio's icon in the Dock.
- Run `open -a InkStudio path/to/folder` in Terminal.

InkStudio opens the last presentation again when it starts.

## Start a presentation

Click **New** (⇧⌘N) and choose an empty folder. InkStudio writes inkpress's
sample deck there: thirteen slides that show each layout, with the default
template in `.templates/default`. Change the slides, or delete them and
write your own.

## Next

- [The editor](../editor): write slides with a live preview.
- [Presenting](../presenting): the presenter view and the slideshow.
- [Exporting](../exporting): PDF and PowerPoint.
