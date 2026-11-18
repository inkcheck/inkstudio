// The CodeMirror editor: one view, one state per open file, so each file
// keeps its own undo history and selection.

import { Compartment, EditorState, RangeSetBuilder, type Extension } from "@codemirror/state";
import {
  EditorView,
  Decoration,
  ViewPlugin,
  keymap,
  lineNumbers,
  highlightActiveLine,
  highlightActiveLineGutter,
  drawSelection,
  dropCursor,
  rectangularSelection,
  highlightSpecialChars,
  type DecorationSet,
  type ViewUpdate,
} from "@codemirror/view";
import { defaultKeymap, history, historyKeymap, indentWithTab } from "@codemirror/commands";
import {
  HighlightStyle,
  LanguageDescription,
  StreamLanguage,
  bracketMatching,
  indentOnInput,
  syntaxHighlighting,
} from "@codemirror/language";
import { markdown, markdownLanguage } from "@codemirror/lang-markdown";
import { languages } from "@codemirror/language-data";
import { toml } from "@codemirror/legacy-modes/mode/toml";
import { tags as t } from "@lezer/highlight";

const highlight = HighlightStyle.define([
  { tag: t.heading, color: "var(--syn-heading)", fontWeight: "600" },
  { tag: [t.strong], fontWeight: "600" },
  { tag: [t.emphasis], fontStyle: "italic" },
  { tag: t.strikethrough, textDecoration: "line-through" },
  { tag: [t.link, t.url], color: "var(--syn-link)" },
  { tag: [t.monospace], color: "var(--syn-code)" },
  { tag: [t.processingInstruction, t.contentSeparator, t.meta], color: "var(--syn-meta)" },
  { tag: [t.keyword, t.operator], color: "var(--syn-keyword)" },
  { tag: [t.string, t.special(t.string)], color: "var(--syn-string)" },
  { tag: [t.number, t.bool, t.atom], color: "var(--syn-number)" },
  { tag: [t.comment, t.quote], color: "var(--syn-comment)", fontStyle: "italic" },
  { tag: [t.propertyName, t.attributeName, t.definition(t.variableName)], color: "var(--syn-property)" },
  { tag: [t.typeName, t.className, t.tagName], color: "var(--syn-type)" },
]);

const theme = EditorView.theme({
  "&": { height: "100%", backgroundColor: "var(--editor-bg)", color: "var(--text)" },
  ".cm-scroller": { fontFamily: "var(--mono)", fontSize: "13.5px", lineHeight: "1.6" },
  ".cm-content": { padding: "12px 0", caretColor: "var(--accent)", maxWidth: "88ch" },
  ".cm-gutters": { backgroundColor: "var(--editor-bg)", color: "var(--text-faint)", border: "none" },
  ".cm-activeLine": { backgroundColor: "var(--editor-active)" },
  ".cm-activeLineGutter": { backgroundColor: "transparent", color: "var(--text-muted)" },
  "&.cm-focused .cm-cursor": { borderLeftColor: "var(--accent)" },
  "&.cm-focused .cm-selectionBackground, .cm-selectionBackground, ::selection": {
    backgroundColor: "var(--selection) !important",
  },
  ".cm-ink-frontmatter": { backgroundColor: "var(--ink-frontmatter)" },
  ".cm-ink-marker": { color: "var(--accent)", fontWeight: "600" },
  ".cm-ink-notes": { color: "var(--text-muted)", backgroundColor: "var(--ink-notes)" },
});

// inkpressMarkers marks what inkpress gives meaning to: the frontmatter, the
// ||| column breaks, ::: boxes and the !-- line that starts the notes. Lines
// inside code fences are left alone, as inkpress leaves them.
const inkpressMarkers = ViewPlugin.fromClass(
  class {
    decorations: DecorationSet;
    constructor(view: EditorView) {
      this.decorations = build(view);
    }
    update(u: ViewUpdate) {
      if (u.docChanged || u.viewportChanged) this.decorations = build(u.view);
    }
  },
  { decorations: (v) => v.decorations },
);

const frontmatterLine = Decoration.line({ class: "cm-ink-frontmatter" });
const markerLine = Decoration.line({ class: "cm-ink-marker" });
const notesLine = Decoration.line({ class: "cm-ink-notes" });

