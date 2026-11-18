package studio

import (
	"os"
	"testing"
)

// TestExportTo writes real files for checking by hand or with other tools:
// INKSTUDIO_EXPORT_PACK=<pack> INKSTUDIO_EXPORT_OUT=<dir> go test -run ExportTo.
func TestExportTo(t *testing.T) {
	pack, out := os.Getenv("INKSTUDIO_EXPORT_PACK"), os.Getenv("INKSTUDIO_EXPORT_OUT")
	if pack == "" || out == "" {
		t.Skip("set INKSTUDIO_EXPORT_PACK and INKSTUDIO_EXPORT_OUT")
	}
	p, err := Open(pack)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []Format{FormatPPTX, FormatODP} {
		if _, err := p.ExportSlides(t.Context(), f, out+"/talk."+f.Ext, nil); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := p.ExportPDF(t.Context(), out+"/talk.pdf"); err != nil {
		t.Fatal(err)
	}
}
