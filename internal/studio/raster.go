package studio

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"

	"github.com/inkcheck/inkpress/pdf"

	"github.com/inkcheck/inkstudio/platform/slidedoc"
)

//go:embed extract.js
var extractJS string

// browserDeck is a rendered deck open in the Chromium-based browser inkpress
// prints with, showing one slide at a time at the slide's size.
type browserDeck struct {
	ctx    context.Context
	cancel func()
	tmp    string
	// Slides is the number of slides.
	Slides int
	scale  float64
	// extractor is loaded on first use.
	extractor bool
}

// openBrowserDeck loads a rendered deck, scale device pixels per CSS pixel.
func openBrowserDeck(ctx context.Context, html []byte, width, height, scale float64) (*browserDeck, error) {
	browser, err := pdf.FindBrowser()
	if err != nil {
		return nil, fmt.Errorf("exporting slides needs Chrome, Chromium, Edge, Brave or Vivaldi installed, or %s set to one: %w", pdf.BrowserEnv, err)
	}
	tmp, err := os.CreateTemp("", "inkstudio-*.html")
	if err != nil {
		return nil, err
	}
	if _, err := tmp.Write(html); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return nil, err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return nil, err
	}

	ctx, cancelTimeout := context.WithTimeout(ctx, 10*time.Minute)
	ctx, cancelAlloc := chromedp.NewExecAllocator(ctx, append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(browser),
		chromedp.Flag("allow-file-access-from-files", true),
		chromedp.Flag("hide-scrollbars", true),
	)...)
	quiet := func(string, ...any) {}
	ctx, cancelTab := chromedp.NewContext(ctx, chromedp.WithErrorf(quiet), chromedp.WithLogf(quiet))
	b := &browserDeck{ctx: ctx, tmp: tmp.Name(), scale: scale, cancel: func() {
		cancelTab()
		cancelAlloc()
		cancelTimeout()
		os.Remove(tmp.Name())
	}}

	u := "file://" + filepath.ToSlash(tmp.Name())
	if runtime.GOOS == "windows" {
		u = "file:///" + filepath.ToSlash(tmp.Name())
	}
	w, h := int64(math.Round(width)), int64(math.Round(height))
	// One slide fills the viewport at a time.
	const setup = `(() => {
		const s = document.createElement('style');
		s.textContent = 'html, body { margin: 0 !important; padding: 0 !important; background: none !important; overflow: hidden !important; }' +
			' .page { display: none !important; margin: 0 !important; box-shadow: none !important; }' +
			' .page.raster-current { display: block !important; }';
		document.head.appendChild(s);
		return document.fonts.ready.then(() => document.querySelectorAll('.page').length);
	})()`
	if err := chromedp.Do(ctx,
		chromedp.EmulateViewport(w, h, chromedp.EmulateScale(scale)),
		chromedp.Navigate(u),
	); err != nil {
		b.Close()
		return nil, fmt.Errorf("render slides with %s: %w", browser, err)
	}
	n, err := chromedp.Run(ctx, chromedp.Evaluate[int](setup, chromedp.EvalAwaitPromise))
	if err != nil {
		b.Close()
		return nil, fmt.Errorf("render slides with %s: %w", browser, err)
	}
	b.Slides = n
	return b, nil
}

// Close shuts the browser down.
func (b *browserDeck) Close() { b.cancel() }

// show shows slide i, from 0, once its images have decoded.
func (b *browserDeck) show(i int) error {
	js := fmt.Sprintf(`(() => {
		document.querySelectorAll('.page').forEach((p, i) => p.classList.toggle('raster-current', i === %d));
		return Promise.all([...document.querySelectorAll('.page.raster-current img')].map(img => img.decode().catch(() => {}))).then(() => true);
	})()`, i)
	_, err := chromedp.Run(b.ctx, chromedp.Evaluate[bool](js, chromedp.EvalAwaitPromise))
	return err
}

// Picture is slide i as a PNG.
func (b *browserDeck) Picture(i int) ([]byte, error) {
	if err := b.show(i); err != nil {
		return nil, err
	}
	png, err := chromedp.Run(b.ctx, chromedp.CaptureScreenshot())
	if err != nil {
		return nil, fmt.Errorf("slide %d: %w", i+1, err)
	}
	return png, nil
}

