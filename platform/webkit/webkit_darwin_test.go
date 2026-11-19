//go:build darwin

package webkit

import (
	"bytes"
	"context"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func init() { runtime.LockOSThread() }

// TestMain runs the tests beside the main run loop, as an app would.
func TestMain(m *testing.M) {
	go func() { os.Exit(m.Run()) }()
	RunMainLoop()
}

func TestView(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	dir := t.TempDir()
	page := filepath.Join(dir, "page.html")
	os.WriteFile(page, []byte(`<!doctype html><html><body style="margin:0">
<div id="box" style="position:absolute;left:10px;top:20px;width:100px;height:50px;background:rgb(255,0,0)"></div>
<p style="position:absolute;left:10px;top:100px;margin:0">Hello</p></body></html>`), 0o644)

	v := New(400, 300)
	defer v.Close()
	if err := v.Load(ctx, page, dir); err != nil {
		t.Fatal(err)
	}
	got, err := v.Eval(ctx, `await new Promise(r => setTimeout(r, 10)); return JSON.stringify(document.getElementById('box').getBoundingClientRect())`)
	if err != nil || !bytes.Contains([]byte(got), []byte(`"x":10,"y":20`)) {
		t.Fatalf("eval = %s, %v", got, err)
	}

	// A picture of the box: red, at twice the size, and the rectangle is
	// measured from the top left.
	shot, err := v.Snapshot(ctx, 10, 20, 100, 50, 2, true)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(shot))
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 200 || b.Dy() != 100 {
		t.Errorf("snapshot is %v, want 200x100", b)
	}
	r, g, bl, a := img.At(100, 50).RGBA()
	if r>>8 < 240 || g>>8 > 20 || bl>>8 > 20 || a>>8 < 240 {
		t.Errorf("middle pixel = %d %d %d %d, want red", r>>8, g>>8, bl>>8, a>>8)
	}
	// Outside the box the page has no background: transparent.
	shot, _ = v.Snapshot(ctx, 200, 200, 20, 20, 1, true)
	img, _ = png.Decode(bytes.NewReader(shot))
	if _, _, _, a := img.At(10, 10).RGBA(); a != 0 {
		t.Errorf("empty area alpha = %d, want 0", a>>8)
	}

	pdf1, err := v.PDF(ctx, 400, 300)
	if err != nil || !bytes.HasPrefix(pdf1, []byte("%PDF")) {
		t.Fatalf("pdf: %v", err)
	}
	merged, err := MergePDF([][]byte{pdf1, pdf1, pdf1})
	if err != nil || !bytes.HasPrefix(merged, []byte("%PDF")) {
		t.Fatalf("merge: %v", err)
	}
}
