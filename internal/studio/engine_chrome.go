package studio

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"

	"github.com/inkcheck/inkpress/pdf"
)

// chromeEngine drives an installed Chromium-based browser over the
// DevTools protocol.
type chromeEngine struct {
	ctx    context.Context
	cancel func()
	tmp    string
	// transparent is the page's current default background.
	transparent bool
}

func openChrome(ctx context.Context, html []byte, width, height, scale float64) (engine, error) {
	browser, err := pdf.FindBrowser()
	if err != nil {
		return nil, fmt.Errorf("exporting needs Chrome, Chromium, Edge, Brave or Vivaldi installed, or %s set to one: %w", pdf.BrowserEnv, err)
	}
	tmp, err := writeTemp(os.TempDir(), html)
	if err != nil {
		return nil, err
	}
	ctx, cancelAlloc := chromedp.NewExecAllocator(ctx, append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(browser),
		chromedp.Flag("allow-file-access-from-files", true),
		chromedp.Flag("hide-scrollbars", true),
	)...)
	quiet := func(string, ...any) {}
	ctx, cancelTab := chromedp.NewContext(ctx, chromedp.WithErrorf(quiet), chromedp.WithLogf(quiet))
	e := &chromeEngine{ctx: ctx, tmp: tmp, cancel: func() { cancelTab(); cancelAlloc(); os.Remove(tmp) }}
	u := "file://" + filepath.ToSlash(tmp)
	if runtime.GOOS == "windows" {
		u = "file:///" + filepath.ToSlash(tmp)
	}
	if err := chromedp.Do(ctx,
		chromedp.EmulateViewport(int64(math.Round(width)), int64(math.Round(height)), chromedp.EmulateScale(scale)),
		chromedp.Navigate(u),
	); err != nil {
		e.close()
		return nil, fmt.Errorf("render slides with %s: %w", browser, err)
	}
	return e, nil
}

func (e *chromeEngine) eval(body string) (string, error) {
	return chromedp.Run(e.ctx, chromedp.Evaluate[string]("(async () => {"+body+"\n})()", chromedp.EvalAwaitPromise))
}

func (e *chromeEngine) clip(x, y, w, h float64, transparent bool) ([]byte, error) {
	if transparent != e.transparent {
		params := emulation.SetDefaultBackgroundColorOverrideParams{}
		if transparent {
			// cdp drops an alpha of 0, which then means opaque; this one
			// rounds to fully transparent.
			params.Color = &cdp.RGBA{A: 0.001}
		}
		if err := chromedp.Do(e.ctx, chromedp.Action[chromedp.Void](func(ctx context.Context, t *chromedp.Target) (chromedp.Void, error) {
			_, err := cdp.Call(ctx, t, emulation.SetDefaultBackgroundColorOverride, params)
			return chromedp.Void{}, err
		})); err != nil {
			return nil, err
		}
		e.transparent = transparent
	}
	clip := &page.Viewport{X: x, Y: y, Width: w, Height: h, Scale: 1}
	return chromedp.Run(e.ctx, chromedp.Action[[]byte](func(ctx context.Context, t *chromedp.Target) ([]byte, error) {
		fromSurface := true
		r, err := cdp.Call(ctx, t, page.CaptureScreenshot, page.CaptureScreenshotParams{
			Format: page.CaptureScreenshotFormatPng, Clip: clip, FromSurface: &fromSurface,
		})
		if err != nil {
			return nil, err
		}
		return r.Data, nil
	}))
}

func (e *chromeEngine) pdfPage() ([]byte, error) { return nil, errNoPDFPage }

func (e *chromeEngine) close() { e.cancel() }
