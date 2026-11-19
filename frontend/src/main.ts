import { Events } from "@wailsio/runtime";
import { api, type Change, type DeckSettings, type ExportFormat, type FileEntry, type ProjectInfo, type RenderResult, type SlideInfo, type TemplateInfo } from "./api";
import { Editor } from "./editor";
import { Inspector } from "./inspector";
import { Preview } from "./preview";

// Edits are saved on their own a moment after typing stops, so switching
// folders or quitting never loses work. ⌘S saves at once.
const AUTOSAVE_MS = 1200;
const RENDER_MS = 250;

type View = "editor" | "presenter";

interface Buffer {
  text: string;
  /** The text on disk, as far as we know. */
  saved: string;
}

const $ = <T extends HTMLElement = HTMLElement>(id: string) => document.getElementById(id) as T;

const els = {
  workspace: $("workspace"),
  presenter: $("presenter"),
  welcome: $("welcome"),
  welcomeError: $("welcome-error"),
  sidebar: $("sidebar"),
  title: $("deck-title"),
  dir: $("deck-dir"),
  fileName: $("file-name"),
  fileState: $("file-state"),
  previewPane: $("preview-pane"),
  previewMessage: $("preview-message"),
  presentMessage: $("present-message"),
  slidePos: $("slide-pos"),
  pageSize: $("page-size"),
  problems: $("problems"),
  status: $("status"),
  statusAction: $<HTMLButtonElement>("status-action"),
  renderStatus: $("render-status"),
  splitter: $("splitter"),
  notesSplitter: $("notes-splitter"),
  frontmatter: $("frontmatter"),
  notes: $("notes"),
  notesBody: $("notes-body"),
  exportBtn: $<HTMLButtonElement>("export-btn"),
  exportMenu: $("export-menu"),
  inspector: $("inspector"),
};

let project: ProjectInfo | null = null;
/** The file being edited; in the presenter, the current slide's file. */
let active: string | null = null;
/** The file the editor shows, which lags active while presenting. */
let editorPath: string | null = null;
const buffers = new Map<string, Buffer>();
let lastRender: RenderResult | null = null;
let slideNumber = 1;
let view: View = safeGet("inkstudio.view") === "presenter" ? "presenter" : "editor";
let templates: TemplateInfo[] = [];
let settings: DeckSettings | null = null;
let inspectorOpen = safeGet("inkstudio.inspector") !== "0";

const SETTINGS = "settings.toml";

const preview = new Preview($("preview-frame"));
const presentPreview = new Preview($("present-frame"));
const strip = new Preview($("strip-frame"), "strip");
const showPreview = new Preview($("show-frame"));
let presenting = false;

const editor = new Editor($("editor"), {
  onChange(path, text) {
    const b = buffers.get(path);
    if (!b || b.text === text) return;
    b.text = text;
    updateDirty();
    scheduleRender();
    scheduleSave();
  },
  onSave: () => void saveAll(),
});

const inspector = new Inspector(els.inspector, {
  editSettings: (changes) => void applyEdit(SETTINGS, "settings", changes),
  editSlide: (path, changes) => void applyEdit(path, "frontmatter", changes),
  importTheme: () => void importTheme(),
  exportTheme: (name) => void exportTheme(name),
  loadTheme: async (template, theme) => {
    const b = await ensureBuffer(themePath(template, theme));
    return api.themeProps(b.text);
  },
  setThemeProp: async (template, theme, name, value) => {
    const path = themePath(template, theme);
    try {
      const b = await ensureBuffer(path);
      const text = await api.editTheme(b.text, [{ name, value }]);
      b.text = text;
      editor.replace(path, text);
      // Templates are read from disk when the deck renders, so save now.
      await api.write(path, text);
      b.saved = text;
      updateDirty();
      scheduleRender(0);
      return api.themeProps(text);
    } catch (e) {
      showStatus(`Could not change ${name}: ${errorText(e)}`, undefined, 8000);
      return api.themeProps(buffers.get(path)?.text ?? "");
    }
  },
  duplicateTheme: async (template, theme, name) => {
    try {
      await api.duplicateTheme(template, theme, name);
      await refreshTemplates();
      showStatus(`Made the ${name} theme from ${theme}`);
      return true;
    } catch (e) {
      showStatus(errorText(e), undefined, 8000);
      return false;
    }
  },
  openThemeFile: (template, theme) => {
    setView("editor");
    void openFile(themePath(template, theme));
  },
});

function themePath(template: string, theme: string) {
  return `.templates/${template}/themes/${theme}.css`;
}

// ---- project -------------------------------------------------------------

