// The inspector: the current slide's template, layout and theme, the deck's
// settings and page size, and the template packs, with import and export.
// It never writes files itself: every change goes back to main.ts as edits
// to settings.toml or a slide's frontmatter, through the editor's buffers.

import type { Change, DeckSettings, SlideInfo, TemplateInfo, ThemeProp } from "./api";

export interface InspectorState {
  settings: DeckSettings | null;
  templates: TemplateInfo[];
  /** The file being edited, and its slide when it is in the deck. */
  active: string | null;
  slide: SlideInfo | null;
  /** The size the last render used. */
  width: number;
  height: number;
}

export interface InspectorHooks {
  editSettings(changes: Change[]): void;
  editSlide(path: string, changes: Change[]): void;
  importTheme(): void;
  exportTheme(name: string): void;
  /** Loads a theme's custom properties for the theme editor. */
  loadTheme(template: string, theme: string): Promise<ThemeProp[]>;
  setThemeProp(template: string, theme: string, name: string, value: string): Promise<ThemeProp[]>;
  duplicateTheme(template: string, theme: string, name: string): Promise<boolean>;
  openThemeFile(template: string, theme: string): void;
}

interface ThemeEdit {
  template: string;
  theme: string;
  props: ThemeProp[] | null;
  error?: string;
}

interface Preset {
  id: string;
  label: string;
  aspect: string;
  width: number;
}

// Widths are CSS pixels at 96 per inch, so paper sizes print at true size.
const PRESETS: Preset[] = [
  { id: "16:9", label: "16:9 widescreen", aspect: "16:9", width: 1280 },
  { id: "16:10", label: "16:10", aspect: "16:10", width: 1280 },
  { id: "4:3", label: "4:3 standard", aspect: "4:3", width: 1024 },
  { id: "1:1", label: "1:1 square", aspect: "1:1", width: 1080 },
  { id: "a4-landscape", label: "A4 landscape", aspect: "297:210", width: 1123 },
  { id: "a4-portrait", label: "A4 portrait", aspect: "210:297", width: 794 },
  { id: "letter-landscape", label: "US Letter landscape", aspect: "11:8.5", width: 1056 },
  { id: "letter-portrait", label: "US Letter portrait", aspect: "8.5:11", width: 816 },
];

function ratio(aspect: string): number | null {
  const m = /^\s*([\d.]+)\s*:\s*([\d.]+)\s*$/.exec(aspect);
  if (!m) return null;
  const r = Number(m[1]) / Number(m[2]);
  return Number.isFinite(r) && r > 0 ? r : null;
}

function presetFor(width: number, height: number): Preset | undefined {
  const r = width / height;
  return PRESETS.find((p) => Math.abs((ratio(p.aspect) ?? 0) - r) < 0.002);
}

// A tiny element builder: h("div.class", {attrs}, ...children).
type Child = Node | string | null | undefined | false;
function h<K extends keyof HTMLElementTagNameMap>(
  spec: K | `${K}.${string}`,
  attrs: Record<string, string | boolean | ((e: Event) => void)> = {},
  ...children: Child[]
): HTMLElementTagNameMap[K] {
  const [tag, ...classes] = spec.split(".");
  const el = document.createElement(tag as K);
  if (classes.length) el.className = classes.join(" ");
  for (const [k, v] of Object.entries(attrs)) {
    if (typeof v === "function") el.addEventListener(k, v);
    else if (v === true) el.setAttribute(k, "");
    else if (v !== false) el.setAttribute(k, v);
  }
  for (const c of children) if (c) el.append(c);
  return el;
}

function select(label: string, value: string, options: [string, string][], onChange: (v: string) => void, disabled = false) {
  const id = `insp-${label.toLowerCase().replace(/\W+/g, "-")}-${Math.random().toString(36).slice(2, 7)}`;
  const sel = h("select", { id, change: (e) => onChange((e.target as HTMLSelectElement).value), disabled });
  for (const [v, text] of options) {
    const opt = h("option", { value: v }, text);
    if (v === value) opt.selected = true;
    sel.append(opt);
  }
  return h("div.field", {}, h("label", { for: id }, label), sel);
}

