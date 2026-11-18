// Package pptx writes a PowerPoint presentation with one picture per slide
// and the speaker notes in the notes pages. It writes the Office Open XML
// parts by hand: one master, one blank layout, one theme, no dependencies.
package pptx

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
	PNG []byte
	// Title is the picture's alternative text.
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
	// Created defaults to now.
	Created time.Time
}

// emuPerPx converts CSS pixels (96 per inch) to EMUs (914400 per inch).
const emuPerPx = 9525

// PowerPoint's limits on the slide size: 1 to 56 inches.
const (
	minEMU = 914400
	maxEMU = 51206400
)

const (
	nsA   = "http://schemas.openxmlformats.org/drawingml/2006/main"
	nsR   = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
	nsP   = "http://schemas.openxmlformats.org/presentationml/2006/main"
	relNS = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
	xmlH  = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n"
	pNS   = `xmlns:a="` + nsA + `" xmlns:r="` + nsR + `" xmlns:p="` + nsP + `"`
)

// Write writes the presentation to w.
func Write(w io.Writer, d Deck) error {
	if len(d.Slides) == 0 {
		return fmt.Errorf("pptx: no slides")
	}
	cx, cy := int64(d.Width*emuPerPx+0.5), int64(d.Height*emuPerPx+0.5)
	// Keep the aspect ratio inside PowerPoint's limits.
	if s := float64(maxEMU) / float64(max(cx, cy)); s < 1 {
		cx, cy = int64(float64(cx)*s), int64(float64(cy)*s)
	}
	if s := float64(minEMU) / float64(min(cx, cy)); s > 1 {
		cx, cy = int64(float64(cx)*s), int64(float64(cy)*s)
	}
	created := d.Created
	if created.IsZero() {
		created = time.Now()
	}

	z := zip.NewWriter(w)
	add := func(name, body string) error {
		f, err := z.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Modified: created})
		if err != nil {
			return err
		}
		_, err = io.WriteString(f, body)
		return err
	}
	addBytes := func(name string, b []byte) error {
		// PNGs are compressed already.
		f, err := z.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Store, Modified: created})
		if err != nil {
			return err
		}
		_, err = f.Write(b)
		return err
	}

	n := len(d.Slides)
	notes := 0
	for _, s := range d.Slides {
		if strings.TrimSpace(s.Notes) != "" {
			notes++
		}
	}

	parts := []struct{ name, body string }{
		{"[Content_Types].xml", contentTypes(d.Slides)},
		{"_rels/.rels", rels(
			rel{"rId1", relNS + "/officeDocument", "ppt/presentation.xml"},
			rel{"rId2", "http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties", "docProps/core.xml"},
			rel{"rId3", relNS + "/extended-properties", "docProps/app.xml"},
		)},
		{"docProps/core.xml", coreXML(d, created)},
		{"docProps/app.xml", fmt.Sprintf(xmlH+`<Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/extended-properties"><Application>inkstudio</Application><Slides>%d</Slides><Notes>%d</Notes></Properties>`, n, notes)},
		{"ppt/presentation.xml", presentationXML(n, cx, cy)},
		{"ppt/_rels/presentation.xml.rels", presentationRels(n)},
		{"ppt/presProps.xml", xmlH + `<p:presentationPr ` + pNS + `/>`},
		{"ppt/viewProps.xml", xmlH + `<p:viewPr ` + pNS + `/>`},
		{"ppt/tableStyles.xml", xmlH + `<a:tblStyleLst xmlns:a="` + nsA + `" def="{5C22544A-7EE6-4342-B048-85BDC9FD1C3A}"/>`},
		{"ppt/theme/theme1.xml", themeXML},
		{"ppt/theme/theme2.xml", themeXML},
		{"ppt/slideMasters/slideMaster1.xml", masterXML},
		{"ppt/slideMasters/_rels/slideMaster1.xml.rels", rels(
			rel{"rId1", relNS + "/slideLayout", "../slideLayouts/slideLayout1.xml"},
			rel{"rId2", relNS + "/theme", "../theme/theme1.xml"},
		)},
		{"ppt/slideLayouts/slideLayout1.xml", layoutXML},
		{"ppt/slideLayouts/_rels/slideLayout1.xml.rels", rels(
			rel{"rId1", relNS + "/slideMaster", "../slideMasters/slideMaster1.xml"},
		)},
		{"ppt/notesMasters/notesMaster1.xml", notesMasterXML(cx, cy)},
		{"ppt/notesMasters/_rels/notesMaster1.xml.rels", rels(
			rel{"rId1", relNS + "/theme", "../theme/theme2.xml"},
		)},
	}
	for _, p := range parts {
		if err := add(p.name, p.body); err != nil {
			return err
		}
	}
	for i, s := range d.Slides {
		k := i + 1
		slideRels := []rel{
			{"rId1", relNS + "/slideLayout", "../slideLayouts/slideLayout1.xml"},
			{"rId2", relNS + "/image", fmt.Sprintf("../media/image%d.png", k)},
			{"rId3", relNS + "/notesSlide", fmt.Sprintf("../notesSlides/notesSlide%d.xml", k)},
		}
		for _, p := range []struct{ name, body string }{
			{fmt.Sprintf("ppt/slides/slide%d.xml", k), slideXML(k, s.Title, cx, cy)},
			{fmt.Sprintf("ppt/slides/_rels/slide%d.xml.rels", k), rels(slideRels...)},
			{fmt.Sprintf("ppt/notesSlides/notesSlide%d.xml", k), notesSlideXML(s.Notes)},
			{fmt.Sprintf("ppt/notesSlides/_rels/notesSlide%d.xml.rels", k), rels(
				rel{"rId1", relNS + "/notesMaster", "../notesMasters/notesMaster1.xml"},
				rel{"rId2", relNS + "/slide", fmt.Sprintf("../slides/slide%d.xml", k)},
			)},
		} {
			if err := add(p.name, p.body); err != nil {
				return err
			}
		}
		if err := addBytes(fmt.Sprintf("ppt/media/image%d.png", k), s.PNG); err != nil {
			return err
		}
	}
	return z.Close()
}