async function setProject(info: ProjectInfo | null) {
  const same = project && info && project.dir === info.dir;
  project = info;
  if (!same) {
    buffers.clear();
    editor.reset();
    for (const p of [preview, presentPreview, strip]) p.clear();
    active = editorPath = null;
    lastRender = null;
    slideNumber = 1;
    templates = [];
    settings = null;
  }
  els.title.textContent = info?.title ?? "InkStudio";
  els.dir.textContent = info ? shortDir(info.dir) : "";
  els.dir.title = info?.dir ?? "";
  els.exportBtn.disabled = !info;
  applyView();
  if (!info) return;

  renderSidebar();
  void refreshTemplates();
  if (!active || !info.files.some((f) => f.path === active)) {
    const first = info.files.find((f) => f.group === "slides") ?? info.files[0];
    if (first) await openFile(first.path);
  }
  await renderNow();
}

function shortDir(dir: string) {
  return dir.replace(/^\/Users\/[^/]+/, "~");
}

async function refreshProject() {
  try {
    const info = await api.project();
    if (info) await setProject(info);
  } catch (e) {
    showStatus(String(e));
  }
}

async function prompt(kind: "open" | "new" | "deck") {
  els.welcomeError.hidden = true;
  closeExportMenu();
  await saveAll();
  try {
    // The backend emits project:opened, which does the rest.
    await (kind === "open" ? api.open() : kind === "new" ? api.create() : api.openDeck());
  } catch (e) {
    const msg = errorText(e);
    if (project) showStatus(msg);
    else {
      els.welcomeError.textContent = msg;
      els.welcomeError.hidden = false;
    }
  }
}

function errorText(e: unknown) {
  return e instanceof Error ? e.message : String(e);
}

// ---- views ---------------------------------------------------------------

function setView(v: View) {
  if (v === view) return;
  view = v;
  safeSet("inkstudio.view", v);
  applyView();
  if (v === "editor" && active && active !== editorPath) void openFile(active);
}

function applyView() {
  const open = !!project;
  els.welcome.hidden = open;
  els.inspector.hidden = !open || !inspectorOpen;
  els.workspace.hidden = !open || view !== "editor";
  els.presenter.hidden = !open || view !== "presenter";
  for (const tab of document.querySelectorAll<HTMLButtonElement>("[data-view]")) {
    tab.setAttribute("aria-selected", String(tab.dataset.view === view));
    tab.disabled = !open;
  }
  for (const btn of document.querySelectorAll<HTMLElement>("[data-for]")) {
    btn.hidden = !open || btn.dataset.for !== view;
  }
  document.querySelector<HTMLButtonElement>('[data-action="toggle-inspector"]')!.disabled = !open;
  document.querySelector<HTMLButtonElement>('[data-action="present"]')!.disabled = !open;
  loadPreviews();
  if (view === "presenter") {
    // Take the keyboard from the hidden editor.
    (document.activeElement as HTMLElement | null)?.blur();
    showPresenterSlide();
  }
}

/** Loads the newest render into the frames that can be seen. */
function loadPreviews() {
  if (!lastRender) return;
  const frames = presenting
    ? [showPreview]
    : view === "editor" ? (els.previewPane.hidden ? [] : [preview]) : [presentPreview, strip];
  for (const p of frames) {
    if (p.loaded === lastRender.url) p.refit();
    else p.load(lastRender.url);
    p.show(slideNumber);
  }
}

// ---- sidebar -------------------------------------------------------------

function renderSidebar() {
  if (!project) return;
  const groups: [string, FileEntry[]][] = [
    ["Pack", project.files.filter((f) => f.group === "pack")],
    ["Slides", project.files.filter((f) => f.group === "slides")],
  ];
  const sourceNumbers = new Map(lastRender?.slides.map((s) => [s.source, s]) ?? []);
  els.sidebar.replaceChildren(
    ...groups
      .filter(([, files]) => files.length)
      .map(([label, files]) => {
        const section = document.createElement("section");
        const h = document.createElement("h2");
        h.textContent = label;
        const list = document.createElement("ul");
        for (const f of files) {
          const li = document.createElement("li");
          const btn = document.createElement("button");
          btn.className = "file";
          btn.dataset.path = f.path;
          if (f.path === active) btn.setAttribute("aria-current", "true");
          const slide = sourceNumbers.get(f.path);
          const num = document.createElement("span");
          num.className = "file-num mono";
          num.textContent = f.group === "slides" ? (slide ? String(slide.number) : "–") : "";
          const name = document.createElement("span");
          name.className = "file-name";
          name.textContent = f.name;
          if (slide?.title) btn.title = slide.title;
          if (f.group === "slides" && !slide && lastRender) {
            btn.classList.add("skipped");
            btn.title = "Skipped: not in the presentation";
          }
          const dot = document.createElement("span");
          dot.className = "dirty-dot";
          dot.setAttribute("aria-hidden", "true");
          btn.append(num, name, dot);
          btn.addEventListener("click", () => void openFile(f.path));
          li.append(btn);
          list.append(li);
        }
        section.append(h, list);
        return section;
      }),
  );
  updateDirty();
}

