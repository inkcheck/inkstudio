package studio

import (
	"context"
	"fmt"
	"os"

	"github.com/inkcheck/inkstudio/platform/webkit"
)

// webkitEngine drives an offscreen view of the system's WebKit.
type webkitEngine struct {
	ctx   context.Context
	view  *webkit.View
	tmp   string
	scale float64
}

func openWebKit(ctx context.Context, html []byte, dir string, width, height, scale float64, print bool) (engine, error) {
	if !webkit.Available {
		return nil, fmt.Errorf("WebKit is not available here")
	}
	// The page reads the pack's fonts and images, so it is loaded from the
	// pack, which WebKit then lets it read.
	tmp, err := writeTemp(dir, html)
	if err != nil {
		return nil, err
	}
	// Printing puts 96 CSS pixels to the inch, as browsers and inkpress do,
	// so an A4 slide prints as A4.
	zoom := 1.0
	if print {
		zoom = 0.75
	}
	e := &webkitEngine{ctx: ctx, view: webkit.New(width, height, zoom), tmp: tmp, scale: scale}
	if err := e.view.Load(ctx, tmp, dir); err != nil {
		e.close()
		return nil, fmt.Errorf("render slides with WebKit: %w", err)
	}
	return e, nil
}

func (e *webkitEngine) eval(body string) (string, error) { return e.view.Eval(e.ctx, body) }

func (e *webkitEngine) clip(x, y, w, h float64, transparent bool) ([]byte, error) {
	return e.view.Snapshot(e.ctx, x, y, w, h, e.scale, transparent)
}

func (e *webkitEngine) pdfPage() ([]byte, error) {
	w, h, err := e.pageSize()
	if err != nil {
		return nil, err
	}
	return e.view.PDF(e.ctx, w, h)
}

func (e *webkitEngine) pageSize() (float64, float64, error) {
	out, err := e.eval(`const r = document.querySelector('.page.raster-current').getBoundingClientRect(); return r.width + ' ' + r.height;`)
	if err != nil {
		return 0, 0, err
	}
	var w, h float64
	if _, err := fmt.Sscan(out, &w, &h); err != nil {
		return 0, 0, err
	}
	return w, h, nil
}

func (e *webkitEngine) close() {
	e.view.Close()
	os.Remove(e.tmp)
}

func mergePDF(pages [][]byte) ([]byte, error) { return webkit.MergePDF(pages) }
