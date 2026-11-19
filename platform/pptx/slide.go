package pptx

import (
	"fmt"
	"math"
	"strings"

	"github.com/inkcheck/inkstudio/platform/slidedoc"
)

// slideWriter writes one slide's parts as DrawingML shapes, and collects
// the pictures and links they refer to.
type slideWriter struct {
	slide int
	// scale shrinks the slide when PowerPoint's size limit did.
	scale float64
	// width and height are the slide's size in CSS pixels.
	width, height float64
	rels          []rel
	media         [][]byte
	b             strings.Builder
	id            int
}

// emu converts CSS pixels to EMUs.
func (w *slideWriter) emu(px float64) int64 { return int64(math.Round(px * emuPerPx * w.scale)) }

// pt100 converts CSS pixels to hundredths of a point, for type sizes and
// spacing.
func (w *slideWriter) pt100(px float64) int64 { return int64(math.Round(px * 0.75 * 100 * w.scale)) }

func (w *slideWriter) nextID() int { w.id++; return w.id }

func (w *slideWriter) rel(typ, target string, external bool) string {
	id := fmt.Sprintf("rId%d", len(w.rels)+3) // rId1 and rId2 are the layout and notes
	w.rels = append(w.rels, rel{id, typ, target, external})
	return id
}

func (w *slideWriter) write(s *slidedoc.Slide) string {
	w.id = 1
	w.b.WriteString(xmlH + `<p:sld ` + pNS + `><p:cSld>`)
	if s.Background != nil {
		w.b.WriteString(`<p:bg><p:bgPr>` + fill(s.Background) + `<a:effectLst/></p:bgPr></p:bg>`)
	}
	w.b.WriteString(`<p:spTree>` + grpSp)
	if s.BackgroundPNG != nil {
		w.picture(slidedoc.Item{Kind: slidedoc.KindImage, PNG: s.BackgroundPNG}, true)
	}
	for _, it := range s.Items {
		switch it.Kind {
		case slidedoc.KindRect:
			w.rect(it)
		case slidedoc.KindImage:
			if it.PNG != nil {
				w.picture(it, false)
			}
		case slidedoc.KindText:
			w.text(it)
		}
	}
	w.b.WriteString(`</p:spTree></p:cSld><p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr></p:sld>`)
	return w.b.String()
}

func (w *slideWriter) xfrm(it slidedoc.Item) string {
	return fmt.Sprintf(`<a:xfrm><a:off x="%d" y="%d"/><a:ext cx="%d" cy="%d"/></a:xfrm>`,
		w.emu(it.X), w.emu(it.Y), max(w.emu(it.W), 1), max(w.emu(it.H), 1))
}

func clr(c *slidedoc.Color) string {
	hex := fmt.Sprintf("%02X%02X%02X", c.R, c.G, c.B)
	if c.A >= 0.999 {
		return `<a:srgbClr val="` + hex + `"/>`
	}
	return fmt.Sprintf(`<a:srgbClr val="%s"><a:alpha val="%d"/></a:srgbClr>`, hex, int(math.Round(c.A*100000)))
}

func fill(c *slidedoc.Color) string {
	if c == nil {
		return `<a:noFill/>`
	}
	return `<a:solidFill>` + clr(c) + `</a:solidFill>`
}

func (w *slideWriter) rect(it slidedoc.Item) {
	id := w.nextID()
	geom := `<a:prstGeom prst="rect"><a:avLst/></a:prstGeom>`
	if it.Radius > 0 {
		// The corner radius as a share of the shorter side, which PowerPoint
		// caps at half.
		adj := min(50000, int(math.Round(it.Radius/math.Min(it.W, it.H)*100000)))
		geom = fmt.Sprintf(`<a:prstGeom prst="roundRect"><a:avLst><a:gd name="adj" fmla="val %d"/></a:avLst></a:prstGeom>`, adj)
	}
	line := `<a:ln><a:noFill/></a:ln>`
	if it.Line != nil && it.LineWidth > 0 {
		line = fmt.Sprintf(`<a:ln w="%d">%s</a:ln>`, w.emu(it.LineWidth), fill(it.Line))
	}
	fmt.Fprintf(&w.b, `<p:sp><p:nvSpPr><p:cNvPr id="%d" name="Shape %d"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>`+
		`<p:spPr>%s%s%s%s</p:spPr></p:sp>`, id, id, w.xfrm(it), geom, fill(it.Fill), line)
}

func (w *slideWriter) picture(it slidedoc.Item, full bool) {
	id := w.nextID()
	w.media = append(w.media, it.PNG)
	rid := w.rel(relNS+"/image", fmt.Sprintf("../media/image%d-%d.png", w.slide, len(w.media)), false)
	if full {
		it.X, it.Y, it.W, it.H = 0, 0, w.width, w.height
	}
	off := w.xfrm(it)
	fmt.Fprintf(&w.b, `<p:pic><p:nvPicPr><p:cNvPr id="%d" name="Picture %d" descr="%s"/><p:cNvPicPr><a:picLocks noChangeAspect="1"/></p:cNvPicPr><p:nvPr/></p:nvPicPr>`+
		`<p:blipFill><a:blip r:embed="%s"/><a:stretch><a:fillRect/></a:stretch></p:blipFill>`+
		`<p:spPr>%s<a:prstGeom prst="rect"><a:avLst/></a:prstGeom></p:spPr></p:pic>`, id, id, esc(clean(it.Alt)), rid, off)
}