function updateDirty() {
  for (const btn of els.sidebar.querySelectorAll<HTMLButtonElement>("button.file")) {
    const b = buffers.get(btn.dataset.path!);
    btn.classList.toggle("dirty", !!b && b.text !== b.saved);
  }
  const b = editorPath ? buffers.get(editorPath) : undefined;
  els.fileState.textContent = b && b.text !== b.saved ? "Edited" : "";
}

function markActiveInSidebar() {
  for (const btn of els.sidebar.querySelectorAll<HTMLButtonElement>("button.file")) {
    if (btn.dataset.path === active) btn.setAttribute("aria-current", "true");
    else btn.removeAttribute("aria-current");
  }
}

// ---- files ---------------------------------------------------------------

async function openFile(path: string) {
  try {
    let b = buffers.get(path);
    if (!b) {
      const text = await api.read(path);
      b = { text, saved: text };
      buffers.set(path, b);
    }
    active = editorPath = path;
    editor.show(path, b.text);
    els.fileName.textContent = path;
    markActiveInSidebar();
    updateDirty();
    syncSlideToFile();
    updateInspector();
  } catch (e) {
    showStatus(errorText(e));
  }
}

let saveTimer = 0;
function scheduleSave() {
  clearTimeout(saveTimer);
  saveTimer = window.setTimeout(() => void saveAll(), AUTOSAVE_MS);
}

async function saveAll() {
  clearTimeout(saveTimer);
  for (const [path, b] of buffers) {
    if (b.text === b.saved) continue;
    const text = b.text;
    try {
      await api.write(path, text);
      b.saved = text;
      showStatus(`Saved ${path}`);
    } catch (e) {
      showStatus(`Could not save ${path}: ${errorText(e)}`);
    }
  }
  updateDirty();
}

/** Files changed on disk: by us, another editor, or git. */
async function onDiskChange(paths: string[]) {
  let listChanged = false;
  for (const path of paths) {
    const b = buffers.get(path);
    if (!project?.files.some((f) => f.path === path)) listChanged = true;
    if (!b) continue;
    let text: string;
    try {
      text = await api.read(path);
    } catch {
      // Deleted or renamed.
      buffers.delete(path);
      editor.forget(path);
      listChanged = true;
      continue;
    }
    if (text === b.saved) continue; // our own save
    if (b.text === b.saved) {
      b.text = b.saved = text;
      editor.replace(path, text);
      showStatus(`Reloaded ${path}`);
    } else {
      // Edited here and there: keep ours, and save over theirs.
      b.saved = text;
      showStatus(`${path} changed on disk; your edits will be saved over it`);
      scheduleSave();
    }
  }
  if (paths.some((p) => p.startsWith(".templates/"))) {
    void refreshTemplates();
    const te = inspector.editingTheme;
    if (te && paths.includes(themePath(te.template, te.theme))) void inspector.reloadTheme();
  }
  if (listChanged) await refreshProject();
  scheduleRender(0);
}

// ---- rendering -----------------------------------------------------------

let renderTimer = 0;
let rendering = false;
let renderAgain = false;

function scheduleRender(delay = RENDER_MS) {
  clearTimeout(renderTimer);
  renderTimer = window.setTimeout(() => void renderNow(), delay);
}

async function renderNow() {
  if (!project) return;
  if (rendering) {
    renderAgain = true;
    return;
  }
  rendering = true;
  const overlay: Record<string, string> = {};
  for (const [path, b] of buffers) if (b.text !== b.saved) overlay[path] = b.text;
  const started = performance.now();
  try {
    const res = await api.render(overlay);
    const hadSlides = lastRender?.slides.map((s) => s.source).join() ?? "";
    lastRender = res;
    loadPreviews();
    els.pageSize.textContent = `${Math.round(res.width)} × ${Math.round(res.height)} · ${aspect(res.width, res.height)}`;
    showProblems(res.warnings, null);
    els.renderStatus.textContent = `${res.slides.length} slides · rendered in ${Math.round(performance.now() - started)} ms`;
    if (res.slides.map((s) => s.source).join() !== hadSlides) renderSidebar();
    if (view === "presenter") showPresenterSlide();
    else syncSlideToFile();
    void refreshSettings();
  } catch (e) {
    // Keep the last good preview, and say what is wrong.
    showProblems([], errorText(e));
    els.renderStatus.textContent = "Render failed";
  } finally {
    rendering = false;
    if (renderAgain) {
      renderAgain = false;
      scheduleRender(0);
    }
  }
}

