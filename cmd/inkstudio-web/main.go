// Command inkstudio-web serves InkStudio's frontend and backend to a normal
// browser, for working on the UI with browser dev tools, and for the site's
// screenshots. There are no native dialogs or events: pass the pack on the
// command line, or -sample for a fresh copy of inkpress's sample deck, and
// reload the page to pick up changes made on disk.
package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/inkcheck/inkpress/scaffold"

	"github.com/inkcheck/inkstudio/internal/studio"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:1416", "address to listen on")
	dist := flag.String("dist", "frontend/dist", "the built frontend")
	sample := flag.Bool("sample", false, "serve a fresh copy of inkpress's sample deck")
	flag.Parse()
	dir := flag.Arg(0)
	switch {
	case *sample && flag.NArg() == 0:
		tmp, err := os.MkdirTemp("", "inkstudio-sample-")
		if err != nil {
			log.Fatal(err)
		}
		defer os.RemoveAll(tmp)
		dir = filepath.Join(tmp, "talk")
		if _, err := scaffold.New(dir, false); err != nil {
			log.Fatal(err)
		}
	case flag.NArg() != 1:
		log.Fatal("usage: inkstudio-web [-addr host:port] <pack> | -sample")
	}
	s := studio.New(studio.Host{})
	if _, err := s.OpenDir(dir); err != nil {
		log.Fatal(err)
	}
	log.Printf("serving %s on http://%s/", dir, *addr)
	log.Fatal(http.ListenAndServe(*addr, s.Middleware(http.FileServer(http.Dir(*dist)))))
}
