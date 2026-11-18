package studio

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/inkcheck/inkpress/pdf"
)

// rasterize renders each slide of a rendered deck to a PNG, scale times its
// size in CSS pixels, with the Chromium-based browser inkpress prints with.
// progress, when set, is called after each slide.
func rasterize(ctx context.Context, html []byte, width, height, scale float64, progress func(done, total int)) ([][]byte, error) {
	browser, err := pdf.FindBrowser()
	if err != nil {
		return nil, fmt.Errorf("exporting slides as images needs Chrome, Chromium, Edge, Brave or Vivaldi installed, or %s set to one: %w", pdf.BrowserEnv, err)
	}
	tmp, err := os.CreateTemp("", "inkstudio-*.html")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(html); err != nil {
		tmp.Close()
		return nil, err
	}
	if err := tmp.Close(); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	ctx, cancelAlloc := chromedp.NewExecAllocator(ctx, append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(browser),
		chromedp.Flag("allow-file-access-from-files", true),
		chromedp.Flag("hide-scrollbars", true),
	)...)
	defer cancelAlloc()
	quiet := func(string, ...any) {}
	ctx, cancelTab := chromedp.NewContext(ctx, chromedp.WithErrorf(quiet), chromedp.WithLogf(quiet))
	defer cancelTab()

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
		return nil, fmt.Errorf("render slides with %s: %w", browser, err)
	}
	total, err := chromedp.Run(ctx, chromedp.Evaluate[int](setup, chromedp.EvalAwaitPromise))
	if err != nil {
		return nil, fmt.Errorf("render slides with %s: %w", browser, err)
	}

	images := make([][]byte, 0, total)
	for i := 0; i < total; i++ {
		show := fmt.Sprintf(`(() => {
			document.querySelectorAll('.page').forEach((p, i) => p.classList.toggle('raster-current', i === %d));
			// Let images in the slide finish decoding before the capture.
			return Promise.all([...document.querySelectorAll('.page.raster-current img')].map(img => img.decode().catch(() => {}))).then(() => true);
		})()`, i)
		if _, err := chromedp.Run(ctx, chromedp.Evaluate[bool](show, chromedp.EvalAwaitPromise)); err != nil {
			return nil, err
		}
		png, err := chromedp.Run(ctx, chromedp.CaptureScreenshot())
		if err != nil {
			return nil, fmt.Errorf("slide %d: %w", i+1, err)
		}
		images = append(images, png)
		if progress != nil {
			progress(i+1, total)
		}
	}
	return images, nil
}