function aspect(w: number, h: number) {
  const known: [string, number][] = [
    ["16:9", 16 / 9], ["16:10", 16 / 10], ["4:3", 4 / 3], ["A4 landscape", 297 / 210],
    ["A4 portrait", 210 / 297], ["1:1", 1], ["9:16", 9 / 16],
  ];
  const r = w / h;
  const hit = known.find(([, k]) => Math.abs(k - r) < 0.005);
  return hit ? hit[0] : `${r.toFixed(3)}:1`;
}

function showProblems(warnings: string[], error: string | null) {
  const items = [...(error ? [{ kind: "error", text: error }] : []), ...warnings.map((w) => ({ kind: "warning", text: w }))];
  els.problems.hidden = items.length === 0;
  els.problems.replaceChildren(
    ...items.map(({ kind, text }) => {
      const p = document.createElement("p");
      p.className = kind;
      const tag = document.createElement("strong");
      tag.textContent = kind === "error" ? "Error" : "Warning";
      p.append(tag, " ", text);
      return p;
    }),
  );
  if (error && view === "presenter") showStatus(`Render failed: ${error}`);
}

// ---- slide position ------------------------------------------------------

/** Points the editor's preview at the active file's slide. */
function syncSlideToFile() {
  if (!lastRender || !active) return;
  const s = lastRender.slides.find((s) => s.source === active);
  if (s) {
    els.previewMessage.hidden = true;
    showSlide(s.number);
  } else if (active.startsWith("slides/")) {
    els.previewMessage.textContent = "This slide is skipped, so it is not in the presentation.";
    els.previewMessage.hidden = false;
  } else {
    els.previewMessage.hidden = true;
    showSlide(slideNumber);
  }
}

function showSlide(n: number) {
  const total = lastRender?.slides.length ?? 0;
  slideNumber = Math.min(Math.max(1, n), Math.max(total, 1));
  for (const p of [preview, presentPreview, strip, showPreview]) p.show(slideNumber);
  els.slidePos.textContent = total ? `${slideNumber} / ${total}` : "–";
}

/** Moves to slide n, from 1, in either view. */
function goTo(n: number) {
  const slides = lastRender?.slides;
  if (!slides?.length) return;
  n = Math.min(Math.max(1, n), slides.length);
  const s = slides[n - 1];
  if (view === "editor") {
    void openFile(s.source);
    return;
  }
  active = s.source;
  markActiveInSidebar();
  showSlide(n);
  showPresenterSlide();
  updateInspector();
}

function step(delta: number) {
  goTo(slideNumber + delta);
}

// ---- presenter -----------------------------------------------------------

function showPresenterSlide() {
  const slides = lastRender?.slides ?? [];
  // A skipped slide or settings.toml in the editor: stay on the slide we had.
  const byFile = slides.find((s) => s.source === active);
  if (byFile && byFile.number !== slideNumber) showSlide(byFile.number);
  const s = slides[slideNumber - 1];
  els.presentMessage.hidden = !!s;
  els.presentMessage.textContent = s ? "" : "No slides to show.";
  renderFrontmatter(s, slides.length);
  renderNotes(s);
}

function renderFrontmatter(s: SlideInfo | undefined, total: number) {
  if (!s) {
    els.frontmatter.replaceChildren();
    return;
  }
  const meta = s.meta ?? {};
  const chip = (key: string, value: string, opts: { implied?: boolean; wide?: boolean } = {}) => {
    const el = document.createElement("span");
    el.className = "chip" + (opts.implied ? " implied" : "") + (opts.wide ? " wide" : "");
    if (opts.implied) el.title = "Not in the frontmatter: the default";
    const k = document.createElement("span");
    k.className = "chip-key";
    k.textContent = key;
    const v = document.createElement("span");
    v.className = "chip-value";
    v.textContent = value;
    el.append(k, v);
    return el;
  };

  const title = document.createElement("div");
  title.className = "fm-title";
  const pos = document.createElement("span");
  pos.className = "mono muted";
  pos.textContent = `${s.number} / ${total}`;
  const name = document.createElement("strong");
  name.textContent = s.title || "Untitled";
  const file = document.createElement("span");
  file.className = "mono muted";
  file.textContent = s.source;
  title.append(pos, name, file);

  const chips = document.createElement("div");
  chips.className = "fm-chips";
  for (const key of ["template", "layout", "theme"] as const) {
    chips.append(chip(key, s[key], { implied: !(key in meta) }));
  }
  for (const [key, value] of Object.entries(meta)) {
    if (key === "template" || key === "layout" || key === "theme") continue;
    const text = typeof value === "string" ? value : JSON.stringify(value);
    chips.append(chip(key, text, { wide: text.length > 40 }));
  }
  els.frontmatter.replaceChildren(title, chips);
}

