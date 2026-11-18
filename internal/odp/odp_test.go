package odp

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"strings"
	"testing"
)

func TestWrite(t *testing.T) {
	var buf bytes.Buffer
	err := Write(&buf, Deck{
		Title: "Talk & more", Width: 1280, Height: 720,
		Slides: []Slide{{PNG: []byte("png"), Title: "One", Notes: "Say <this>.\n\n- and this"}, {PNG: []byte("png")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	first := zr.File[0]
	if first.Name != "mimetype" || first.Method != zip.Store {
		t.Errorf("first entry %s, method %d: want an uncompressed mimetype", first.Name, first.Method)
	}
	parts := map[string]string{}
	for _, f := range zr.File {
		rc, _ := f.Open()
		b, _ := io.ReadAll(rc)
		rc.Close()
		parts[f.Name] = string(b)
		if strings.HasSuffix(f.Name, ".xml") {
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
	for _, p := range []string{"Pictures/slide1.png", "Pictures/slide2.png"} {
		if _, ok := parts[p]; !ok {
			t.Errorf("%s missing", p)
		}
		if !strings.Contains(parts["META-INF/manifest.xml"], `full-path="`+p+`"`) {
			t.Errorf("%s not in the manifest", p)
		}
	}
	content := parts["content.xml"]
	if strings.Count(content, "<draw:page ") != 2 || !strings.Contains(content, "Say &lt;this&gt;.") {
		t.Errorf("content.xml: %s", content)
	}
	if !strings.Contains(parts["styles.xml"], `fo:page-width="33.867cm" fo:page-height="19.050cm"`) {
		t.Errorf("page size: %s", parts["styles.xml"])
	}
}
