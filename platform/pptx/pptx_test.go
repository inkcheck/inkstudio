package pptx

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"image"
	"image/color"
	"image/png"
	"io"
	"path"
	"regexp"
	"strings"
	"testing"

	"github.com/inkcheck/inkstudio/platform/slidedoc"
)

func tinyPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 3))
	img.Set(1, 1, color.RGBA{255, 0, 0, 255})
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// Check opens a written package and checks what PowerPoint is strict
// about: well-formed parts, a content type for each, and relationships
// that point at parts that exist.
func Check(t *testing.T, data []byte) map[string][]byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	parts := map[string][]byte{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(rc)
		rc.Close()
		parts[f.Name] = b
		if strings.HasSuffix(f.Name, ".xml") || strings.HasSuffix(f.Name, ".rels") {
			d := xml.NewDecoder(bytes.NewReader(b))
			for {
				if _, err := d.Token(); err == io.EOF {
					break
				} else if err != nil {
					t.Fatalf("%s: %v", f.Name, err)
				}
			}
		}
	}
	types := string(parts["[Content_Types].xml"])
	for name := range parts {
		ext := path.Ext(name)
		if name == "[Content_Types].xml" || strings.Contains(types, `Extension="`+strings.TrimPrefix(ext, ".")+`"`) && ext != ".xml" {
			continue
		}
		if !strings.Contains(types, `PartName="/`+name+`"`) && ext == ".xml" {
			t.Errorf("%s has no content type override", name)
		}
	}
	target := regexp.MustCompile(`Target="([^"]+)"(?: TargetMode="(\w+)")?`)
	for name, b := range parts {
		if !strings.HasSuffix(name, ".rels") {
			continue
		}
		base := path.Dir(path.Dir(name)) // the part the rels belong to lives here
		for _, m := range target.FindAllStringSubmatch(string(b), -1) {
			if m[2] == "External" {
				continue
			}
			p := path.Clean(path.Join(base, m[1]))
			if _, ok := parts[p]; !ok {
				t.Errorf("%s points at %s, which is missing", name, p)
			}
		}
	}
	return parts
}