function renderNotes(s: SlideInfo | undefined) {
  const notes = s?.notes.trim() ?? "";
  if (!notes) {
    const p = document.createElement("p");
    p.className = "muted";
    p.textContent = "No notes for this slide. Add them after a !-- line.";
    els.notesBody.replaceChildren(p);
    return;
  }
  // Paragraphs and list items, as plain text: notes are for reading aloud.
  els.notesBody.replaceChildren(
    ...notes.split(/\n\s*\n/).map((para) => {
      const lines = para.split("\n");
      if (lines.every((l) => /^\s*([-*+]|\d+[.)])\s+/.test(l))) {
        const ul = document.createElement("ul");
        for (const l of lines) {
          const li = document.createElement("li");
          li.textContent = l.replace(/^\s*([-*+]|\d+[.)])\s+/, "");
          ul.append(li);
        }
        return ul;
      }
      const p = document.createElement("p");
      p.textContent = para.replace(/\s*\n\s*/g, " ");
      return p;
    }),
  );
  els.notesBody.scrollTop = 0;
}

function toggleNotes(force?: boolean) {
  const show = force ?? els.notes.hidden;
  els.notes.hidden = !show;
  els.notesSplitter.hidden = !show;
  safeSet("inkstudio.notes", show ? "1" : "0");
  document.querySelector('[data-action="toggle-notes"]')?.setAttribute("aria-pressed", String(show));
  presentPreview.refit();
}

// ---- slideshow -----------------------------------------------------------

const show = $("show");
const showHud = $("show-hud");
let hudTimer = 0;

/** Presents full screen, from the current slide or the first. */
async function startShow(from: "current" | "start" = "current") {
  if (!project || presenting || !lastRender?.slides.length) return;
  presenting = true;
  closeExportMenu();
  (document.activeElement as HTMLElement | null)?.blur();
  show.hidden = false;
  show.focus();
  if (from === "start") slideNumber = 1;
  loadPreviews();
  showSlide(slideNumber);
  flashHud();
  try {
    await api.fullscreen(true);
  } catch {
    // The browser build: use the page's own full screen.
    void document.documentElement.requestFullscreen?.().catch(() => {});
  }
  // The window resizes as it goes full screen.
  setTimeout(() => showPreview.refit(), 700);
}

function endShow() {
  if (!presenting) return;
  presenting = false;
  show.hidden = true;
  showPreview.clear();
  void api.fullscreen(false).catch(() => {
    if (document.fullscreenElement) void document.exitFullscreen();
  });
  // Land on the slide the show ended on.
  const s = lastRender?.slides[slideNumber - 1];
  if (s) {
    active = s.source;
    markActiveInSidebar();
    if (view === "editor") void openFile(s.source);
    else showPresenterSlide();
  }
  loadPreviews();
}

/** Shows the slide number for a moment. */
function flashHud() {
  const total = lastRender?.slides.length ?? 0;
  showHud.textContent = `${slideNumber} / ${total}`;
  showHud.classList.add("visible");
  show.classList.remove("idle");
  clearTimeout(hudTimer);
  hudTimer = window.setTimeout(() => {
    showHud.classList.remove("visible");
    show.classList.add("idle"); // hides the pointer
  }, 1600);
}

function showStep(delta: number) {
  const total = lastRender?.slides.length ?? 0;
  const n = Math.min(Math.max(1, slideNumber + delta), total);
  if (n !== slideNumber) showSlide(n);
  flashHud();
}

function showKey(e: KeyboardEvent) {
  const total = lastRender?.slides.length ?? 1;
  switch (e.key) {
    case "Escape": endShow(); break;
    case "ArrowRight": case "ArrowDown": case "PageDown": case " ": case "Enter": case "n": showStep(1); break;
    case "ArrowLeft": case "ArrowUp": case "PageUp": case "Backspace": case "p": showStep(-1); break;
    case "Home": showSlide(1); flashHud(); break;
    case "End": showSlide(total); flashHud(); break;
    default: return;
  }
  e.preventDefault();
  e.stopPropagation();
}

show.addEventListener("click", (e) => {
  // The left third goes back, the rest forward, as in Keynote.
  showStep(e.clientX < window.innerWidth / 3 ? -1 : 1);
});
show.addEventListener("pointermove", () => flashHud());