type rel struct{ id, typ, target string }

func rels(rs ...rel) string {
	var b strings.Builder
	b.WriteString(xmlH + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`)
	for _, r := range rs {
		fmt.Fprintf(&b, `<Relationship Id="%s" Type="%s" Target="%s"/>`, r.id, r.typ, esc(r.target))
	}
	b.WriteString(`</Relationships>`)
	return b.String()
}

func esc(s string) string {
	var b strings.Builder
	xml.EscapeText(&b, []byte(s))
	return strings.ReplaceAll(b.String(), `"`, "&#34;")
}

// clean drops characters XML 1.0 cannot hold.
func clean(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\t' || r == '\n' || r == '\r' || (r >= 0x20 && r != 0xFFFE && r != 0xFFFF) {
			return r
		}
		return -1
	}, s)
}

func contentTypes(slides []Slide) string {
	const pml = "application/vnd.openxmlformats-officedocument.presentationml."
	var b strings.Builder
	b.WriteString(xmlH + `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">`)
	b.WriteString(`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>`)
	b.WriteString(`<Default Extension="xml" ContentType="application/xml"/>`)
	b.WriteString(`<Default Extension="png" ContentType="image/png"/>`)
	over := func(part, typ string) { fmt.Fprintf(&b, `<Override PartName="%s" ContentType="%s"/>`, part, typ) }
	over("/ppt/presentation.xml", pml+"presentation.main+xml")
	over("/ppt/presProps.xml", pml+"presProps+xml")
	over("/ppt/viewProps.xml", pml+"viewProps+xml")
	over("/ppt/tableStyles.xml", pml+"tableStyles+xml")
	over("/ppt/theme/theme1.xml", "application/vnd.openxmlformats-officedocument.theme+xml")
	over("/ppt/theme/theme2.xml", "application/vnd.openxmlformats-officedocument.theme+xml")
	over("/ppt/slideMasters/slideMaster1.xml", pml+"slideMaster+xml")
	over("/ppt/slideLayouts/slideLayout1.xml", pml+"slideLayout+xml")
	over("/ppt/notesMasters/notesMaster1.xml", pml+"notesMaster+xml")
	for i := range slides {
		over(fmt.Sprintf("/ppt/slides/slide%d.xml", i+1), pml+"slide+xml")
		over(fmt.Sprintf("/ppt/notesSlides/notesSlide%d.xml", i+1), pml+"notesSlide+xml")
	}
	over("/docProps/core.xml", "application/vnd.openxmlformats-package.core-properties+xml")
	over("/docProps/app.xml", "application/vnd.openxmlformats-officedocument.extended-properties+xml")
	b.WriteString(`</Types>`)
	return b.String()
}

