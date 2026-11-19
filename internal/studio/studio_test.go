package studio

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/inkcheck/inkpress/pdf"
	"github.com/inkcheck/inkpress/scaffold"
)

func samplePack(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "talk")
	if _, err := scaffold.New(dir, false); err != nil {
		t.Fatal(err)
	}
	// No inkline in tests: diagrams stay as source.
	t.Setenv("INKPRESS_INKLINE", "")
	t.Setenv("PATH", "")
	return dir
}

func TestAbs(t *testing.T) {
	p := &Project{Dir: "/pack"}
	for _, bad := range []string{"", "..", "../x", "a/../../x", "/etc/passwd", `a\b`} {
		if _, err := p.Abs(bad); err == nil {
			t.Errorf("Abs(%q) accepted", bad)
		}
	}
	if got, err := p.Abs("slides/01.md"); err != nil || got != filepath.FromSlash("/pack/slides/01.md") {
		t.Errorf("Abs = %q, %v", got, err)
	}
}

func TestEditable(t *testing.T) {
	for rel, want := range map[string]bool{
		"slides/01.md":                         true,
		"settings.toml":                        true,
		".templates/default/style.css":         true,
		".templates/default/fonts/x.ttf":       false,
		".git/config":                          false,
		"slides/.inkpress-abc.d2":              false,
		"assets/cover.svg":                     false,
		".templates/default/.hidden/style.css": false,
	} {
		if got := editable(rel); got != want {
			t.Errorf("editable(%q) = %v, want %v", rel, got, want)
		}
	}
}

func TestInfoAndRender(t *testing.T) {
	p, err := Open(samplePack(t))
	if err != nil {
		t.Fatal(err)
	}
	info, err := p.Info()
	if err != nil {
		t.Fatal(err)
	}
	if info.Files[0].Path != "settings.toml" {
		t.Errorf("first file = %q, want settings.toml", info.Files[0].Path)
	}

	slide := "slides/01-title.md"
	pv, err := p.Render(map[string]string{slide: "# Unsaved title\n"})
	if err != nil {
		t.Fatal(err)
	}
	html := string(pv.HTML)
	if !strings.Contains(html, "Unsaved title") {
		t.Error("the overlay text is not in the preview")
	}
	if strings.Contains(html, "file:///") {
		t.Error("the preview still has file: URLs")
	}
	if !strings.Contains(html, `<base href="`+FSPrefix) {
		t.Error("the base URL does not point at the asset server")
	}
	if pv.Slides[0].Source != slide || pv.Slides[0].Title != "Unsaved title" {
		t.Errorf("slide 1 = %+v", pv.Slides[0])
	}
	if pv.Width != 1280 || pv.Height != 720 {
		t.Errorf("size = %gx%g", pv.Width, pv.Height)
	}
}

