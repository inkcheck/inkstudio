// Package odp writes an OpenDocument presentation with one picture per
// slide and the speaker notes on the notes pages, for LibreOffice Impress
// and other OpenDocument apps.
package odp

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
	"time"
)

// Slide is one slide: a PNG that fills it, and its notes.
type Slide struct {
	PNG   []byte
	Title string
	// Notes are plain text; blank lines separate paragraphs.
	Notes string
}

// Deck is a presentation.
type Deck struct {
	Title, Author string
	// Width and Height are the slide size in CSS pixels, 96 per inch.
	Width, Height float64
	Slides        []Slide
	Created       time.Time
}

const mimeType = "application/vnd.oasis.opendocument.presentation"

const ns = `xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0" ` +
	`xmlns:style="urn:oasis:names:tc:opendocument:xmlns:style:1.0" ` +
	`xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0" ` +
	`xmlns:draw="urn:oasis:names:tc:opendocument:xmlns:drawing:1.0" ` +
	`xmlns:fo="urn:oasis:names:tc:opendocument:xmlns:xsl-fo-compatible:1.0" ` +
	`xmlns:xlink="http://www.w3.org/1999/xlink" ` +
	`xmlns:dc="http://purl.org/dc/elements/1.1/" ` +
	`xmlns:meta="urn:oasis:names:tc:opendocument:xmlns:meta:1.0" ` +
	`xmlns:presentation="urn:oasis:names:tc:opendocument:xmlns:presentation:1.0" ` +
	`xmlns:svg="urn:oasis:names:tc:opendocument:xmlns:svg-compatible:1.0" ` +
	`office:version="1.3"`

const xmlH = `<?xml version="1.0" encoding="UTF-8"?>` + "\n"

// cm converts CSS pixels to centimetres.
func cm(px float64) string { return fmt.Sprintf("%.3fcm", px/96*2.54) }

// Write writes the presentation to w.
func Write(w io.Writer, d Deck) error {
	if len(d.Slides) == 0 {
		return fmt.Errorf("odp: no slides")
	}
	created := d.Created
	if created.IsZero() {
		created = time.Now()
	}
	z := zip.NewWriter(w)
	// The mimetype comes first, uncompressed, so tools can sniff it.
	put := func(name string, method uint16, b []byte) error {
		f, err := z.CreateHeader(&zip.FileHeader{Name: name, Method: method, Modified: created})
		if err != nil {
			return err
		}
		_, err = f.Write(b)
		return err
	}
	if err := put("mimetype", zip.Store, []byte(mimeType)); err != nil {
		return err
	}

	var manifest strings.Builder
	manifest.WriteString(xmlH + `<manifest:manifest xmlns:manifest="urn:oasis:names:tc:opendocument:xmlns:manifest:1.0" manifest:version="1.3">`)
	fmt.Fprintf(&manifest, `<manifest:file-entry manifest:full-path="/" manifest:version="1.3" manifest:media-type="%s"/>`, mimeType)
	for _, f := range []string{"content.xml", "styles.xml", "meta.xml"} {
		fmt.Fprintf(&manifest, `<manifest:file-entry manifest:full-path="%s" manifest:media-type="text/xml"/>`, f)
	}
	for i := range d.Slides {
		fmt.Fprintf(&manifest, `<manifest:file-entry manifest:full-path="Pictures/slide%d.png" manifest:media-type="image/png"/>`, i+1)
	}
	manifest.WriteString(`</manifest:manifest>`)

	for _, p := range []struct {
		name, body string
	}{
		{"META-INF/manifest.xml", manifest.String()},
		{"meta.xml", metaXML(d, created)},
		{"styles.xml", stylesXML(d.Width, d.Height)},
		{"content.xml", contentXML(d)},
	} {
		if err := put(p.name, zip.Deflate, []byte(p.body)); err != nil {
			return err
		}
	}
	for i, s := range d.Slides {
		if err := put(fmt.Sprintf("Pictures/slide%d.png", i+1), zip.Store, s.PNG); err != nil {
			return err
		}
	}
	return z.Close()
}

func esc(s string) string {
	s = strings.Map(func(r rune) rune {
		if r == '\t' || r == '\n' || r == '\r' || (r >= 0x20 && r != 0xFFFE && r != 0xFFFF) {
			return r
		}
		return -1
	}, s)
	var b strings.Builder
	xml.EscapeText(&b, []byte(s))
	return strings.ReplaceAll(b.String(), `"`, "&#34;")
}

func metaXML(d Deck, created time.Time) string {
	ts := created.UTC().Format("2006-01-02T15:04:05")
	return xmlH + `<office:document-meta ` + ns + `><office:meta>` +
		`<meta:generator>InkStudio</meta:generator>` +
		`<dc:title>` + esc(d.Title) + `</dc:title>` +
		`<meta:initial-creator>` + esc(d.Author) + `</meta:initial-creator>` +
		`<meta:creation-date>` + ts + `</meta:creation-date>` +
		`</office:meta></office:document-meta>`
}

// A4 portrait, for the notes pages.
const notesW, notesH = 793.7, 1122.5