function textField(label: string, value: string, onCommit: (v: string) => void, attrs: Record<string, string> = {}) {
  const id = `insp-${label.toLowerCase().replace(/\W+/g, "-")}`;
  const input = h("input", { id, type: "text", spellcheck: "false", ...attrs });
  input.value = value;
  const commit = () => {
    if (input.value !== value) onCommit(input.value);
  };
  input.addEventListener("change", commit);
  input.addEventListener("keydown", (e) => {
    if (e.key === "Enter") input.blur();
    if (e.key === "Escape") {
      input.value = value;
      input.blur();
    }
  });
  return h("div.field", {}, h("label", { for: id }, label), input);
}

/** The colours worth showing first, when a theme has them. */
const SWATCH_ORDER = ["--bg", "--fg", "--accent", "--muted", "--surface", "--section-bg"];

function swatches(colors: { name: string; value: string }[], max = 6) {
  const sorted = [...colors].sort((a, b) => {
    const ia = SWATCH_ORDER.indexOf(a.name), ib = SWATCH_ORDER.indexOf(b.name);
    return (ia < 0 ? 99 : ia) - (ib < 0 ? 99 : ib);
  });
  const row = h("span.swatches", { "aria-hidden": "true" });
  for (const c of sorted.slice(0, max)) {
    const s = h("span.swatch", { title: `${c.name}: ${c.value}` });
    s.style.background = c.value;
    row.append(s);
  }
  return row;
}

export class Inspector {
  private el: HTMLElement;
  private hooks: InspectorHooks;
  private state: InspectorState | null = null;
  private pending = false;
  /** The user picked Custom… for the page size: show its fields. */
  private customSize = false;
  /** The theme open in the theme editor, which then fills the inspector. */
  private themeEdit: ThemeEdit | null = null;

  constructor(el: HTMLElement, hooks: InspectorHooks) {
    this.el = el;
    this.hooks = hooks;
    // Don't rebuild under someone who is typing in a field.
    el.addEventListener("focusout", () => {
      queueMicrotask(() => {
        if (this.pending && !this.editing()) this.render();
      });
    });
  }

  update(state: InspectorState) {
    this.state = state;
    if (this.el.hidden) return;
    if (this.editing()) this.pending = true;
    else this.render();
  }

  /** Draws the inspector, after it was hidden. */
  refresh() {
    if (this.state && !this.el.hidden) this.render();
  }

  private editing() {
    const a = document.activeElement;
    return !!a && this.el.contains(a) && a instanceof HTMLInputElement;
  }

  private render() {
    this.pending = false;
    const st = this.state;
    if (!st) return;
    const scroll = this.el.scrollTop;
    if (this.themeEdit) this.el.replaceChildren(this.themeEditor(st, this.themeEdit));
    else this.el.replaceChildren(this.slideSection(st), this.deckSection(st), this.templatesSection(st));
    this.el.scrollTop = scroll;
  }

  private slideSection(st: InspectorState) {
    const section = h("section.insp-section", {}, h("h2", {}, "Slide"));
    const path = st.active;
    if (!path || !path.startsWith("slides/")) {
      section.append(h("p.insp-note", {}, "Open a slide to set its layout and theme."));
      return section;
    }
    const s = st.slide;
    const meta = s?.meta ?? {};
    const deckTemplate = st.settings?.template || "default";
    const tplName = s?.template ?? (typeof meta.template === "string" ? meta.template : deckTemplate);
    const tpl = st.templates.find((t) => t.name === tplName);
    const str = (k: string) => (typeof meta[k] === "string" ? (meta[k] as string) : "");

    section.append(
      h("p.insp-sub", {}, s ? `${s.number}. ${s.title || "Untitled"}` : "Not in the presentation (skipped or not rendered)"),
    );
    const set = (key: string) => (v: string) => this.hooks.editSlide(path, [{ key, value: v || null }]);
    section.append(
      select(
        "Layout",
        str("layout"),
        [["", `Default (${tpl?.defaultLayout ?? "content"})`], ...(tpl?.layouts ?? []).map((l): [string, string] => [l, l])],
        set("layout"),
        !tpl,
      ),
      select(
        "Theme",
        str("theme"),
        [["", `Deck theme (${st.settings?.theme || tpl?.defaultTheme || "light"})`], ...(tpl?.themes ?? []).map((t): [string, string] => [t.name, t.name])],
        set("theme"),
        !tpl,
      ),
      select(
        "Template",
        str("template"),
        [["", `Deck template (${deckTemplate})`], ...st.templates.filter((t) => !t.error).map((t): [string, string] => [t.name, t.name])],
        set("template"),
      ),
    );
    return section;
  }