func TestWriteFile(t *testing.T) {
	p, err := Open(samplePack(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := p.WriteFile("slides/01-title.md", "# New"); err != nil {
		t.Fatal(err)
	}
	if got, _ := p.ReadFile("slides/01-title.md"); got != "# New" {
		t.Errorf("read back %q", got)
	}
	if err := p.WriteFile("../escape.md", "x"); err == nil {
		t.Error("wrote outside the pack")
	}
	if err := p.WriteFile("assets/x.svg", "x"); err == nil {
		t.Error("wrote a file that is not editable")
	}
}

func TestServer(t *testing.T) {
	dir := samplePack(t)
	s := New(Host{})
	if _, err := s.OpenDir(dir); err != nil {
		t.Fatal(err)
	}
	h := s.Middleware(http.NotFoundHandler())
	do := func(method, target string, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(method, target, strings.NewReader(body)))
		return rec
	}

	rec := do("POST", "/api/render", `{"overlay":{}}`)
	if rec.Code != 200 {
		t.Fatalf("render: %d %s", rec.Code, rec.Body)
	}
	var res renderResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if rec := do("GET", res.URL, ""); rec.Code != 200 || !bytes.Contains(rec.Body.Bytes(), []byte("studio-current")) {
		t.Errorf("preview: %d", rec.Code)
	}

	// The pack's files are served; nothing else is.
	logo := filepath.ToSlash(filepath.Join(dir, ".templates/default/assets/logo.svg"))
	if rec := do("GET", FSPrefix+strings.TrimPrefix(logo, "/"), ""); rec.Code != 200 {
		t.Errorf("pack file: %d", rec.Code)
	}
	outside := filepath.ToSlash(filepath.Join(filepath.Dir(dir), "secret.txt"))
	os.WriteFile(outside, []byte("no"), 0o644)
	if rec := do("GET", FSPrefix+strings.TrimPrefix(outside, "/"), ""); rec.Code != 404 {
		t.Errorf("file outside the pack: %d", rec.Code)
	}
	if rec := do("GET", FSPrefix+strings.TrimPrefix(dir, "/")+"/../secret.txt", ""); rec.Code != 404 {
		t.Errorf("dot-dot path: %d", rec.Code)
	}

	if rec := do("GET", "/api/file?path=../secret.txt", ""); rec.Code == 200 {
		t.Error("read a file outside the pack")
	}
	if rec := do("GET", "/elsewhere", ""); rec.Code != 404 {
		t.Errorf("fallthrough: %d", rec.Code)
	}
}

func TestWatch(t *testing.T) {
	p, err := Open(samplePack(t))
	if err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	defer close(stop)
	got := make(chan []string, 4)
	if err := p.watch(stop, func(paths []string) { got <- paths }); err != nil {
		t.Fatal(err)
	}
	// A hidden file, like inkpress's temporary diagram source, is ignored.
	os.WriteFile(filepath.Join(p.Dir, "slides", ".inkpress-x.d2"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(p.Dir, "slides", "99-new.md"), []byte("# New"), 0o644)
	select {
	case paths := <-got:
		if len(paths) != 1 || paths[0] != "slides/99-new.md" {
			t.Errorf("changed = %v", paths)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no change reported")
	}
}

func TestExportPDF(t *testing.T) {
	if testing.Short() {
		t.Skip("prints with a browser")
	}
	if _, err := pdf.FindBrowser(); err != nil {
		t.Skip("no Chromium-based browser")
	}
	p, err := Open(samplePack(t))
	if err != nil {
		t.Fatal(err)
	}
	if got := p.DefaultPDF(); filepath.Base(got) != "talk.pdf" {
		t.Errorf("DefaultPDF = %q", got)
	}
	out := filepath.Join(t.TempDir(), "out.pdf")
	res, err := p.ExportPDF(t.Context(), out)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(out)
	if err != nil || !bytes.HasPrefix(b, []byte("%PDF")) {
		t.Fatalf("not a PDF: %v", err)
	}
	if res.Slides != 13 {
		t.Errorf("slides = %d", res.Slides)
	}
}

func TestRenderSlideInfo(t *testing.T) {
	p, err := Open(samplePack(t))
	if err != nil {
		t.Fatal(err)
	}
	pv, err := p.Render(map[string]string{
		"slides/01-title.md": "---\nlayout: title\nkicker: Hello\n---\n# Title\n\n!--\nSay hi.\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	s := pv.Slides[0]
	if s.Layout != "title" || s.Template != "default" || s.Theme == "" || s.Meta["kicker"] != "Hello" || s.Notes != "Say hi." {
		t.Errorf("slide 1 = %+v", s)
	}
}

func TestExportSlides(t *testing.T) {
	if testing.Short() {
		t.Skip("renders with a browser")
	}
	if _, err := pdf.FindBrowser(); err != nil {
		t.Skip("no Chromium-based browser")
	}
	p, err := Open(samplePack(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []Format{FormatPPTXPictures} {
		out := filepath.Join(t.TempDir(), "talk."+f.Ext)
		calls := 0
		res, err := p.ExportSlides(t.Context(), f, out, func(done, total int) { calls++ })
		if err != nil {
			t.Fatalf("%s: %v", f.Ext, err)
		}
		if res.Slides != 13 || calls != 13 {
			t.Errorf("%s: %d slides, %d progress calls", f.Ext, res.Slides, calls)
		}
		zr, err := zip.OpenReader(out)
		if err != nil {
			t.Fatal(err)
		}
		var img *zip.File
		for _, zf := range zr.File {
			if strings.HasSuffix(zf.Name, "1.png") {
				img = zf
			}
		}
		if img == nil {
			t.Fatalf("%s: no slide image", f.Ext)
		}
		rc, _ := img.Open()
		cfg, err := png.DecodeConfig(rc)
		rc.Close()
		zr.Close()
		if err != nil || cfg.Width != 2560 || cfg.Height != 1440 {
			t.Errorf("%s: slide image %dx%d, %v; want 2560x1440", f.Ext, cfg.Width, cfg.Height, err)
		}
	}
}

func TestExportEditable(t *testing.T) {
	if testing.Short() {
		t.Skip("renders with a browser")
	}
	if _, err := pdf.FindBrowser(); err != nil {
		t.Skip("no Chromium-based browser")
	}
	p, err := Open(samplePack(t))
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "talk.pptx")
	if _, err := p.ExportSlides(t.Context(), FormatPPTX, out, nil); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.OpenReader(out)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	read := func(name string) string {
		for _, f := range zr.File {
			if f.Name == name {
				rc, _ := f.Open()
				b, _ := io.ReadAll(rc)
				rc.Close()
				return string(b)
			}
		}
		t.Fatalf("no %s", name)
		return ""
	}
	title := read("ppt/slides/slide1.xml")
	for _, want := range []string{`<a:t>Inkpress</a:t>`, `<a:t>Markdown in, slides out</a:t>`, `typeface="IBM Plex Sans"`, `<p:pic>`} {
		if !strings.Contains(title, want) {
			t.Errorf("slide 1 lacks %s", want)
		}
	}
	// The agenda's numbers come from CSS counters; they become text.
	if agenda := read("ppt/slides/slide2.xml"); !strings.Contains(agenda, `<a:t>01</a:t>`) {
		t.Error("slide 2 lacks its 01")
	}
	// Bullets are real bullets.
	if why := read("ppt/slides/slide3.xml"); !strings.Contains(why, `<a:buChar char="•"/>`) {
		t.Error("slide 3 has no bullets")
	}
}