func stylesXML(w, h float64) string {
	orient := "landscape"
	if h > w {
		orient = "portrait"
	}
	return xmlH + `<office:document-styles ` + ns + `>` +
		`<office:styles><style:default-style style:family="graphic"><style:graphic-properties draw:stroke="none" draw:fill="none"/></style:default-style></office:styles>` +
		`<office:automatic-styles>` +
		fmt.Sprintf(`<style:page-layout style:name="PM1"><style:page-layout-properties fo:margin-top="0cm" fo:margin-bottom="0cm" fo:margin-left="0cm" fo:margin-right="0cm" fo:page-width="%s" fo:page-height="%s" style:print-orientation="%s"/></style:page-layout>`, cm(w), cm(h), orient) +
		fmt.Sprintf(`<style:page-layout style:name="PM2"><style:page-layout-properties fo:margin-top="0cm" fo:margin-bottom="0cm" fo:margin-left="0cm" fo:margin-right="0cm" fo:page-width="%s" fo:page-height="%s" style:print-orientation="portrait"/></style:page-layout>`, cm(notesW), cm(notesH)) +
		`<style:style style:name="Mdp1" style:family="drawing-page"><style:drawing-page-properties draw:fill="none"/></style:style>` +
		`</office:automatic-styles>` +
		`<office:master-styles><style:master-page style:name="Default" style:page-layout-name="PM1" draw:style-name="Mdp1">` +
		`<presentation:notes style:page-layout-name="PM2"/>` +
		`</style:master-page></office:master-styles></office:document-styles>`
}

func contentXML(d Deck) string {
	var b strings.Builder
	b.WriteString(xmlH + `<office:document-content ` + ns + `>`)
	b.WriteString(`<office:automatic-styles>` +
		`<style:style style:name="dp1" style:family="drawing-page"><style:drawing-page-properties presentation:background-visible="true" presentation:background-objects-visible="true"/></style:style>` +
		`<style:style style:name="gr1" style:family="graphic"><style:graphic-properties draw:stroke="none" draw:fill="none"/></style:style>` +
		`</office:automatic-styles>`)
	b.WriteString(`<office:body><office:presentation>`)
	// The slide picture fills the top of the notes page, the notes the rest.
	imgW := notesW - 2*72
	imgH := imgW * d.Height / d.Width
	if imgH > notesH*0.4 {
		imgH = notesH * 0.4
		imgW = imgH * d.Width / d.Height
	}
	for i, s := range d.Slides {
		k := i + 1
		fmt.Fprintf(&b, `<draw:page draw:name="page%d" draw:style-name="dp1" draw:master-page-name="Default">`, k)
		fmt.Fprintf(&b, `<draw:frame draw:style-name="gr1" draw:layer="layout" svg:x="0cm" svg:y="0cm" svg:width="%s" svg:height="%s">`, cm(d.Width), cm(d.Height))
		fmt.Fprintf(&b, `<draw:image xlink:href="Pictures/slide%d.png" xlink:type="simple" xlink:show="embed" xlink:actuate="onLoad"/>`, k)
		if s.Title != "" {
			fmt.Fprintf(&b, `<svg:title>%s</svg:title>`, esc(s.Title))
		}
		b.WriteString(`</draw:frame>`)
		b.WriteString(`<presentation:notes draw:style-name="dp1">`)
		fmt.Fprintf(&b, `<draw:page-thumbnail draw:layer="layout" svg:x="%s" svg:y="%s" svg:width="%s" svg:height="%s" draw:page-number="%d" presentation:class="page"/>`,
			cm((notesW-imgW)/2), cm(72), cm(imgW), cm(imgH), k)
		fmt.Fprintf(&b, `<draw:frame draw:layer="layout" presentation:class="notes" svg:x="%s" svg:y="%s" svg:width="%s" svg:height="%s"><draw:text-box>`,
			cm(72), cm(72+imgH+36), cm(notesW-2*72), cm(notesH-72-imgH-36-72))
		paras := paragraphs(s.Notes)
		if len(paras) == 0 {
			b.WriteString(`<text:p/>`)
		}
		for _, p := range paras {
			fmt.Fprintf(&b, `<text:p>%s</text:p>`, esc(p))
		}
		b.WriteString(`</draw:text-box></draw:frame></presentation:notes>`)
		b.WriteString(`</draw:page>`)
	}
	b.WriteString(`</office:presentation></office:body></office:document-content>`)
	return b.String()
}

// paragraphs splits notes at blank lines, and puts each list item on its
// own line.
func paragraphs(notes string) []string {
	var out, cur []string
	flush := func() {
		if len(cur) > 0 {
			out = append(out, strings.Join(cur, " "))
			cur = nil
		}
	}
	for _, line := range strings.Split(strings.ReplaceAll(notes, "\r\n", "\n"), "\n") {
		t := strings.TrimSpace(line)
		switch {
		case t == "":
			flush()
		case strings.HasPrefix(t, "- ") || strings.HasPrefix(t, "* ") || strings.HasPrefix(t, "+ "):
			flush()
			out = append(out, t)
		default:
			cur = append(cur, t)
		}
	}
	flush()
	return out
}