func coreXML(d Deck, created time.Time) string {
	ts := created.UTC().Format("2006-01-02T15:04:05Z")
	return xmlH + `<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:dcterms="http://purl.org/dc/terms/" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">` +
		`<dc:title>` + esc(clean(d.Title)) + `</dc:title>` +
		`<dc:creator>` + esc(clean(d.Author)) + `</dc:creator>` +
		`<dcterms:created xsi:type="dcterms:W3CDTF">` + ts + `</dcterms:created>` +
		`<dcterms:modified xsi:type="dcterms:W3CDTF">` + ts + `</dcterms:modified>` +
		`</cp:coreProperties>`
}

func presentationXML(n int, cx, cy int64) string {
	var b strings.Builder
	b.WriteString(xmlH + `<p:presentation ` + pNS + ` saveSubsetFonts="1">`)
	b.WriteString(`<p:sldMasterIdLst><p:sldMasterId id="2147483648" r:id="rId1"/></p:sldMasterIdLst>`)
	b.WriteString(`<p:notesMasterIdLst><p:notesMasterId r:id="rId2"/></p:notesMasterIdLst>`)
	b.WriteString(`<p:sldIdLst>`)
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, `<p:sldId id="%d" r:id="rId%d"/>`, 256+i, 10+i)
	}
	b.WriteString(`</p:sldIdLst>`)
	fmt.Fprintf(&b, `<p:sldSz cx="%d" cy="%d"/><p:notesSz cx="6858000" cy="9144000"/>`, cx, cy)
	b.WriteString(`</p:presentation>`)
	return b.String()
}

func presentationRels(n int) string {
	rs := []rel{
		{"rId1", relNS + "/slideMaster", "slideMasters/slideMaster1.xml"},
		{"rId2", relNS + "/notesMaster", "notesMasters/notesMaster1.xml"},
		{"rId3", relNS + "/theme", "theme/theme1.xml"},
		{"rId4", relNS + "/presProps", "presProps.xml"},
		{"rId5", relNS + "/viewProps", "viewProps.xml"},
		{"rId6", relNS + "/tableStyles", "tableStyles.xml"},
	}
	for i := 0; i < n; i++ {
		rs = append(rs, rel{fmt.Sprintf("rId%d", 10+i), relNS + "/slide", fmt.Sprintf("slides/slide%d.xml", i+1)})
	}
	return rels(rs...)
}

const grpSp = `<p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="0" cy="0"/><a:chOff x="0" y="0"/><a:chExt cx="0" cy="0"/></a:xfrm></p:grpSpPr>`

const clrMap = `<p:clrMap bg1="lt1" tx1="dk1" bg2="lt2" tx2="dk2" accent1="accent1" accent2="accent2" accent3="accent3" accent4="accent4" accent5="accent5" accent6="accent6" hlink="hlink" folHlink="folHlink"/>`

const masterXML = xmlH + `<p:sldMaster ` + pNS + `><p:cSld><p:bg><p:bgRef idx="1001"><a:schemeClr val="bg1"/></p:bgRef></p:bg><p:spTree>` + grpSp + `</p:spTree></p:cSld>` + clrMap +
	`<p:sldLayoutIdLst><p:sldLayoutId id="2147483649" r:id="rId1"/></p:sldLayoutIdLst><p:txStyles><p:titleStyle/><p:bodyStyle/><p:otherStyle/></p:txStyles></p:sldMaster>`

const layoutXML = xmlH + `<p:sldLayout ` + pNS + ` type="blank" preserve="1"><p:cSld name="Blank"><p:spTree>` + grpSp + `</p:spTree></p:cSld><p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr></p:sldLayout>`

func slideXML(k int, title string, cx, cy int64) string {
	return fmt.Sprintf(xmlH+`<p:sld `+pNS+`><p:cSld><p:spTree>`+grpSp+
		`<p:pic><p:nvPicPr><p:cNvPr id="2" name="Slide %d" descr="%s"/><p:cNvPicPr><a:picLocks noChangeAspect="1"/></p:cNvPicPr><p:nvPr/></p:nvPicPr>`+
		`<p:blipFill><a:blip r:embed="rId2"/><a:stretch><a:fillRect/></a:stretch></p:blipFill>`+
		`<p:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="%d" cy="%d"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></p:spPr></p:pic>`+
		`</p:spTree></p:cSld><p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr></p:sld>`, k, esc(clean(title)), cx, cy)
}

