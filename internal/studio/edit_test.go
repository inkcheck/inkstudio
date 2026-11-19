package studio

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEditSettings(t *testing.T) {
	in := `# My talk
title = "Old"   # the title
template = "default"

[page]
width = 1280
height = 720

[meta]
x = 1
`
	out, err := EditSettings(in, []Change{
		{Key: "title", Value: `Quarterly "review"`},
		{Key: "theme", Value: "dark"},
		{Table: "page", Key: "height", Value: nil},
		{Table: "page", Key: "aspect", Value: "297:210"},
		{Table: "page", Key: "width", Value: float64(1123)},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := `# My talk
title = "Quarterly \"review\""  # the title
template = "default"
theme = "dark"

[page]
width = 1123
aspect = "297:210"

[meta]
x = 1
`
	if out != want {
		t.Errorf("got:\n%s\nwant:\n%s", out, want)
	}
	if s := ParseDeckSettings(out); s.Error != "" || s.Aspect != "297:210" || s.Theme != "dark" {
		t.Errorf("parsed = %+v", s)
	}
}

func TestEditSettingsNewTable(t *testing.T) {
	out, err := EditSettings("title = \"T\"\n", []Change{{Table: "page", Key: "aspect", Value: "4:3"}})
	if err != nil {
		t.Fatal(err)
	}
	if out != "title = \"T\"\n\n[page]\naspect = \"4:3\"\n" {
		t.Errorf("got %q", out)
	}
	if out, _ := EditSettings("", []Change{{Key: "theme", Value: "dark"}}); out != "theme = \"dark\"\n" {
		t.Errorf("empty settings: got %q", out)
	}
	if _, err := EditSettings("", []Change{{Key: "bad key", Value: "x"}}); err == nil {
		t.Error("accepted a bad key")
	}
}

func TestEditFrontmatter(t *testing.T) {
	for _, tc := range []struct {
		name, in string
		changes  []Change
		want     string
	}{
		{
			"add block", "# Hello\n",
			[]Change{{Key: "layout", Value: "title"}},
			"---\nlayout: title\n---\n# Hello\n",
		},
		{
			"replace and add", "---\nlayout: content\nsubtitle: Hi # note\n---\n\n# Hello\n",
			[]Change{{Key: "layout", Value: "two-column"}, {Key: "theme", Value: "dark"}},
			"---\nlayout: two-column\nsubtitle: Hi # note\ntheme: dark\n---\n\n# Hello\n",
		},
		{
			"remove the last key drops the block", "---\nlayout: title\n---\n\n# Hello\n",
			[]Change{{Key: "layout", Value: nil}},
			"# Hello\n",
		},
		{
			"remove a multi-line value", "---\nmeta:\n  a: 1\n  b: 2\nlayout: x\n---\nBody\n",
			[]Change{{Key: "meta", Value: nil}},
			"---\nlayout: x\n---\nBody\n",
		},
		{
			"quote what YAML would misread", "Body\n",
			[]Change{{Key: "theme", Value: "yes"}, {Key: "title", Value: "A: b"}},
			"---\ntheme: \"yes\"\ntitle: \"A: b\"\n---\nBody\n",
		},
		{
			"toml frontmatter", "+++\nlayout = \"content\"\n+++\nBody\n",
			[]Change{{Key: "layout", Value: "title"}},
			"+++\nlayout = \"title\"\n+++\nBody\n",
		},
		{
			"no frontmatter, nothing to remove", "Body\n",
			[]Change{{Key: "layout", Value: nil}},
			"Body\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := EditFrontmatter(tc.in, tc.changes)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("got:\n%q\nwant:\n%q", got, tc.want)
			}
		})
	}
	if _, err := EditFrontmatter("---\nlayout: x\n", []Change{{Key: "layout", Value: "y"}}); err == nil {
		t.Error("edited an unclosed frontmatter")
	}
}

func TestTemplates(t *testing.T) {
	p, err := Open(samplePack(t))
	if err != nil {
		t.Fatal(err)
	}
	list, err := p.Templates()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Name != "default" || len(list[0].Layouts) < 10 || len(list[0].Themes) != 2 {
		t.Fatalf("templates = %+v", list)
	}
	dark := list[0].Themes[0]
	if dark.Name != "dark" || len(dark.Colors) == 0 || dark.Colors[0].Name != "--bg" {
		t.Errorf("dark = %+v", dark)
	}
}

func writeZip(t *testing.T, path string, files map[string]string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(body))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	f.Close()
}