function presenterKey(key: string) {
  switch (key) {
    case "ArrowRight": case "ArrowDown": case "PageDown": case " ": return step(1), true;
    case "ArrowLeft": case "ArrowUp": case "PageUp": return step(-1), true;
    case "Home": return goTo(1), true;
    case "End": return goTo(lastRender?.slides.length ?? 1), true;
  }
  return false;
}

// ---- inspector -----------------------------------------------------------

function updateInspector() {
  if (!project) return;
  inspector.update({
    settings,
    templates,
    active,
    slide: lastRender?.slides.find((s) => s.source === active) ?? null,
    width: lastRender?.width ?? 1280,
    height: lastRender?.height ?? 720,
  });
}

async function refreshTemplates() {
  try {
    templates = await api.templates();
  } catch (e) {
    showStatus(errorText(e));
  }
  updateInspector();
}

/** Reads the settings the inspector shows, unsaved edits included. */
async function refreshSettings() {
  try {
    const b = buffers.get(SETTINGS);
    let text = b?.text;
    if (text === undefined) text = await api.read(SETTINGS).catch(() => "");
    settings = await api.parseSettings(text);
  } catch (e) {
    showStatus(errorText(e));
  }
  updateInspector();
}

async function ensureBuffer(path: string): Promise<Buffer> {
  let b = buffers.get(path);
  if (!b) {
    let text = "";
    try {
      text = await api.read(path);
    } catch (e) {
      // A pack may have no settings.toml yet; the edit creates it.
      if (path !== SETTINGS) throw e;
    }
    b = { text, saved: path === SETTINGS && !text ? "\u0000" : text };
    buffers.set(path, b);
  }
  return b;
}

/** Applies an inspector change to a file's buffer, as if typed. */
async function applyEdit(path: string, kind: "settings" | "frontmatter", changes: Change[]) {
  try {
    const b = await ensureBuffer(path);
    const text = await api.edit(kind, b.text, changes);
    if (text === b.text) return;
    b.text = text;
    editor.replace(path, text);
    updateDirty();
    scheduleSave();
    scheduleRender(0);
  } catch (e) {
    showStatus(`Could not change ${path}: ${errorText(e)}`, undefined, 8000);
  }
}

function toggleInspector(force?: boolean) {
  inspectorOpen = force ?? !inspectorOpen;
  safeSet("inkstudio.inspector", inspectorOpen ? "1" : "0");
  els.inspector.hidden = !project || !inspectorOpen;
  document.querySelector('[data-action="toggle-inspector"]')?.setAttribute("aria-pressed", String(inspectorOpen));
  inspector.refresh();
  for (const p of [preview, presentPreview, strip, showPreview]) p.refit();
}

async function importTheme() {
  if (!project) return;
  try {
    const name = await api.importTheme();
    if (!name) return;
    await refreshTemplates();
    showStatus(`Imported the ${name} template`, {
      label: "Use it",
      run: () => void applyEdit(SETTINGS, "settings", [{ key: "template", value: name }, { key: "theme", value: null }]),
    });
  } catch (e) {
    showStatus(`Import failed: ${errorText(e)}`, undefined, 12000);
  }
}

async function exportTheme(name: string) {
  try {
    const path = await api.exportTheme(name);
    if (!path) return;
    showStatus(`Exported ${name} to ${shortDir(path)}`, {
      label: "Show in Finder",
      run: () => void api.reveal(path).catch((e) => showStatus(errorText(e))),
    });
  } catch (e) {
    showStatus(`Export failed: ${errorText(e)}`, undefined, 12000);
  }
}

// ---- export --------------------------------------------------------------

let exporting = false;
const FORMAT_NAMES: Record<ExportFormat, string> = { pdf: "PDF", pptx: "PowerPoint" };

async function exportAs(format: ExportFormat) {
  closeExportMenu();
  if (!project || exporting) return;
  exporting = true;
  els.exportBtn.disabled = true;
  els.exportBtn.textContent = "Exporting…";
  try {
    await saveAll();
    const res = await api.exportAs(format);
    if (res.path) {
      const extra = res.warnings.length ? ` (${res.warnings.length} warnings)` : "";
      showStatus(`Exported ${res.slides} slides to ${shortDir(res.path)}${extra}`, {
        label: "Show in Finder",
        run: () => void api.reveal(res.path).catch((e) => showStatus(errorText(e))),
      });
    } else {
      els.status.textContent = "";
    }
  } catch (e) {
    showStatus(`${FORMAT_NAMES[format]} export failed: ${errorText(e)}`, undefined, 15000);
  } finally {
    exporting = false;
    els.exportBtn.disabled = !project;
    els.exportBtn.textContent = "Export ▾";
  }
}

