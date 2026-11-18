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
	target := regexp.MustCompile(`Target="([^"]+)"`)
	for name, b := range parts {
		if !strings.HasSuffix(name, ".rels") {
			continue
		}
		base := path.Dir(path.Dir(name)) // the part the rels belong to lives here
		for _, m := range target.FindAllStringSubmatch(string(b), -1) {
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
	err := Write(&buf, Deck{
		Title: `Q3 <review> & "plans"`, Author: "A. Presenter", Width: 1123, Height: 794,
		Slides: []Slide{
			{PNG: tinyPNG(t), Title: "Intro", Notes: "Hello <everyone>.\n\nSecond paragraph\ncontinues here.\n- one\n- two"},
			{PNG: tinyPNG(t), Title: "No notes"},
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
	if _, ok := parts["ppt/media/image2.png"]; !ok {
		t.Error("image2.png missing")
	}
}

func TestSizeLimits(t *testing.T) {
	var buf bytes.Buffer
	// 20000 px is 208 inches: too wide for PowerPoint, so it is scaled down.
	if err := Write(&buf, Deck{Width: 20000, Height: 5000, Slides: []Slide{{PNG: tinyPNG(t)}}}); err != nil {
		t.Fatal(err)
	}
	pres := string(Check(t, buf.Bytes())["ppt/presentation.xml"])
	if !strings.Contains(pres, `cx="51206400" cy="12801600"`) {
		t.Errorf("not clamped: %s", pres)
	}
	if err := Write(&buf, Deck{Width: 100, Height: 100}); err == nil {
		t.Error("wrote a deck with no slides")
	}
}