func TestThemeRoundTrip(t *testing.T) {
	p, err := Open(samplePack(t))
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "default.inktheme")
	if err := p.ExportTheme("default", out); err != nil {
		t.Fatal(err)
	}
	name, err := ThemeName(filepath.Join(filepath.Dir(out), "My Theme.inktheme"))
	if err != nil || name != "My-Theme" {
		t.Errorf("ThemeName = %q, %v", name, err)
	}
	if err := p.ImportTheme(out, "copy", false); err != nil {
		t.Fatal(err)
	}
	list, _ := p.Templates()
	if len(list) != 2 || list[0].Name != "copy" || list[0].Error != "" || len(list[0].Layouts) != len(list[1].Layouts) {
		t.Fatalf("after import: %+v", list)
	}
	if err := p.ImportTheme(out, "copy", false); err == nil {
		t.Error("imported over an existing template without replace")
	}
	if err := p.ImportTheme(out, "copy", true); err != nil {
		t.Errorf("replace: %v", err)
	}
	// Nothing is left behind in .templates.
	entries, _ := os.ReadDir(filepath.Join(p.Dir, ".templates"))
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			t.Errorf("left behind %s", e.Name())
		}
	}
}

func TestThemeImportNested(t *testing.T) {
	p, err := Open(samplePack(t))
	if err != nil {
		t.Fatal(err)
	}
	zipPath := filepath.Join(t.TempDir(), "nested.inktheme")
	writeZip(t, zipPath, map[string]string{
		"mine/template.toml":        "layout = \"content\"\ntheme = \"light\"\n",
		"mine/style.css":            ".slide { color: var(--fg); }",
		"mine/themes/light.css":     ".slide { --fg: #111; }",
		"mine/layouts/content.html": "{{.Content}}",
		"__MACOSX/mine/._style.css": "junk",
		"mine/.DS_Store":            "junk",
	})
	if err := p.ImportTheme(zipPath, "nested", false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(p.Dir, ".templates/nested/layouts/content.html")); err != nil {
		t.Error(err)
	}
	if _, err := os.Stat(filepath.Join(p.Dir, ".templates/nested/.DS_Store")); err == nil {
		t.Error(".DS_Store was unpacked")
	}
}

func TestThemeImportRefusesBadZips(t *testing.T) {
	p, err := Open(samplePack(t))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	base := map[string]string{"template.toml": "", "style.css": "", "layouts/content.html": "x", "themes/light.css": ""}
	for name, extra := range map[string]map[string]string{
		"escape":  {"../../evil.txt": "x"},
		"abs":     {"/tmp/evil.txt": "x"},
		"none":    nil,
		"twoTpls": {"a/template.toml": "", "b/template.toml": ""},
	} {
		files := map[string]string{}
		if name != "none" && name != "twoTpls" {
			for k, v := range base {
				files[k] = v
			}
		}
		for k, v := range extra {
			files[k] = v
		}
		if name == "none" {
			files["readme.txt"] = "no template"
		}
		zipPath := filepath.Join(dir, name+".inktheme")
		writeZip(t, zipPath, files)
		if err := p.ImportTheme(zipPath, name, false); err == nil {
			t.Errorf("%s: imported", name)
		}
		if p.TemplateExists(name) {
			t.Errorf("%s: template left behind", name)
		}
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(p.Dir), "evil.txt")); err == nil {
		t.Error("a file escaped the pack")
	}
	notZip := filepath.Join(dir, "plain.inktheme")
	os.WriteFile(notZip, []byte("not a zip"), 0o644)
	if err := p.ImportTheme(notZip, "plain", false); err == nil {
		t.Error("imported a file that is not a zip")
	}
}

