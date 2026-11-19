//go:build darwin

// Package webkit drives an offscreen WebKit view, the engine macOS apps
// draw web pages with: load a page, run JavaScript, take pictures of parts
// of it, and print it to PDF. It needs the main thread's run loop to be
// running, as it is in an app; tests can run one with RunMainLoop.
package webkit

/*
#cgo CFLAGS: -x objective-c -fobjc-arc -mmacosx-version-min=13.0
#cgo LDFLAGS: -framework Cocoa -framework WebKit -framework PDFKit
#include <stdlib.h>
#include "webkit_darwin.h"
*/
import "C"

import (
	"bytes"
	"context"
	"errors"
	"image/png"
	"runtime/cgo"
	"unsafe"
)

// Available reports whether this platform has WebKit.
const Available = true

// View is an offscreen web view of a fixed size, in CSS pixels.
type View struct {
	p unsafe.Pointer
}

type result struct {
	data []byte
	err  error
}

//export inkWKDone
func inkWKDone(h C.uintptr_t, data unsafe.Pointer, n C.int, err *C.char) {
	ch := cgo.Handle(h).Value().(chan result)
	r := result{}
	if data != nil && n > 0 {
		r.data = C.GoBytes(data, n)
	}
	if err != nil {
		r.err = errors.New(C.GoString(err))
	}
	ch <- r
}

// call starts f with a handle for its result, and waits for it.
func call(ctx context.Context, f func(h C.uintptr_t)) ([]byte, error) {
	ch := make(chan result, 1)
	h := cgo.NewHandle(ch)
	f(C.uintptr_t(h))
	select {
	case r := <-ch:
		h.Delete()
		return r.data, r.err
	case <-ctx.Done():
		// The handle stays, for a result that may still come.
		return nil, ctx.Err()
	}
}

// New makes a view width by height CSS pixels, drawn zoom points per CSS
// pixel. Zoom only changes PDFs: a zoom of 0.75 prints 96 CSS pixels to the
// inch, as browsers do, where 1 prints one point per pixel.
func New(width, height, zoom float64) *View {
	if zoom <= 0 {
		zoom = 1
	}
	return &View{p: C.ink_wk_new(C.double(width), C.double(height), C.double(zoom))}
}

// Load loads the HTML file at file, letting it read files under readAccess.
func (v *View) Load(ctx context.Context, file, readAccess string) error {
	f, r := C.CString(file), C.CString(readAccess)
	defer C.free(unsafe.Pointer(f))
	defer C.free(unsafe.Pointer(r))
	_, err := call(ctx, func(h C.uintptr_t) { C.ink_wk_load(v.p, f, r, h) })
	return err
}

// Eval runs body as the body of an async function and returns its result,
// which must be a string.
func (v *View) Eval(ctx context.Context, body string) (string, error) {
	b := C.CString(body)
	defer C.free(unsafe.Pointer(b))
	out, err := call(ctx, func(h C.uintptr_t) { C.ink_wk_eval(v.p, b, h) })
	return string(out), err
}

// Snapshot returns a PNG of the rectangle x, y, w, h, scale pixels per CSS
// pixel. Unless transparent, it is drawn on white.
func (v *View) Snapshot(ctx context.Context, x, y, w, h, scale float64, transparent bool) ([]byte, error) {
	t := C.int(0)
	if transparent {
		t = 1
	}
	shot, err := call(ctx, func(hd C.uintptr_t) {
		C.ink_wk_snapshot(v.p, C.double(x), C.double(y), C.double(w), C.double(h), C.double(scale), t, hd)
	})
	if err != nil {
		return nil, err
	}
	return recompress(shot), nil
}

// recompress encodes a PNG again with Go's best compression, which makes
// AppKit's PNGs several times smaller, and drops the alpha channel of an
// opaque one.
func recompress(b []byte) []byte {
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		return b
	}
	var out bytes.Buffer
	if (&png.Encoder{CompressionLevel: png.BestCompression}).Encode(&out, img) != nil || out.Len() >= len(b) {
		return b
	}
	return out.Bytes()
}

// PDF prints the top-left width by height CSS pixels of the page as one
// PDF page, zoom points per pixel.
func (v *View) PDF(ctx context.Context, width, height float64) ([]byte, error) {
	return call(ctx, func(h C.uintptr_t) { C.ink_wk_pdf(v.p, C.double(width), C.double(height), h) })
}

// Close frees the view.
func (v *View) Close() {
	if v.p != nil {
		C.ink_wk_free(v.p)
		v.p = nil
	}
}

// MergePDF joins PDFs into one, page after page.
func MergePDF(pages [][]byte) ([]byte, error) {
	if len(pages) == 0 {
		return nil, errors.New("no pages")
	}
	ptrs := C.malloc(C.size_t(len(pages)) * C.size_t(unsafe.Sizeof(uintptr(0))))
	lens := C.malloc(C.size_t(len(pages)) * C.size_t(unsafe.Sizeof(C.int(0))))
	defer C.free(ptrs)
	defer C.free(lens)
	ps := unsafe.Slice((*unsafe.Pointer)(ptrs), len(pages))
	ls := unsafe.Slice((*C.int)(lens), len(pages))
	for i, p := range pages {
		ps[i] = C.CBytes(p)
		ls[i] = C.int(len(p))
	}
	defer func() {
		for _, p := range ps {
			C.free(p)
		}
	}()
	return call(context.Background(), func(h C.uintptr_t) {
		C.ink_pdf_merge((*unsafe.Pointer)(ptrs), (*C.int)(lens), C.int(len(pages)), h)
	})
}

// RunMainLoop runs the main thread's run loop, for programs that are not
// apps, such as tests. Call it from main, on the main thread; it does not
// return.
func RunMainLoop() { C.ink_run_main() }