  private deckSection(st: InspectorState) {
    const section = h("section.insp-section", {}, h("h2", {}, "Presentation"));
    const s = st.settings;
    if (!s) {
      section.append(h("p.insp-note", {}, "Loading settings…"));
      return section;
    }
    if (s.error) {
      section.append(h("p.insp-error", {}, `settings.toml: ${s.error}`), h("p.insp-note", {}, "Fix it in the editor to use the inspector."));
      return section;
    }
    const edit = (changes: Change[]) => this.hooks.editSettings(changes);
    const tpl = st.templates.find((t) => t.name === (s.template || "default"));

    section.append(
      textField("Title", s.title, (v) => edit([{ key: "title", value: v || null }]), { placeholder: "The folder's name" }),
      textField("Author", s.author, (v) => edit([{ key: "author", value: v || null }])),
      textField("Date", s.date, (v) => edit([{ key: "date", value: v || null }]), { placeholder: "2026-11-18" }),
      select(
        "Template",
        s.template || "default",
        st.templates.map((t): [string, string] => [t.name, t.error ? `${t.name} (broken)` : t.name]),
        (v) => {
          // A theme the new template lacks would break the render.
          const next = st.templates.find((t) => t.name === v);
          const changes: Change[] = [{ key: "template", value: v }];
          if (s.theme && next && !next.themes.some((th) => th.name === s.theme)) changes.push({ key: "theme", value: null });
          edit(changes);
        },
      ),
      select(
        "Theme",
        s.theme,
        [["", `Template default (${tpl?.defaultTheme ?? "light"})`], ...(tpl?.themes ?? []).map((t): [string, string] => [t.name, t.name])],
        (v) => edit([{ key: "theme", value: v || null }]),
        !tpl,
      ),
    );

    // Page size.
    const current = presetFor(st.width, st.height);
    const custom = this.customSize || !current;
    const presetOptions: [string, string][] = [...PRESETS.map((p): [string, string] => [p.id, p.label]), ["custom", "Custom…"]];
    section.append(
      h("h3", {}, "Page"),
      select("Size", custom ? "custom" : current!.id, presetOptions, (id) => {
        const p = PRESETS.find((x) => x.id === id);
        this.customSize = !p;
        if (!p) {
          // Custom: keep the size, and show its fields.
          this.render();
          return;
        }
        edit([
          { table: "page", key: "height", value: null },
          { table: "page", key: "aspect", value: p.aspect },
          { table: "page", key: "width", value: p.width },
        ]);
      }),
    );
    const row = h("div.field-row");
    if (custom) {
      row.append(
        textField("Aspect", s.aspect || (s.height ? "" : "16:9"), (v) => {
          if (v && !ratio(v)) return;
          edit([{ table: "page", key: "aspect", value: v || null }, { table: "page", key: "height", value: null }]);
        }, { placeholder: "W:H" }),
      );
    }
    row.append(
      textField("Width (px)", String(s.width || 1280), (v) => {
        const n = Math.round(Number(v));
        if (n >= 100 && n <= 10000) edit([{ table: "page", key: "width", value: n }]);
      }, { inputmode: "numeric" }),
    );
    section.append(row, h("p.insp-note.mono", {}, `${Math.round(st.width)} × ${Math.round(st.height)} px`));
    return section;
  }