func TestThemeProps(t *testing.T) {
	css := `/* Colours for the dark theme. */
.slide {
  --bg: #161616;
  --fg: #f4f4f4;
  /* --bg: #000; commented out */
  --font: "IBM Plex Sans", sans-serif;
  /* Callout tones: note, tip. */
  --note: rgb(120 169 255);
  --bg: #ffffff;
}`
	props := ThemeProps(css)
	if len(props) != 4 {
		t.Fatalf("props = %+v", props)
	}
	if props[0].Name != "--bg" || props[0].Value != "#161616" || !props[0].Color || props[0].Group != "Colours for the dark theme" {
		t.Errorf("first = %+v", props[0])
	}
	if props[2].Color || props[3].Group != "Callout tones" || !props[3].Color {
		t.Errorf("props = %+v", props)
	}

	out, err := EditTheme(css, []PropChange{{Name: "--bg", Value: "#101010"}, {Name: "--accent", Value: "#ff0000"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "--bg: #101010;\n  --fg") || !strings.Contains(out, "/* --bg: #000; commented out */") || !strings.Contains(out, "--bg: #ffffff;") {
		t.Errorf("edit touched the wrong declaration:\n%s", out)
	}
	if !strings.Contains(out, "  --bg: #ffffff;\n  --accent: #ff0000;\n}") {
		t.Errorf("new property not added to the rule:\n%s", out)
	}
	for _, bad := range []PropChange{{"--x", "red; } body { color: red"}, {"bg", "red"}, {"--x", ""}, {"--x", "a /* b"}} {
		if _, err := EditTheme(css, []PropChange{bad}); err == nil {
			t.Errorf("accepted %+v", bad)
		}
	}
	if out, _ := EditTheme("", []PropChange{{"--bg", "#fff"}}); out != "\n.slide {\n  --bg: #fff;\n}\n" {
		t.Errorf("empty file: %q", out)
	}
}

func TestDuplicateTheme(t *testing.T) {
	p, err := Open(samplePack(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := p.DuplicateTheme("default", "dark", "midnight"); err != nil {
		t.Fatal(err)
	}
	list, _ := p.Templates()
	if len(list[0].Themes) != 3 {
		t.Errorf("themes = %+v", list[0].Themes)
	}
	if err := p.DuplicateTheme("default", "dark", "midnight"); err == nil {
		t.Error("duplicated over an existing theme")
	}
	if err := p.DuplicateTheme("default", "dark", "../x"); err == nil {
		t.Error("accepted a path as a name")
	}
}

func TestShareAndUnpackDeck(t *testing.T) {
	p, err := Open(samplePack(t))
	if err != nil {
		t.Fatal(err)
	}
	// Things that stay out of a shared deck.
	for name, body := range map[string]string{
		"talk.pdf": "pdf", "talk.pptx": "x", ".git/HEAD": "ref", "slides/.inkpress-x.d2": "x", ".DS_Store": "x",
	} {
		os.MkdirAll(filepath.Join(p.Dir, filepath.Dir(name)), 0o755)
		os.WriteFile(filepath.Join(p.Dir, name), []byte(body), 0o644)
	}
	out := filepath.Join(t.TempDir(), "My Talk.inkdeck")
	n, err := p.ShareDeck(out)
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.OpenReader(out)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, f := range zr.File {
		names[f.Name] = true
	}
	zr.Close()
	if len(names) != n {
		t.Errorf("%d entries, %d reported", len(names), n)
	}
	for _, want := range []string{"talk/settings.toml", "talk/slides/01-title.md", "talk/.templates/default/template.toml", "talk/.templates/default/fonts/IBMPlexSans-Regular.ttf", "talk/assets/cover.svg"} {
		if !names[want] {
			t.Errorf("missing %s", want)
		}
	}
	for name := range names {
		for _, bad := range []string{".pdf", ".pptx", ".git/", ".inkpress-", ".DS_Store"} {
			if strings.Contains(name, bad) {
				t.Errorf("packed %s", name)
			}
		}
	}

	dest := t.TempDir()
	folder, err := UnpackDeck(out, dest)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(folder) != "My Talk" {
		t.Errorf("folder = %s", folder)
	}
	q, err := Open(folder)
	if err != nil {
		t.Fatal(err)
	}
	if pv, err := q.Render(nil); err != nil || len(pv.Slides) != 13 {
		t.Fatalf("render the unpacked deck: %v", err)
	}
	// A second copy gets its own folder.
	again, err := UnpackDeck(out, dest)
	if err != nil || filepath.Base(again) != "My Talk 2" {
		t.Errorf("second unpack = %s, %v", again, err)
	}
}

func TestUnpackDeckRefusesBadZips(t *testing.T) {
	dir := t.TempDir()
	dest := t.TempDir()
	for name, files := range map[string]map[string]string{
		"noslides": {"talk/settings.toml": ""},
		"escape":   {"talk/slides/01.md": "# x", "talk/../../evil.md": "x"},
		"two":      {"a/slides/01.md": "", "b/slides/01.md": ""},
	} {
		zipPath := filepath.Join(dir, name+".inkdeck")
		writeZip(t, zipPath, files)
		if _, err := UnpackDeck(zipPath, dest); err == nil {
			t.Errorf("%s: unpacked", name)
		}
	}
	entries, _ := os.ReadDir(dest)
	if len(entries) != 0 {
		t.Errorf("left behind: %v", entries)
	}
	// A zip of the pack's contents, without the folder, works too.
	flat := filepath.Join(dir, "flat.inkdeck")
	writeZip(t, flat, map[string]string{"slides/01.md": "# Hi"})
	if folder, err := UnpackDeck(flat, dest); err != nil || filepath.Base(folder) != "flat" {
		t.Errorf("flat: %s, %v", folder, err)
	}
}
