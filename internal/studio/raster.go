package studio

import (
	"context"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"strconv"
	"time"

	"github.com/inkcheck/inkstudio/platform/slidedoc"
)

//go:embed extract.js
var extractJS string

// Engines that render slides for export.
const (
	// EngineWebKit uses the system's WebKit, which the macOS app draws with,
	// so exports match the preview and need nothing installed.
	EngineWebKit = "webkit"
	// EngineChrome uses an installed Chromium-based browser, as inkpress
	// does; for builds without WebKit, such as the browser build.
	EngineChrome = "chrome"
)

// Engine is the engine exports use. The macOS app sets it to EngineWebKit.
var Engine = EngineChrome

// engine is a browser showing a rendered deck at the slide's size.
type engine interface {
	// eval runs body as the body of an async function, and returns the
	// string it returns.
	eval(body string) (string, error)
	// clip is a PNG of a rectangle of the page, on white unless
	// transparent.
	clip(x, y, w, h float64, transparent bool) ([]byte, error)
	// pdfPage prints the slide shown as one PDF page.
	pdfPage() ([]byte, error)
	close()
}

var errNoPDFPage = errors.New("this engine prints whole documents")

// deckSession is a rendered deck open in an engine, one slide at a time.
type deckSession struct {
	e             engine
	Slides        int
	width, height float64
	extractor     bool
}

// openDeck loads a rendered deck into the export engine, scale device
// pixels per CSS pixel for pictures, or for printing when print is set. dir
// is the pack, which the page reads fonts and images from.
func openDeck(ctx context.Context, html []byte, dir string, width, height, scale float64, print bool) (*deckSession, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	var (
		e   engine
		err error
	)
	if Engine == EngineWebKit {
		e, err = openWebKit(ctx, html, dir, width, height, scale, print)
	} else {
		e, err = openChrome(ctx, html, width, height, scale)
	}
	if err != nil {
		cancel()
		return nil, err
	}
	s := &deckSession{e: closer{e, cancel}, width: width, height: height}
	// One slide fills the viewport at a time.
	out, err := e.eval(`
		const s = document.createElement('style');
		s.textContent = 'html, body { margin: 0 !important; padding: 0 !important; background: none !important; overflow: hidden !important; }' +
			' .page { display: none !important; margin: 0 !important; box-shadow: none !important; }' +
			' .page.raster-current { display: block !important; }';
		document.head.appendChild(s);
		await document.fonts.ready;
		return String(document.querySelectorAll('.page').length);`)
	if err == nil {
		s.Slides, err = strconv.Atoi(out)
	}
	if err != nil {
		s.Close()
		return nil, fmt.Errorf("render slides: %w", err)
	}
	return s, nil
}

// closer closes the engine and ends its context.
type closer struct {
	engine
	cancel func()
}

func (c closer) close() { c.engine.close(); c.cancel() }

// Close shuts the engine down.
func (s *deckSession) Close() { s.e.close() }

// show shows slide i, from 0, once its images have decoded. It does not
// wait for a frame: WebKit runs no animation frames in a view nobody sees,
// and both engines draw the page before they take a picture of it.
func (s *deckSession) show(i int) error {
	_, err := s.e.eval(fmt.Sprintf(`
		document.querySelectorAll('.page').forEach((p, i) => p.classList.toggle('raster-current', i === %d));
		await Promise.all([...document.querySelectorAll('.page.raster-current img')].map(img => img.decode().catch(() => {})));
		return '';`, i))
	return err
}

// Picture is slide i as a PNG.
func (s *deckSession) Picture(i int) ([]byte, error) {
	if err := s.show(i); err != nil {
		return nil, err
	}
	png, err := s.e.clip(0, 0, s.width, s.height, false)
	if err != nil {
		return nil, fmt.Errorf("slide %d: %w", i+1, err)
	}
	return png, nil
}

// PDF prints every slide, one per page.
func (s *deckSession) PDF() ([]byte, error) {
	var pages [][]byte
	for i := 0; i < s.Slides; i++ {
		if err := s.show(i); err != nil {
			return nil, err
		}
		if err := s.flattenMasks(); err != nil {
			return nil, err
		}
		p, err := s.e.pdfPage()
		if err != nil {
			return nil, fmt.Errorf("slide %d: %w", i+1, err)
		}
		pages = append(pages, p)
	}
	return mergePDF(pages)
}