async function shareDeck() {
  closeExportMenu();
  if (!project) return;
  try {
    await saveAll();
    const res = await api.shareDeck();
    if (!res.path) return;
    showStatus(`Shared ${res.files} files as ${shortDir(res.path)}`, {
      label: "Show in Finder",
      run: () => void api.reveal(res.path).catch((e) => showStatus(errorText(e))),
    });
  } catch (e) {
    showStatus(`Share failed: ${errorText(e)}`, undefined, 12000);
  }
}

function openExportMenu() {
  if (!project || exporting) return;
  els.exportMenu.hidden = false;
  els.exportBtn.setAttribute("aria-expanded", "true");
  els.exportMenu.querySelector<HTMLButtonElement>("[data-export]")?.focus();
}

function closeExportMenu() {
  if (els.exportMenu.hidden) return;
  els.exportMenu.hidden = true;
  els.exportBtn.setAttribute("aria-expanded", "false");
}

els.exportMenu.addEventListener("keydown", (e) => {
  const items = [...els.exportMenu.querySelectorAll<HTMLButtonElement>("[role=menuitem]")];
  const i = items.indexOf(document.activeElement as HTMLButtonElement);
  if (e.key === "ArrowDown") items[(i + 1) % items.length].focus();
  else if (e.key === "ArrowUp") items[(i - 1 + items.length) % items.length].focus();
  else if (e.key === "Escape") {
    closeExportMenu();
    els.exportBtn.focus();
  } else return;
  e.preventDefault();
});

// ---- chrome --------------------------------------------------------------

let statusTimer = 0;
function showStatus(text: string, action?: { label: string; run: () => void }, ms = action ? 12000 : 4000) {
  els.status.textContent = text;
  els.statusAction.hidden = !action;
  els.statusAction.textContent = action?.label ?? "";
  els.statusAction.onclick = action ? () => action.run() : null;
  clearTimeout(statusTimer);
  statusTimer = window.setTimeout(() => {
    els.status.textContent = "";
    els.statusAction.hidden = true;
  }, ms);
}

function togglePreview(force?: boolean) {
  const show = force ?? els.previewPane.hidden;
  els.previewPane.hidden = !show;
  els.splitter.hidden = !show;
  document.querySelector('[data-action="toggle-preview"]')?.setAttribute("aria-pressed", String(show));
  if (show) loadPreviews();
}

/** A splitter that resizes a pane by setting a CSS variable on :root. */
function initSplitter(el: HTMLElement, prop: string, axis: "x" | "y", pane: HTMLElement, size: (e: PointerEvent) => number, min: number, max: () => number, keyStep: Record<string, number>) {
  const root = document.documentElement;
  const saved = Number(safeGet(`inkstudio.${prop}`));
  if (saved) root.style.setProperty(`--${prop}`, `${saved}px`);
  const set = (v: number) => {
    v = Math.min(Math.max(v, min), max());
    root.style.setProperty(`--${prop}`, `${v}px`);
    safeSet(`inkstudio.${prop}`, String(Math.round(v)));
    presentPreview.refit();
  };
  el.addEventListener("pointerdown", (e) => {
    el.setPointerCapture(e.pointerId);
    document.body.classList.add(axis === "x" ? "resizing" : "resizing-y");
    const move = (ev: PointerEvent) => set(size(ev));
    el.addEventListener("pointermove", move);
    el.addEventListener("pointerup", () => {
      el.removeEventListener("pointermove", move);
      document.body.classList.remove("resizing", "resizing-y");
    }, { once: true });
  });
  el.addEventListener("keydown", (e) => {
    if (e.key in keyStep) {
      e.preventDefault();
      const r = pane.getBoundingClientRect();
      set((axis === "x" ? r.width : r.height) + keyStep[e.key]);
    }
  });
}

function safeGet(k: string) {
  try { return localStorage.getItem(k); } catch { return null; }
}
function safeSet(k: string, v: string) {
  try { localStorage.setItem(k, v); } catch { /* storage off */ }
}

document.addEventListener("click", (e) => {
  const item = (e.target as HTMLElement).closest<HTMLElement>("[data-export]");
  if (item) return void exportAs(item.dataset.export as ExportFormat);
  if (!(e.target as HTMLElement).closest(".menu-wrap")) closeExportMenu();
  const target = (e.target as HTMLElement).closest<HTMLElement>("[data-action], [data-view]");
  if (target?.dataset.view) return setView(target.dataset.view as View);
  switch (target?.dataset.action) {
    case "open": return void prompt("open");
    case "new": return void prompt("new");
    case "toggle-preview": return togglePreview();
    case "toggle-notes": return toggleNotes();
    case "present": return void startShow();
    case "share": return void shareDeck();
    case "open-deck": return void prompt("deck");
    case "toggle-inspector": return toggleInspector();
    case "export-menu": return els.exportMenu.hidden ? openExportMenu() : closeExportMenu();
    case "prev": return step(-1);
    case "next": return step(1);
  }
});