// Parts is slide i as editable parts, with its pictures captured.
func (b *browserDeck) Parts(i int) (*slidedoc.Slide, error) {
	if err := b.show(i); err != nil {
		return nil, err
	}
	if !b.extractor {
		if _, err := chromedp.Run(b.ctx, chromedp.Evaluate[bool](extractJS+";true")); err != nil {
			return nil, fmt.Errorf("load the extractor: %w", err)
		}
		b.extractor = true
	}
	raw, err := chromedp.Run(b.ctx, chromedp.Evaluate[string](fmt.Sprintf("inkExtract(%d)", i)))
	if err != nil {
		return nil, fmt.Errorf("slide %d: %w", i+1, err)
	}
	var s slidedoc.Slide
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		return nil, fmt.Errorf("slide %d: %w", i+1, err)
	}
	s.Normalize()

	// Pictures are captured alone, on a transparent background.
	if err := chromedp.Do(b.ctx, chromedp.Action[chromedp.Void](func(ctx context.Context, t *chromedp.Target) (chromedp.Void, error) {
		_, err := cdp.Call(ctx, t, emulation.SetDefaultBackgroundColorOverride,
			// cdp drops an alpha of 0, which then means opaque; this one
			// rounds to fully transparent.
			emulation.SetDefaultBackgroundColorOverrideParams{Color: &cdp.RGBA{A: 0.001}})
		return chromedp.Void{}, err
	})); err != nil {
		return nil, err
	}
	defer b.capture("", 0)
	if s.BackgroundCap != 0 {
		w, h := b.slideSize()
		if s.BackgroundPNG, err = b.captureBox(s.BackgroundCap, "bg", 0, 0, w, h); err != nil {
			return nil, err
		}
	}
	for k := range s.Items {
		it := &s.Items[k]
		if it.Kind != slidedoc.KindImage || it.Cap == 0 {
			continue
		}
		mode := "el"
		if raw, _ := chromedp.Run(b.ctx, chromedp.Evaluate[string](fmt.Sprintf(
			`document.querySelector('[data-ink-cap^="%d:"]')?.getAttribute('data-ink-cap') ?? ''`, it.Cap))); strings.HasSuffix(raw, ":bg") {
			mode = "bg"
		}
		if it.PNG, err = b.captureBox(it.Cap, mode, it.X, it.Y, it.W, it.H); err != nil {
			return nil, err
		}
	}
	return &s, nil
}

func (b *browserDeck) slideSize() (float64, float64) {
	sz, err := chromedp.Run(b.ctx, chromedp.Evaluate[[]float64](
		`(() => { const r = document.querySelector('.page.raster-current').getBoundingClientRect(); return [r.width, r.height]; })()`))
	if err != nil || len(sz) != 2 {
		return 0, 0
	}
	return sz[0], sz[1]
}

// capture hides everything on the slide but capture id: the element and
// what is in it ("el"), or only its own background ("bg"). id 0 shows the
// slide again.
func (b *browserDeck) capture(mode string, id int) error {
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
	js := fmt.Sprintf(`(() => {
		let s = document.getElementById('ink-capture');
		if (!s) { s = document.createElement('style'); s.id = 'ink-capture'; document.head.appendChild(s); }
		s.textContent = %q;
		return true;
	})()`, css)
	_, err := chromedp.Run(b.ctx, chromedp.Evaluate[bool](js))
	return err
}

func (b *browserDeck) captureBox(id int, mode string, x, y, w, h float64) ([]byte, error) {
	if err := b.capture(mode, id); err != nil {
		return nil, err
	}
	// Whole pixels, covering the box.
	x0, y0 := math.Floor(x), math.Floor(y)
	clip := &page.Viewport{X: x0, Y: y0, Width: math.Ceil(x+w) - x0, Height: math.Ceil(y+h) - y0, Scale: 1}
	if clip.Width < 1 || clip.Height < 1 {
		return nil, nil
	}
	return chromedp.Run(b.ctx, chromedp.Action[[]byte](func(ctx context.Context, t *chromedp.Target) ([]byte, error) {
		r, err := cdp.Call(ctx, t, page.CaptureScreenshot, page.CaptureScreenshotParams{
			Format: page.CaptureScreenshotFormatPng, Clip: clip, FromSurface: ptrBool(true),
		})
		if err != nil {
			return nil, err
		}
		return r.Data, nil
	}))
}

func ptrBool(v bool) *bool { return &v }