function build(view: EditorView): DecorationSet {
  const b = new RangeSetBuilder<Decoration>();
  const doc = view.state.doc;
  let fm: string | null = null; // the open frontmatter fence
  let fence = "";
  let notes = false;
  for (let i = 1; i <= doc.lines; i++) {
    const line = doc.line(i);
    const text = line.text.trim();
    if (i === 1 && (line.text === "---" || line.text === "+++")) {
      fm = line.text;
      b.add(line.from, line.from, frontmatterLine);
      continue;
    }
    if (fm) {
      b.add(line.from, line.from, frontmatterLine);
      if (line.text.trimEnd() === fm) fm = null;
      continue;
    }
    if (notes) {
      b.add(line.from, line.from, notesLine);
      continue;
    }
    if (fence) {
      if (text.startsWith(fence) && text.replaceAll(fence[0], "") === "") fence = "";
      continue;
    }
    const open = /^(`{3,}|~{3,})/.exec(text);
    if (open) {
      fence = open[1];
      continue;
    }
    if (text === "!--") {
      notes = true;
      b.add(line.from, line.from, markerLine);
    } else if (text === "|||" || text.startsWith(":::")) {
      b.add(line.from, line.from, markerLine);
    }
  }
  return b.finish();
}

function languageFor(path: string): Extension {
  if (path.endsWith(".md")) return markdown({ base: markdownLanguage, codeLanguages: languages });
  if (path.endsWith(".toml")) return StreamLanguage.define(toml);
  return [];
}

export interface EditorHooks {
  onChange(path: string, text: string): void;
  onSave(): void;
}

export class Editor {
  private view: EditorView;
  private states = new Map<string, EditorState>();
  private path: string | null = null;
  private hooks: EditorHooks;
  private language = new Compartment();

  constructor(parent: HTMLElement, hooks: EditorHooks) {
    this.hooks = hooks;
    this.view = new EditorView({ parent, state: EditorState.create({ doc: "" }) });
    this.view.contentDOM.setAttribute("aria-label", "Slide source");
  }

  private extensions(path: string): Extension[] {
    const ext: Extension[] = [
      lineNumbers(),
      highlightActiveLineGutter(),
      highlightSpecialChars(),
      history(),
      drawSelection(),
      dropCursor(),
      rectangularSelection(),
      indentOnInput(),
      bracketMatching(),
      highlightActiveLine(),
      EditorView.lineWrapping,
      syntaxHighlighting(highlight),
      theme,
      this.language.of(languageFor(path)),
      keymap.of([
        { key: "Mod-s", preventDefault: true, run: () => (this.hooks.onSave(), true) },
        indentWithTab,
        ...defaultKeymap,
        ...historyKeymap,
      ]),
      EditorView.updateListener.of((u) => {
        if (u.docChanged) this.hooks.onChange(path, u.state.doc.toString());
      }),
    ];
    if (path.endsWith(".md")) ext.push(inkpressMarkers);
    return ext;
  }

  // Template CSS and HTML get their highlighting once it has loaded.
  private loadLanguage(path: string) {
    if (path.endsWith(".md") || path.endsWith(".toml")) return;
    const desc = LanguageDescription.matchFilename(languages, path);
    desc?.load().then((support) => {
      const effects = this.language.reconfigure(support);
      if (this.path === path) {
        this.view.dispatch({ effects });
      } else {
        const st = this.states.get(path);
        if (st) this.states.set(path, st.update({ effects }).state);
      }
    });
  }

  /** Shows path, creating its state from text the first time. */
  show(path: string, text: string) {
    if (this.path) this.states.set(this.path, this.view.state);
    let state = this.states.get(path);
    const created = !state;
    if (!state) state = EditorState.create({ doc: text, extensions: this.extensions(path) });
    this.path = path;
    if (created) this.loadLanguage(path);
    this.view.setState(state);
    this.view.focus();
  }

  /**
   * Replaces a file's text, for a change made elsewhere: on disk, or by the
   * inspector. Only the part that differs changes, so the cursor stays put
   * and the change is one step to undo.
   */
  replace(path: string, text: string) {
    if (path !== this.path) {
      this.states.delete(path);
      return;
    }
    const old = this.view.state.doc.toString();
    if (old === text) return;
    let start = 0;
    while (start < old.length && start < text.length && old[start] === text[start]) start++;
    let endOld = old.length, endNew = text.length;
    while (endOld > start && endNew > start && old[endOld - 1] === text[endNew - 1]) endOld--, endNew--;
    this.view.dispatch({ changes: { from: start, to: endOld, insert: text.slice(start, endNew) } });
  }

  forget(path: string) {
    this.states.delete(path);
  }

  reset() {
    this.states.clear();
    this.path = null;
    this.view.setState(EditorState.create({ doc: "" }));
  }
}