document.addEventListener("keydown", (e) => {
  if (presenting) return showKey(e);
  if ((e.metaKey || e.ctrlKey) && e.key === "Enter") {
    e.preventDefault();
    return void startShow(e.shiftKey ? "start" : "current");
  }
  // ⌥/⌘ PageUp and PageDown move between slides from anywhere.
  if ((e.altKey || e.metaKey) && (e.key === "PageDown" || e.key === "PageUp")) {
    e.preventDefault();
    return step(e.key === "PageDown" ? 1 : -1);
  }
  if (view !== "presenter" || e.metaKey || e.ctrlKey || e.altKey) return;
  if ((e.target as HTMLElement).closest("input, textarea, [contenteditable], .splitter-h")) return;
  if (presenterKey(e.key)) e.preventDefault();
});

// The thumbnail strip is a frame; it tells us about clicks and keys.
window.addEventListener("message", (e) => {
  const msg = e.data as { inkstudio?: string; slide?: number; key?: string };
  if (e.origin !== location.origin || !msg?.inkstudio) return;
  if (msg.inkstudio === "select" && msg.slide) goTo(msg.slide);
  if (msg.inkstudio === "key" && msg.key) presenterKey(msg.key);
});

Events.On("project:opened", (ev) => void setProject(ev.data as ProjectInfo));
Events.On("project:changed", (ev) => void onDiskChange((ev.data as { paths: string[] }).paths));
Events.On("menu:save", () => void saveAll());
Events.On("menu:export", (ev) => void exportAs(ev.data as ExportFormat));
Events.On("export:progress", (ev) => {
  const { done, total } = ev.data as { done: number; total: number };
  els.status.textContent = done < total ? `Rendering slide ${done + 1} of ${total}…` : "Writing the file…";
  els.statusAction.hidden = true;
  clearTimeout(statusTimer);
});
Events.On("menu:view", (ev) => project && setView(ev.data === "presenter" ? "presenter" : "editor"));
Events.On("menu:toggle-preview", () => togglePreview());
Events.On("menu:share", () => void shareDeck());
Events.On("menu:open-deck", () => void prompt("deck"));
Events.On("menu:present", (ev) => void startShow(ev.data === "start" ? "start" : "current"));
// Leaving full screen another way, such as the green button, ends the show.
document.addEventListener("fullscreenchange", () => {
  if (!document.fullscreenElement && presenting) endShow();
});
Events.On("menu:toggle-notes", () => view === "presenter" && toggleNotes());
Events.On("menu:toggle-inspector", () => project && toggleInspector());
Events.On("menu:import-theme", () => void importTheme());
Events.On("theme:imported", (ev) => {
  const name = (ev.data as { name: string }).name;
  void refreshTemplates();
  showStatus(`Imported the ${name} template`, {
    label: "Use it",
    run: () => void applyEdit(SETTINGS, "settings", [{ key: "template", value: name }, { key: "theme", value: null }]),
  });
});
window.addEventListener("beforeunload", () => void saveAll());

// Errors go to the backend's log too, where a terminal can see them.
function report(msg: string) {
  void fetch("/api/log", { method: "POST", body: msg }).catch(() => {});
}
window.addEventListener("error", (e) => report(`${e.message} at ${e.filename}:${e.lineno}`));
window.addEventListener("unhandledrejection", (e) => report(`unhandled: ${errorText(e.reason)}`));

initSplitter(els.splitter, "preview-width", "x", els.previewPane, (e) => window.innerWidth - e.clientX, 280, () => window.innerWidth - 420, {
  ArrowLeft: 24, ArrowRight: -24,
});
initSplitter(els.notesSplitter, "notes-height", "y", els.notes, (e) => window.innerHeight - e.clientY - 23, 80, () => window.innerHeight * 0.6, {
  ArrowUp: 24, ArrowDown: -24,
});
toggleNotes(safeGet("inkstudio.notes") !== "0");
toggleInspector(inspectorOpen);
els.exportBtn.disabled = true;
void (async () => {
  try {
    await setProject(await api.project());
  } catch (e) {
    els.welcomeError.textContent = errorText(e);
    els.welcomeError.hidden = false;
  }
})();