// notesImage is where the slide picture goes on a notes page: the top half
// of a portrait page, keeping the slide's shape.
func notesImage(cx, cy int64) (x, y, w, h int64) {
	const pageW, top, boxW, boxH = 6858000, 685800, 5486400, 3657600
	w, h = boxW, boxW*cy/cx
	if h > boxH {
		w, h = boxH*cx/cy, boxH
	}
	return (pageW - w) / 2, top, w, h
}

func notesMasterXML(cx, cy int64) string {
	x, y, w, h := notesImage(cx, cy)
	return xmlH + `<p:notesMaster ` + pNS + `><p:cSld><p:bg><p:bgRef idx="1001"><a:schemeClr val="bg1"/></p:bgRef></p:bg><p:spTree>` + grpSp +
		fmt.Sprintf(`<p:sp><p:nvSpPr><p:cNvPr id="2" name="Slide Image Placeholder 1"/><p:cNvSpPr><a:spLocks noGrp="1" noRot="1" noChangeAspect="1"/></p:cNvSpPr><p:nvPr><p:ph type="sldImg" idx="2"/></p:nvPr></p:nvSpPr>`+
			`<p:spPr><a:xfrm><a:off x="%d" y="%d"/><a:ext cx="%d" cy="%d"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom><a:noFill/><a:ln w="12700"><a:solidFill><a:prstClr val="black"/></a:solidFill></a:ln></p:spPr></p:sp>`, x, y, w, h) +
		fmt.Sprintf(`<p:sp><p:nvSpPr><p:cNvPr id="3" name="Notes Placeholder 2"/><p:cNvSpPr><a:spLocks noGrp="1"/></p:cNvSpPr><p:nvPr><p:ph type="body" sz="quarter" idx="3"/></p:nvPr></p:nvSpPr>`+
			`<p:spPr><a:xfrm><a:off x="685800" y="%d"/><a:ext cx="5486400" cy="%d"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></p:spPr>`+
			`<p:txBody><a:bodyPr vert="horz" lIns="91440" tIns="45720" rIns="91440" bIns="45720" rtlCol="0"/><a:lstStyle/><a:p><a:endParaRPr lang="en-US"/></a:p></p:txBody></p:sp>`, y+h+228600, 9144000-(y+h+228600)-685800) +
		`</p:spTree></p:cSld>` + clrMap +
		`<p:notesStyle><a:lvl1pPr marL="0" algn="l" defTabSz="914400" rtl="0" eaLnBrk="1" latinLnBrk="0" hangingPunct="1"><a:defRPr sz="1200" kern="1200"><a:solidFill><a:schemeClr val="tx1"/></a:solidFill><a:latin typeface="+mn-lt"/><a:ea typeface="+mn-ea"/><a:cs typeface="+mn-cs"/></a:defRPr></a:lvl1pPr></p:notesStyle></p:notesMaster>`
}

func notesSlideXML(notes string) string {
	var body strings.Builder
	paras := notesParagraphs(notes)
	if len(paras) == 0 {
		body.WriteString(`<a:p><a:endParaRPr lang="en-US"/></a:p>`)
	}
	for _, p := range paras {
		fmt.Fprintf(&body, `<a:p><a:r><a:rPr lang="en-US" dirty="0"/><a:t>%s</a:t></a:r></a:p>`, esc(p))
	}
	return xmlH + `<p:notes ` + pNS + `><p:cSld><p:spTree>` + grpSp +
		`<p:sp><p:nvSpPr><p:cNvPr id="2" name="Slide Image Placeholder 1"/><p:cNvSpPr><a:spLocks noGrp="1" noRot="1" noChangeAspect="1"/></p:cNvSpPr><p:nvPr><p:ph type="sldImg"/></p:nvPr></p:nvSpPr><p:spPr/></p:sp>` +
		`<p:sp><p:nvSpPr><p:cNvPr id="3" name="Notes Placeholder 2"/><p:cNvSpPr><a:spLocks noGrp="1"/></p:cNvSpPr><p:nvPr><p:ph type="body" idx="1"/></p:nvPr></p:nvSpPr><p:spPr/>` +
		`<p:txBody><a:bodyPr/><a:lstStyle/>` + body.String() + `</p:txBody></p:sp>` +
		`</p:spTree></p:cSld><p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr></p:notes>`
}

