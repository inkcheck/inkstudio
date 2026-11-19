// The backend's HTTP API, served by the Wails asset server (internal/studio).

export interface FileEntry {
  path: string;
  name: string;
  group: "pack" | "slides";
}

export interface ProjectInfo {
  dir: string;
  title: string;
  files: FileEntry[];
}

export interface SlideInfo {
  number: number;
  source: string;
  title: string;
  template: string;
  layout: string;
  theme: string;
  meta: Record<string, unknown> | null;
  notes: string;
}

export interface Exported {
  /** "" when the user cancelled. */
  path: string;
  slides: number;
  warnings: string[];
}

export interface ThemeVar {
  name: string;
  value: string;
}

export interface TemplateInfo {
  name: string;
  description: string;
  defaultLayout: string;
  defaultTheme: string;
  layouts: string[];
  themes: { name: string; colors: ThemeVar[] }[];
  error?: string;
}

export interface DeckSettings {
  title: string;
  author: string;
  date: string;
  template: string;
  theme: string;
  aspect: string;
  width: number;
  height: number;
  output: string;
  error?: string;
}

export interface ThemeProp {
  name: string;
  value: string;
  color: boolean;
  group: string;
}

export type ExportFormat = "pdf" | "pptx" | "odp";

/** Sets a key, or removes it when value is null. */
export interface Change {
  table?: string;
  key: string;
  value: string | number | boolean | null;
}

export interface RenderResult {
  version: number;
  url: string;
  width: number;
  height: number;
  slides: SlideInfo[];
  warnings: string[];
}

async function call<T>(method: string, route: string, body?: BodyInit): Promise<T> {
  const res = await fetch(`/api/${route}`, { method, body });
  const data = await res.json().catch(() => ({ error: `${res.status} ${res.statusText}` }));
  if (!res.ok) throw new Error(data?.error ?? `${res.status} ${res.statusText}`);
  return data as T;
}

export const api = {
  project: () => call<ProjectInfo | null>("GET", "project"),
  open: () => call<ProjectInfo | null>("POST", "open"),
  create: () => call<ProjectInfo | null>("POST", "new"),
  read: (path: string) =>
    call<{ text: string }>("GET", `file?path=${encodeURIComponent(path)}`).then((r) => r.text),
  write: (path: string, text: string) =>
    call<{ ok: boolean }>("PUT", `file?path=${encodeURIComponent(path)}`, text),
  render: (overlay: Record<string, string>) =>
    call<RenderResult>("POST", "render", JSON.stringify({ overlay })),
  exportAs: (format: ExportFormat) => call<Exported>("POST", `export/${format}`),
  themeProps: (text: string) => call<ThemeProp[]>("POST", "theme/props", JSON.stringify({ text })),
  editTheme: (text: string, changes: { name: string; value: string }[]) =>
    call<{ text: string }>("POST", "edit/theme", JSON.stringify({ text, changes })).then((r) => r.text),
  duplicateTheme: (template: string, theme: string, name: string) =>
    call<{ name: string }>("POST", "theme/duplicate", JSON.stringify({ template, theme, name })),
  templates: () => call<TemplateInfo[]>("GET", "templates"),
  parseSettings: (text: string) => call<DeckSettings>("POST", "settings/parse", JSON.stringify({ text })),
  edit: (kind: "settings" | "frontmatter", text: string, changes: Change[]) =>
    call<{ text: string }>("POST", `edit/${kind}`, JSON.stringify({ text, changes })).then((r) => r.text),
  importTheme: () => call<{ name: string }>("POST", "theme/import").then((r) => r.name),
  exportTheme: (name: string) =>
    call<{ path: string }>("POST", "theme/export", JSON.stringify({ name })).then((r) => r.path),
  fullscreen: (on: boolean) => call<{ ok: boolean }>("POST", "fullscreen", JSON.stringify({ on })),
  shareDeck: () => call<{ path: string; files?: number }>("POST", "deck/share"),
  openDeck: () => call<ProjectInfo | null>("POST", "deck/open"),
  reveal: (path: string) => call<{ ok: boolean }>("POST", "reveal", JSON.stringify({ path })),
};