  private templatesSection(st: InspectorState) {
    const section = h(
      "section.insp-section",
      {},
      h("div.insp-head", {}, h("h2", {}, "Templates"), h("button.btn.small", { click: () => this.hooks.importTheme(), title: "Import an .inktheme file" }, "Import…")),
    );
    if (!st.templates.length) {
      section.append(h("p.insp-note", {}, "This presentation has no templates in .templates/. Import an .inktheme to add one."));
      return section;
    }
    const inUse = st.settings?.template || "default";
    for (const t of st.templates) {
      const card = h("article.tpl-card");
      card.classList.toggle("in-use", t.name === inUse);
      card.append(
        h(
          "div.tpl-head",
          {},
          h("strong", {}, t.name),
          t.name === inUse ? h("span.tag", {}, "In use") : null,
          h("span.spacer"),
          h("button.btn.small", { click: () => this.hooks.exportTheme(t.name), title: `Export ${t.name} as an .inktheme` }, "Export…"),
        ),
      );
      if (t.error) {
        card.append(h("p.insp-error", {}, t.error));
        section.append(card);
        continue;
      }
      if (t.description) card.append(h("p.tpl-desc", {}, t.description));
      const themes = h("div.tpl-themes");
      for (const th of t.themes) {
        const active = t.name === inUse && (st.settings?.theme || t.defaultTheme) === th.name;
        themes.append(
          h(
            "button.tpl-theme",
            {
              title: `Use ${t.name} with the ${th.name} theme`,
              "aria-pressed": String(active),
              click: () =>
                this.hooks.editSettings([
                  { key: "template", value: t.name },
                  { key: "theme", value: th.name },
                ]),
            },
            swatches(th.colors),
            h("span", {}, th.name),
          ),
        );
      }
      const editing = t.name === inUse ? st.settings?.theme || t.defaultTheme : t.defaultTheme;
      card.append(themes, h("p.insp-note", {}, `${t.layouts.length} layouts: ${t.layouts.join(", ")}`));
      if (t.themes.length) {
        const first = t.themes.some((x) => x.name === editing) ? editing : t.themes[0].name;
        card.append(h("button.btn.small.tpl-edit", { click: () => void this.openThemeEditor(t.name, first) }, "Edit themes…"));
      }
      section.append(card);
    }
    return section;
  }

  // ---- theme editor ------------------------------------------------------

  async openThemeEditor(template: string, theme: string) {
    this.themeEdit = { template, theme, props: null };
    this.render();
    this.el.scrollTop = 0;
    await this.reloadTheme();
  }

  /** Reloads the theme being edited, after it changed on disk. */
  async reloadTheme() {
    const te = this.themeEdit;
    if (!te) return;
    try {
      const props = await this.hooks.loadTheme(te.template, te.theme);
      if (this.themeEdit === te) Object.assign(te, { props, error: undefined });
    } catch (e) {
      if (this.themeEdit === te) te.error = e instanceof Error ? e.message : String(e);
    }
    if (this.editing()) this.pending = true;
    else this.render();
  }

  get editingTheme(): { template: string; theme: string } | null {
    return this.themeEdit && { template: this.themeEdit.template, theme: this.themeEdit.theme };
  }

  private closeThemeEditor() {
    this.themeEdit = null;
    this.render();
  }

  private themeEditor(st: InspectorState, te: ThemeEdit) {
    const tpl = st.templates.find((t) => t.name === te.template);
    const section = h("section.insp-section.theme-editor");
    section.append(
      h(
        "div.insp-head",
        {},
        h("button.btn.small", { click: () => this.closeThemeEditor(), "aria-label": "Back to the inspector" }, "‹ Back"),
        h("button.link-btn", { click: () => this.hooks.openThemeFile(te.template, te.theme), title: "Open the theme's CSS in the editor" }, "Edit CSS"),
      ),
      h("h2", {}, `Theme · ${te.template}`),
      select(
        "Theme",
        te.theme,
        (tpl?.themes ?? []).map((t): [string, string] => [t.name, t.name]),
        (v) => void this.openThemeEditor(te.template, v),
      ),
    );

    // Where the preview stands.
    const deckTheme = st.settings?.theme || tpl?.defaultTheme;
    const usedByDeck = (st.settings?.template || "default") === te.template && deckTheme === te.theme;
    const slideTheme = st.slide && st.slide.template === te.template ? st.slide.theme : null;
    if (slideTheme !== te.theme) {
      const note = h(
        "p.insp-note",
        {},
        st.slide ? `Slide ${st.slide.number} uses ${st.slide.template} / ${st.slide.theme}, so the preview won't show these changes. ` : "",
      );
      if (!usedByDeck) {
        note.append(
          h(
            "button.link-btn",
            {
              click: () =>
                this.hooks.editSettings([
                  { key: "template", value: te.template },
                  { key: "theme", value: te.theme },
                ]),
            },
            "Use this theme for the presentation",
          ),
        );
      }
      section.append(note);
    }

    // Duplicate, so you can change a copy and keep the original.
    const dupInput = h("input", { type: "text", placeholder: `${te.theme}-2`, "aria-label": "New theme name", spellcheck: "false" });
    const dup = async () => {
      const name = dupInput.value.trim() || `${te.theme}-2`;
      if (await this.hooks.duplicateTheme(te.template, te.theme, name)) void this.openThemeEditor(te.template, name);
    };
    dupInput.addEventListener("keydown", (e) => e.key === "Enter" && void dup());
    section.append(h("div.dup-row", {}, dupInput, h("button.btn.small", { click: () => void dup() }, "Duplicate")));

    if (te.error) {
      section.append(h("p.insp-error", {}, te.error));
      return section;
    }
    if (!te.props) {
      section.append(h("p.insp-note", {}, "Loading…"));
      return section;
    }
    if (!te.props.length) {
      section.append(h("p.insp-note", {}, "This theme sets no custom properties. Edit its CSS to add some, such as --bg and --fg."));
      return section;
    }

    const contrast = contrastChecks(te.props);
    if (contrast) section.append(contrast);

    let group = "\u0000";
    for (const p of te.props) {
      if (p.group !== group) {
        group = p.group;
        if (group) section.append(h("h3", {}, group));
      }
      section.append(this.propRow(te, p));
    }
    return section;
  }

