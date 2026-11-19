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
	"github.com/inkcheck/inkstudio/platform/slidedoc"
)

// Format is an export format.
type Format struct {
	// ID names it in the API: export/<ID>.
	ID string
	// Ext is the file extension, without the dot.
	Ext string
	// Name is what save dialogs call it.
	Name string
}

// Export formats. An editable PowerPoint has text boxes, shapes and
// pictures; the pictures one has one picture per slide, exactly as drawn.
var (
	FormatPDF          = Format{"pdf", "pdf", "PDF"}
	FormatPPTX         = Format{"pptx", "pptx", "PowerPoint presentation"}
	FormatPPTXPictures = Format{"pptx-pictures", "pptx", "PowerPoint presentation"}
)

// FormatByID returns the format called id.
func FormatByID(id string) (Format, bool) {
	for _, f := range []Format{FormatPDF, FormatPPTX, FormatPPTXPictures} {
		if f.ID == id {
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

// ExportPDF renders the saved pack and prints it to out, one page per
// slide, with the export engine.
func (p *Project) ExportPDF(ctx context.Context, out string) (*Exported, error) {
	d, err := deck.Load(p.Dir)
	if err != nil {
		return nil, err
	}
	res, err := render.HTMLWith(d, render.Options{Diagrams: p.diagrams})
	if err != nil {
		return nil, err
	}
	var buf []byte
	if Engine == EngineWebKit {
		b, err := openDeck(ctx, res.HTML, p.Dir, res.Width, res.Height, 1, true)
		if err != nil {
			return nil, err
		}
		buf, err = b.PDF()
		b.Close()
		if err != nil {
			return nil, err
		}
	} else if buf, err = printChrome(ctx, res.HTML, res.Width, res.Height); err != nil {
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
	if f != FormatPPTX && f != FormatPPTXPictures {
		return nil, fmt.Errorf("cannot export slides as %s", f.Name)
	}
	b, err := openDeck(ctx, res.HTML, p.Dir, res.Width, res.Height, imageScale, false)
	if err != nil {
		return nil, err
	}
	defer b.Close()
	if b.Slides != len(res.Slides) {
		return nil, fmt.Errorf("the browser has %d slides of %d", b.Slides, len(res.Slides))
	}
	pd := slidedoc.Deck{Title: d.Title(), Author: d.Settings.Author, Width: res.Width, Height: res.Height}
	for i, r := range res.Slides {
		notes := d.Slides[i].Notes
		if f == FormatPPTXPictures {
			png, err := b.Picture(i)
			if err != nil {
				return nil, err
			}
			pd.Slides = append(pd.Slides, slidedoc.Picture(png, res.Width, res.Height, r.Title, notes))
		} else {
			s, err := b.Parts(i)
			if err != nil {
				return nil, err
			}
			s.Title, s.Notes = r.Title, notes
			pd.Slides = append(pd.Slides, *s)
		}
		if progress != nil {
			progress(i+1, len(res.Slides))
		}
	}
	var buf bytes.Buffer
	if err := pptx.Write(&buf, pd); err != nil {
		return nil, err
	}
	if err := os.WriteFile(out, buf.Bytes(), 0o644); err != nil {
		return nil, err
	}
	warnings := res.Warnings
	if warnings == nil {
		warnings = []string{}
	}
	return &Exported{Path: out, Slides: len(pd.Slides), Warnings: warnings}, nil
}

// printChrome prints a rendered deck with an installed Chromium-based
// browser, as inkpress does.
func printChrome(ctx context.Context, html []byte, width, height float64) ([]byte, error) {
	tmp, err := writeTemp(os.TempDir(), html)
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmp)
	buf, err := pdf.Print(ctx, tmp, pdf.Options{Width: width, Height: height, Timeout: 2 * time.Minute})
	if err != nil {
		if _, ferr := pdf.FindBrowser(); ferr != nil {
			return nil, fmt.Errorf("PDF export needs Chrome, Chromium, Edge, Brave or Vivaldi installed, or %s set to one: %w", pdf.BrowserEnv, err)
		}
		return nil, err
	}
	return buf, nil
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