// flattenMasks replaces each masked element on the slide shown, such as a
// logo, with a picture of itself: WebKit's PDFs draw masks as solid boxes.
func (s *deckSession) flattenMasks() error {
	raw, err := s.e.eval(`
		const page = document.querySelector('.page.raster-current');
		const origin = page.getBoundingClientRect();
		const out = [];
		page.querySelectorAll('*').forEach(el => {
			const cs = getComputedStyle(el);
			const m = cs.maskImage || cs.webkitMaskImage;
			if (!m || m === 'none' || el.hasAttribute('data-ink-flat')) return;
			const r = el.getBoundingClientRect();
			if (r.width < 1 || r.height < 1) return;
			const id = 100000 + out.length;
			el.setAttribute('data-ink-cap', id + ':el');
			out.push([id, r.left - origin.left, r.top - origin.top, r.width, r.height]);
		});
		return JSON.stringify(out);`)
	if err != nil {
		return err
	}
	var masked [][5]float64
	if err := json.Unmarshal([]byte(raw), &masked); err != nil {
		return err
	}
	for _, m := range masked {
		id := int(m[0])
		// Whole pixels, as captureBox takes them.
		x0, y0 := math.Floor(m[1]), math.Floor(m[2])
		w, h := math.Ceil(m[1]+m[3])-x0, math.Ceil(m[2]+m[4])-y0
		png, err := s.captureBox(id, "el", m[1], m[2], m[3], m[4])
		if err != nil {
			return err
		}
		if png == nil {
			continue
		}
		// The picture covers the pixel-aligned box; place it the same way.
		if _, err := s.e.eval(fmt.Sprintf(`
			const el = document.querySelector('.page.raster-current [data-ink-cap^="%d:"]');
			const r = el.getBoundingClientRect();
			el.setAttribute('data-ink-flat', '');
			el.style.setProperty('mask', 'none', 'important');
			el.style.setProperty('-webkit-mask', 'none', 'important');
			el.style.setProperty('background', 'url("data:image/png;base64,%s") no-repeat', 'important');
			el.style.setProperty('background-size', '%gpx %gpx', 'important');
			el.style.setProperty('background-position', (%g - (r.left - el.closest('.page').getBoundingClientRect().left)) + 'px ' + (%g - (r.top - el.closest('.page').getBoundingClientRect().top)) + 'px', 'important');
			return '';`, id, base64.StdEncoding.EncodeToString(png), w, h, x0, y0)); err != nil {
			return err
		}
	}
	return s.capture("", 0)
}

// Parts is slide i as editable parts, with its pictures captured.
func (s *deckSession) Parts(i int) (*slidedoc.Slide, error) {
	if err := s.show(i); err != nil {
		return nil, err
	}
	if !s.extractor {
		if _, err := s.e.eval(extractJS + "; return '';"); err != nil {
			return nil, fmt.Errorf("load the extractor: %w", err)
		}
		s.extractor = true
	}
	raw, err := s.e.eval(fmt.Sprintf("return inkExtract(%d);", i))
	if err != nil {
		return nil, fmt.Errorf("slide %d: %w", i+1, err)
	}
	var sl slidedoc.Slide
	if err := json.Unmarshal([]byte(raw), &sl); err != nil {
		return nil, fmt.Errorf("slide %d: %w", i+1, err)
	}
	sl.Normalize()

	defer s.capture("", 0)
	if sl.BackgroundCap != 0 {
		if sl.BackgroundPNG, err = s.captureBox(sl.BackgroundCap, "bg", 0, 0, s.width, s.height); err != nil {
			return nil, err
		}
	}
	for k := range sl.Items {
		it := &sl.Items[k]
		if it.Kind != slidedoc.KindImage || it.Cap == 0 {
			continue
		}
		mode, _ := s.e.eval(fmt.Sprintf(
			`return (document.querySelector('.page.raster-current [data-ink-cap^="%d:"]')?.getAttribute('data-ink-cap') ?? '').split(':')[1] ?? '';`, it.Cap))
		if it.PNG, err = s.captureBox(it.Cap, mode, it.X, it.Y, it.W, it.H); err != nil {
			return nil, err
		}
	}
	return &sl, nil
}

// capture hides everything on the slide but capture id: the element and
// what is in it ("el"), or only its own background ("bg"). id 0 shows the
// slide again.
func (s *deckSession) capture(mode string, id int) error {
	css := ""
	if id != 0 {
		// More specific than the rule that hides the rest, so it wins.
		sel := `.page.raster-current [data-ink-cap^="` + strconv.Itoa(id) + `:"]`
		css = `.page.raster-current, .page.raster-current * { visibility: hidden !important; }`
		if mode == "bg" {
			css += sel + ` { visibility: visible !important; }`
		} else {
			css += sel + `, ` + sel + ` * { visibility: visible !important; }`
		}
	}
	_, err := s.e.eval(fmt.Sprintf(`
		let s = document.getElementById('ink-capture');
		if (!s) { s = document.createElement('style'); s.id = 'ink-capture'; document.head.appendChild(s); }
		s.textContent = %q;
		return '';`, css))
	return err
}

func (s *deckSession) captureBox(id int, mode string, x, y, w, h float64) ([]byte, error) {
	if err := s.capture(mode, id); err != nil {
		return nil, err
	}
	// Whole pixels, covering the box.
	x0, y0 := math.Floor(x), math.Floor(y)
	cw, ch := math.Ceil(x+w)-x0, math.Ceil(y+h)-y0
	if cw < 1 || ch < 1 {
		return nil, nil
	}
	return s.e.clip(x0, y0, cw, ch, true)
}

// writeTemp writes html to a hidden file in dir, so a page loaded from it
// may read the pack's files.
func writeTemp(dir string, html []byte) (string, error) {
	tmp, err := os.CreateTemp(dir, ".inkstudio-export-*.html")
	if err != nil {
		return "", err
	}
	if _, err := tmp.Write(html); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return "", err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return "", err
	}
	return tmp.Name(), nil
}