func (w *slideWriter) text(it slidedoc.Item) {
	var body strings.Builder
	for _, p := range it.Paras {
		body.WriteString(w.para(p))
	}
	if body.Len() == 0 {
		return
	}
	id := w.nextID()
	wrap := "square"
	if it.NoWrap {
		wrap = "none"
	}
	fmt.Fprintf(&w.b, `<p:sp><p:nvSpPr><p:cNvPr id="%d" name="Text %d"/><p:cNvSpPr txBox="1"/><p:nvPr/></p:nvSpPr>`+
		`<p:spPr>%s<a:prstGeom prst="rect"><a:avLst/></a:prstGeom><a:noFill/></p:spPr>`+
		`<p:txBody><a:bodyPr wrap="%s" lIns="0" tIns="0" rIns="0" bIns="0" rtlCol="0" anchor="t"><a:noAutofit/></a:bodyPr><a:lstStyle/>%s</p:txBody></p:sp>`,
		id, id, w.xfrm(it), wrap, body.String())
}

func (w *slideWriter) para(p slidedoc.Para) string {
	var b strings.Builder
	align := p.Align
	if align == "" {
		align = "l"
	}
	attrs := fmt.Sprintf(` algn="%s"`, align)
	if p.Indent > 0 {
		attrs += fmt.Sprintf(` marL="%d" indent="%d"`, w.emu(p.Indent), -w.emu(p.Indent))
	}
	fmt.Fprintf(&b, `<a:p><a:pPr%s>`, attrs)
	if p.LineHeight > 0 {
		fmt.Fprintf(&b, `<a:lnSpc><a:spcPts val="%d"/></a:lnSpc>`, min(w.pt100(p.LineHeight), 158400))
	}
	if p.Before > 0 {
		fmt.Fprintf(&b, `<a:spcBef><a:spcPts val="%d"/></a:spcBef>`, min(w.pt100(p.Before), 158400))
	}
	switch {
	case p.Bullet == "1.":
		start := ""
		if p.Start > 1 {
			start = fmt.Sprintf(` startAt="%d"`, p.Start)
		}
		b.WriteString(w.bulletColor(p) + `<a:buAutoNum type="arabicPeriod"` + start + `/>`)
	case p.Bullet != "":
		b.WriteString(w.bulletColor(p) + `<a:buFont typeface="Arial"/><a:buChar char="` + esc(p.Bullet) + `"/>`)
	default:
		b.WriteString(`<a:buNone/>`)
	}
	b.WriteString(`</a:pPr>`)
	var last *slidedoc.Run
	for i := range p.Runs {
		r := &p.Runs[i]
		if r.Break {
			fmt.Fprintf(&b, `<a:br><a:rPr lang="en-US" sz="%d"/></a:br>`, w.size(r.Size))
			continue
		}
		if r.Text == "" {
			continue
		}
		fmt.Fprintf(&b, `<a:r>%s<a:t>%s</a:t></a:r>`, w.rPr(r), esc(clean(r.Text)))
		last = r
	}
	if last != nil {
		fmt.Fprintf(&b, `<a:endParaRPr lang="en-US" sz="%d"/>`, w.size(last.Size))
	}
	b.WriteString(`</a:p>`)
	return b.String()
}

// bulletColor gives the bullet its text's colour.
func (w *slideWriter) bulletColor(p slidedoc.Para) string {
	for _, r := range p.Runs {
		if r.Color != nil && !r.Break {
			return `<a:buClr>` + clr(r.Color) + `</a:buClr>`
		}
	}
	return ""
}

func (w *slideWriter) size(px float64) int64 {
	return min(max(w.pt100(px), 100), 400000)
}

func (w *slideWriter) rPr(r *slidedoc.Run) string {
	attrs := fmt.Sprintf(` lang="en-US" sz="%d"`, w.size(r.Size))
	if r.Bold {
		attrs += ` b="1"`
	}
	if r.Italic {
		attrs += ` i="1"`
	}
	if r.Underline {
		attrs += ` u="sng"`
	}
	if r.Strike {
		attrs += ` strike="sngStrike"`
	}
	if r.Spacing != 0 {
		attrs += fmt.Sprintf(` spc="%d"`, w.pt100(r.Spacing))
	}
	attrs += ` dirty="0"`
	var b strings.Builder
	b.WriteString(`<a:rPr` + attrs + `>`)
	if r.Color != nil {
		b.WriteString(fill(r.Color))
	}
	font := esc(r.Font)
	fmt.Fprintf(&b, `<a:latin typeface="%s"/><a:ea typeface="%s"/><a:cs typeface="%s"/>`, font, font, font)
	if r.Href != "" {
		rid := w.rel(relNS+"/hyperlink", r.Href, true)
		fmt.Fprintf(&b, `<a:hlinkClick r:id="%s"/>`, rid)
	}
	b.WriteString(`</a:rPr>`)
	return b.String()
}
