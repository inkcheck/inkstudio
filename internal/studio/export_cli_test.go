package studio

import (
	"os"
	"testing"
)

// TestExportTo writes real files for checking by hand or with other tools:
// INKSTUDIO_EXPORT_PACK=<pack> INKSTUDIO_EXPORT_OUT=<dir> go test -run ExportTo.
// INKSTUDIO_ENGINE picks webkit or chrome, and INKSTUDIO_EXPORT_PDF=1 adds a PDF.
func TestExportTo(t *testing.T) {
	pack, out := os.Getenv("INKSTUDIO_EXPORT_PACK"), os.Getenv("INKSTUDIO_EXPORT_OUT")
	if pack == "" || out == "" {
		t.Skip("set INKSTUDIO_EXPORT_PACK and INKSTUDIO_EXPORT_OUT")
	}
	if e := os.Getenv("INKSTUDIO_ENGINE"); e != "" {
		defer func(old string) { Engine = old }(Engine)
		Engine = e
	}
	p, err := Open(pack)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []Format{FormatPPTX, FormatPPTXPictures} {
		if _, err := p.ExportSlides(t.Context(), f, out+"/talk-"+f.ID+"."+f.Ext, nil); err != nil {
			t.Fatal(err)
		}
	}
	if os.Getenv("INKSTUDIO_EXPORT_PDF") == "" {
		return
	}
	if _, err := p.ExportPDF(t.Context(), out+"/talk.pdf"); err != nil {
		t.Fatal(err)
	}
}
