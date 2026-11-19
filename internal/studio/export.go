package studio

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/inkcheck/inkpress/deck"
	"github.com/inkcheck/inkpress/pdf"
	"github.com/inkcheck/inkpress/render"

	"github.com/inkcheck/inkstudio/platform/pptx"
)

// Format is an export format.
type Format struct {
	// Ext is the file extension, without the dot.
	Ext string
	// Name is what save dialogs call it.
	Name string
}

// Export formats.
var (
	FormatPDF  = Format{"pdf", "PDF"}
	FormatPPTX = Format{"pptx", "PowerPoint presentation"}
)

// FormatByExt returns the format with extension ext.
func FormatByExt(ext string) (Format, bool) {
	for _, f := range []Format{FormatPDF, FormatPPTX} {
		if f.Ext == ext {
			return f, true
		}
	}
	return Format{}, false
}

// imageScale is how many device pixels slide images get per CSS pixel:
// 2 is sharp on a projector and on a Retina screen.
const imageScale = 2

// Exported is the result of an export.
type Exported struct {
	// Path is "" when the user cancelled.
	Path     string   `json:"path"`
	Slides   int      `json:"slides"`
	Warnings []string `json:"warnings"`
}

// DefaultPDF is where the pack's PDF goes by default: settings' output, or
// <pack name>.pdf, in the pack.
func (p *Project) DefaultPDF() string {
	return p.DefaultExport(FormatPDF)
}

// DefaultExport is where an export goes by default: in the pack, named
// like settings' output, or after the pack, with the format's extension.
func (p *Project) DefaultExport(f Format) string {
	name := filepath.Base(p.Dir) + ".pdf"
	if text, err := os.ReadFile(filepath.Join(p.Dir, deck.SettingsFile)); err == nil {
		if s, err := deck.ParseSettings(string(text)); err == nil && s.Output != "" {
			name = s.Output
		}
	}
	name = strings.TrimSuffix(name, filepath.Ext(name)) + "." + f.Ext
	if filepath.IsAbs(name) {
		return name
	}
	return filepath.Join(p.Dir, name)
}

// ExportPDF renders the saved pack and prints it to out with an installed
// Chromium-based browser, as inkpress does.
func (p *Project) ExportPDF(ctx context.Context, out string) (*Exported, error) {
	d, err := deck.Load(p.Dir)
	if err != nil {
		return nil, err
	}
	res, err := render.HTMLWith(d, render.Options{Diagrams: p.diagrams})
	if err != nil {
		return nil, err
	}
	tmp, err := os.CreateTemp("", "inkstudio-*.html")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(res.HTML); err != nil {
		tmp.Close()
		return nil, err
	}
	if err := tmp.Close(); err != nil {
		return nil, err
	}
	buf, err := pdf.Print(ctx, tmp.Name(), pdf.Options{Width: res.Width, Height: res.Height, Timeout: 2 * time.Minute})
	if err != nil {
		if _, ferr := pdf.FindBrowser(); ferr != nil {
			return nil, fmt.Errorf("PDF export needs Chrome, Chromium, Edge, Brave or Vivaldi installed, or %s set to one: %w", pdf.BrowserEnv, err)
		}
		return nil, err
	}
	if err := os.WriteFile(out, buf, 0o644); err != nil {
		return nil, err
	}
	warnings := res.Warnings
	if warnings == nil {
		warnings = []string{}
	}
	return &Exported{Path: out, Slides: len(d.Slides), Warnings: warnings}, nil
}

// ExportSlides renders the saved pack to images, one per slide, and writes
// them with the notes as a PowerPoint presentation at out.
func (p *Project) ExportSlides(ctx context.Context, f Format, out string, progress func(done, total int)) (*Exported, error) {
	d, err := deck.Load(p.Dir)
	if err != nil {
		return nil, err
	}
	res, err := render.HTMLWith(d, render.Options{Diagrams: p.diagrams})
	if err != nil {
		return nil, err
	}
	images, err := rasterize(ctx, res.HTML, res.Width, res.Height, imageScale, progress)
	if err != nil {
		return nil, err
	}
	if len(images) != len(res.Slides) {
		return nil, fmt.Errorf("rendered %d slide images for %d slides", len(images), len(res.Slides))
	}
	var buf bytes.Buffer
	switch f {
	case FormatPPTX:
		pd := pptx.Deck{Title: d.Title(), Author: d.Settings.Author, Width: res.Width, Height: res.Height}
		for i, r := range res.Slides {
			pd.Slides = append(pd.Slides, pptx.Slide{PNG: images[i], Title: r.Title, Notes: d.Slides[i].Notes})
		}
		err = pptx.Write(&buf, pd)
	default:
		return nil, fmt.Errorf("cannot export slides as %s", f.Name)
	}
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(out, buf.Bytes(), 0o644); err != nil {
		return nil, err
	}
	warnings := res.Warnings
	if warnings == nil {
		warnings = []string{}
	}
	return &Exported{Path: out, Slides: len(images), Warnings: warnings}, nil
}

// Reveal shows path in the system's file manager.
func Reveal(path string) error {
	if _, err := os.Stat(path); err != nil {
		return err
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", "-R", path)
	case "windows":
		cmd = exec.Command("explorer", "/select,", path)
	case "linux":
		cmd = exec.Command("xdg-open", filepath.Dir(path))
	default:
		return errors.New("revealing files is not supported here")
	}
	return cmd.Start()
}