// notesParagraphs turns notes into paragraphs: one per line of a list, and
// one per run of other lines.
func notesParagraphs(notes string) []string {
	var out []string
	var cur []string
	flush := func() {
		if len(cur) > 0 {
			out = append(out, strings.Join(cur, " "))
			cur = nil
		}
	}
	for _, line := range strings.Split(clean(strings.ReplaceAll(notes, "\r\n", "\n")), "\n") {
		t := strings.TrimSpace(line)
		switch {
		case t == "":
			flush()
		case listItem(t):
			flush()
			out = append(out, t)
		default:
			cur = append(cur, t)
		}
	}
	flush()
	return out
}

func listItem(t string) bool {
	if strings.HasPrefix(t, "- ") || strings.HasPrefix(t, "* ") || strings.HasPrefix(t, "+ ") {
		return true
	}
	i := 0
	for i < len(t) && t[i] >= '0' && t[i] <= '9' {
		i++
	}
	return i > 0 && i+1 < len(t) && (t[i] == '.' || t[i] == ')') && t[i+1] == ' '
}

const themeXML = xmlH + `<a:theme xmlns:a="` + nsA + `" name="inkstudio"><a:themeElements>` +
	`<a:clrScheme name="inkstudio"><a:dk1><a:sysClr val="windowText" lastClr="000000"/></a:dk1><a:lt1><a:sysClr val="window" lastClr="FFFFFF"/></a:lt1>` +
	`<a:dk2><a:srgbClr val="1F2937"/></a:dk2><a:lt2><a:srgbClr val="F3F4F6"/></a:lt2>` +
	`<a:accent1><a:srgbClr val="0F62FE"/></a:accent1><a:accent2><a:srgbClr val="24A148"/></a:accent2><a:accent3><a:srgbClr val="8A3FFC"/></a:accent3>` +
	`<a:accent4><a:srgbClr val="F1C21B"/></a:accent4><a:accent5><a:srgbClr val="DA1E28"/></a:accent5><a:accent6><a:srgbClr val="007D79"/></a:accent6>` +
	`<a:hlink><a:srgbClr val="0F62FE"/></a:hlink><a:folHlink><a:srgbClr val="8A3FFC"/></a:folHlink></a:clrScheme>` +
	`<a:fontScheme name="inkstudio"><a:majorFont><a:latin typeface="Calibri Light"/><a:ea typeface=""/><a:cs typeface=""/></a:majorFont>` +
	`<a:minorFont><a:latin typeface="Calibri"/><a:ea typeface=""/><a:cs typeface=""/></a:minorFont></a:fontScheme>` +
	`<a:fmtScheme name="inkstudio">` +
	`<a:fillStyleLst><a:solidFill><a:schemeClr val="phClr"/></a:solidFill><a:solidFill><a:schemeClr val="phClr"/></a:solidFill><a:solidFill><a:schemeClr val="phClr"/></a:solidFill></a:fillStyleLst>` +
	`<a:lnStyleLst><a:ln w="6350"><a:solidFill><a:schemeClr val="phClr"/></a:solidFill></a:ln><a:ln w="12700"><a:solidFill><a:schemeClr val="phClr"/></a:solidFill></a:ln><a:ln w="19050"><a:solidFill><a:schemeClr val="phClr"/></a:solidFill></a:ln></a:lnStyleLst>` +
	`<a:effectStyleLst><a:effectStyle><a:effectLst/></a:effectStyle><a:effectStyle><a:effectLst/></a:effectStyle><a:effectStyle><a:effectLst/></a:effectStyle></a:effectStyleLst>` +
	`<a:bgFillStyleLst><a:solidFill><a:schemeClr val="phClr"/></a:solidFill><a:solidFill><a:schemeClr val="phClr"/></a:solidFill><a:solidFill><a:schemeClr val="phClr"/></a:solidFill></a:bgFillStyleLst>` +
	`</a:fmtScheme></a:themeElements><a:objectDefaults/><a:extraClrSchemeLst/></a:theme>`
