//go:build !darwin

// Package webkit drives an offscreen WebKit view on macOS. Elsewhere it is
// not available, and every call fails.
package webkit

import (
	"context"
	"errors"
)

// Available reports whether this platform has WebKit.
const Available = false

var errUnavailable = errors.New("webkit: not available on this platform")

// View is an offscreen web view.
type View struct{}

// New makes a view.
func New(width, height, zoom float64) *View { return &View{} }

// Load loads an HTML file.
func (v *View) Load(ctx context.Context, file, readAccess string) error { return errUnavailable }

// Eval runs JavaScript.
func (v *View) Eval(ctx context.Context, body string) (string, error) { return "", errUnavailable }

// Snapshot takes a picture.
func (v *View) Snapshot(ctx context.Context, x, y, w, h, scale float64, transparent bool) ([]byte, error) {
	return nil, errUnavailable
}

// PDF prints a page.
func (v *View) PDF(ctx context.Context, width, height float64) ([]byte, error) {
	return nil, errUnavailable
}

// Close frees the view.
func (v *View) Close() {}

// MergePDF joins PDFs.
func MergePDF(pages [][]byte) ([]byte, error) { return nil, errUnavailable }

// RunMainLoop is for macOS.
func RunMainLoop() {}