func TestWrite(t *testing.T) {
	var buf bytes.Buffer
	png := tinyPNG(t)
	err := Write(&buf, slidedoc.Deck{
		Title: `Q3 <review> & "plans"`, Author: "A. Presenter", Width: 1123, Height: 794,
		Slides: []slidedoc.Slide{
			slidedoc.Picture(png, 1123, 794, "Intro", "Hello <everyone>.\n\nSecond paragraph\ncontinues here.\n- one\n- two"),
			slidedoc.Picture(png, 1123, 794, "No notes", ""),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	parts := Check(t, buf.Bytes())
	if len(parts) == 0 {
		t.Fatal("empty package")
	}
	pres := string(parts["ppt/presentation.xml"])
	// A4 landscape at 96 px per inch.
	if !strings.Contains(pres, `<p:sldSz cx="10696575" cy="7562850"/>`) {
		t.Errorf("slide size: %s", pres)
	}
	notes := string(parts["ppt/notesSlides/notesSlide1.xml"])
	for _, want := range []string{"Hello &lt;everyone&gt;.", "Second paragraph continues here.", "<a:t>- one</a:t>", "<a:t>- two</a:t>"} {
		if !strings.Contains(notes, want) {
			t.Errorf("notes lack %q", want)
		}
	}
	if !strings.Contains(string(parts["docProps/core.xml"]), "Q3 &lt;review&gt; &amp;") {
		t.Error("title not escaped in core.xml")
	}
	if _, ok := parts["ppt/media/image2-1.png"]; !ok {
		t.Error("image2-1.png missing")
	}
	if !strings.Contains(string(parts["docProps/app.xml"]), "<AppVersion>16.0000</AppVersion>") {
		t.Error("no current application version")
	}
}

func TestSizeLimits(t *testing.T) {
	var buf bytes.Buffer
	// 20000 px is 208 inches: too wide for PowerPoint, so it is scaled down.
	if err := Write(&buf, slidedoc.Deck{Width: 20000, Height: 5000, Slides: []slidedoc.Slide{slidedoc.Picture(tinyPNG(t), 20000, 5000, "", "")}}); err != nil {
		t.Fatal(err)
	}
	pres := string(Check(t, buf.Bytes())["ppt/presentation.xml"])
	if !strings.Contains(pres, `cx="51206400" cy="12801600"`) {
		t.Errorf("not clamped: %s", pres)
	}
	if err := Write(&buf, slidedoc.Deck{Width: 100, Height: 100}); err == nil {
		t.Error("wrote a deck with no slides")
	}
}

func TestEditable(t *testing.T) {
	ink := &slidedoc.Color{R: 0x16, G: 0x16, B: 0x16, A: 1}
	accent := &slidedoc.Color{R: 0x0f, G: 0x62, B: 0xfe, A: 0.5}
	slide := slidedoc.Slide{
		Background: &slidedoc.Color{R: 255, G: 255, B: 255, A: 1},
		Items: []slidedoc.Item{
			{Kind: slidedoc.KindRect, X: 10, Y: 10, W: 200, H: 100, Fill: accent, Radius: 8},
			{Kind: slidedoc.KindText, X: 20, Y: 20, W: 400, H: 60, Paras: []slidedoc.Para{
				{Align: "l", LineHeight: 40, Runs: []slidedoc.Run{
					{Text: "Bold & ", Font: "IBM Plex Sans", Size: 32, Bold: true, Color: ink},
					{Text: "linked", Font: "IBM Plex Sans", Size: 32, Color: ink, Href: "https://example.com/?a=1&b=2"},
				}},
				{Bullet: "•", Indent: 24, Before: 8, LineHeight: 30, Runs: []slidedoc.Run{{Text: "point", Font: "Arial", Size: 24}}},
				{Bullet: "1.", Start: 3, Indent: 24, LineHeight: 30, Runs: []slidedoc.Run{{Text: "third", Font: "Arial", Size: 24}}},
			}},
			{Kind: slidedoc.KindImage, X: 300, Y: 200, W: 40, H: 30, PNG: tinyPNG(t), Alt: "logo"},
		},
	}
	var buf bytes.Buffer
	if err := Write(&buf, slidedoc.Deck{Width: 1280, Height: 720, Slides: []slidedoc.Slide{slide}}); err != nil {
		t.Fatal(err)
	}
	parts := Check(t, buf.Bytes())
	xml := string(parts["ppt/slides/slide1.xml"])
	for _, want := range []string{
		`<p:bg><p:bgPr><a:solidFill><a:srgbClr val="FFFFFF"/>`,
		`prst="roundRect"`, `<a:alpha val="50000"/>`,
		`<a:t>Bold &amp; </a:t>`, `sz="2400" b="1"`, `<a:latin typeface="IBM Plex Sans"/>`,
		`<a:hlinkClick r:id="rId3"/>`, `<a:buChar char="•"/>`, `<a:buAutoNum type="arabicPeriod" startAt="3"/>`,
		`<a:lnSpc><a:spcPts val="3000"/></a:lnSpc>`, `<a:spcBef><a:spcPts val="600"/></a:spcBef>`,
		`<p:cNvSpPr txBox="1"/>`, `descr="logo"`, `<a:off x="2857500" y="1905000"/>`,
	} {
		if !strings.Contains(xml, want) {
			t.Errorf("slide lacks %s", want)
		}
	}
	rels := string(parts["ppt/slides/_rels/slide1.xml.rels"])
	if !strings.Contains(rels, `Target="https://example.com/?a=1&amp;b=2" TargetMode="External"`) {
		t.Errorf("link relationship: %s", rels)
	}
}
