---
title: Sharing
weight: 7
---

A presentation is a folder of files. To send one to someone with InkStudio,
share it as one `.inkdeck` file.

## Share a presentation

Choose **Export ▾ ▸ Share as .inkdeck…**, or **File ▸ Share as .inkdeck…**.
InkStudio saves your edits, and writes one file with `settings.toml`, the
slides, the images in `assets/`, and the templates in `.templates/`, fonts
included, so it looks the same on another Mac.

It leaves out hidden files, such as a `.git` folder, and the PDF,
PowerPoint and HTML files that exports leave in the presentation's folder.

## Open a shared presentation

Double-click the `.inkdeck` file, or choose **File ▸ Open Shared
Presentation…**. InkStudio asks where to put the presentation, next to the
file to start with, unpacks a copy there as a normal folder, and opens it.
The `.inkdeck` file does not change. If a folder of that name is there
already, the copy is named `My Talk 2`, and so on.

An `.inkdeck` file is a zip file, so `unzip` opens one too. InkStudio checks
it before it unpacks it: it refuses files that would land outside the
folder, links, more than 5000 files or 1 GB, and files without a `slides/`
folder.