  private propRow(te: ThemeEdit, p: ThemeProp) {
    const set = (value: string) => {
      if (value.trim() === p.value) return;
      void this.hooks.setThemeProp(te.template, te.theme, p.name, value.trim()).then((props) => {
        if (this.themeEdit === te) te.props = props;
      });
    };
    const row = h("div.prop-row");
    const label = h("label", { title: p.name }, p.name.replace(/^--/, ""));
    const text = h("input", { type: "text", spellcheck: "false", "aria-label": p.name });
    text.value = p.value;
    text.addEventListener("change", () => set(text.value));
    text.addEventListener("keydown", (e) => {
      if (e.key === "Enter") text.blur();
      if (e.key === "Escape") {
        text.value = p.value;
        text.blur();
      }
    });
    const hex = toHex6(p.value);
    if (p.color && hex) {
      const picker = h("input", { type: "color", "aria-label": `${p.name} colour` });
      picker.value = hex;
      let timer = 0;
      picker.addEventListener("input", () => {
        text.value = picker.value;
        clearTimeout(timer);
        timer = window.setTimeout(() => set(picker.value), 120);
      });
      row.append(label, picker, text);
    } else if (p.color) {
      const sw = h("span.swatch.prop-swatch", { title: p.value });
      sw.style.background = p.value;
      row.append(label, sw, text);
    } else {
      row.append(label, h("span"), text);
    }
    return row;
  }
}

// ---- colour helpers --------------------------------------------------------

/** #rgb or #rrggbb as #rrggbb, or null for anything else. */
function toHex6(v: string): string | null {
  const m = /^#([0-9a-f]{3}|[0-9a-f]{6})$/i.exec(v.trim());
  if (!m) return null;
  const x = m[1].length === 3 ? [...m[1]].map((c) => c + c).join("") : m[1];
  return "#" + x.toLowerCase();
}

function luminance(hex: string): number {
  const n = parseInt(hex.slice(1), 16);
  const ch = [(n >> 16) & 255, (n >> 8) & 255, n & 255].map((c) => {
    const s = c / 255;
    return s <= 0.03928 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4;
  });
  return 0.2126 * ch[0] + 0.7152 * ch[1] + 0.0722 * ch[2];
}

/** WCAG contrast of the text colours on their backgrounds, where both are hex. */
function contrastChecks(props: ThemeProp[]) {
  const get = (n: string) => toHex6(props.find((p) => p.name === n)?.value ?? "");
  const pairs: [string, string, string][] = [
    ["Text", "--fg", "--bg"],
    ["Muted text", "--muted", "--bg"],
    ["Section text", "--section-fg", "--section-bg"],
  ];
  const box = h("div.contrast");
  for (const [label, fg, bg] of pairs) {
    const a = get(fg), b = get(bg);
    if (!a || !b) continue;
    const [l1, l2] = [luminance(a), luminance(b)].sort((x, y) => y - x);
    const ratio = (l1 + 0.05) / (l2 + 0.05);
    const grade = ratio >= 7 ? "AAA" : ratio >= 4.5 ? "AA" : ratio >= 3 ? "AA large" : "Fails";
    const chip = h("span.contrast-chip", { title: `${fg} on ${bg}` });
    if (ratio < 3) chip.classList.add("bad");
    else if (ratio < 4.5) chip.classList.add("meh");
    const sample = h("span.contrast-sample", {}, "Aa");
    sample.style.color = a;
    sample.style.background = b;
    chip.append(sample, `${label} ${ratio.toFixed(1)}:1 · ${grade}`);
    box.append(chip);
  }
  return box.childElementCount ? box : null;
}
