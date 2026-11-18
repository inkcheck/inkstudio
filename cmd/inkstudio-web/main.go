// Command inkstudio-web serves inkstudio's frontend and backend to a normal
// browser, for working on the UI with browser dev tools. There are no native
// dialogs or events: pass the pack on the command line, and reload the page
// to pick up changes made on disk.
package main

import (
	"flag"
	"log"
	"net/http"

	"github.com/inkcheck/inkstudio/internal/studio"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:1416", "address to listen on")
	dist := flag.String("dist", "frontend/dist", "the built frontend")
	flag.Parse()
	if flag.NArg() != 1 {
		log.Fatal("usage: inkstudio-web [-addr host:port] <pack>")
	}
	s := studio.New(studio.Host{})
	if _, err := s.OpenDir(flag.Arg(0)); err != nil {
		log.Fatal(err)
	}
	log.Printf("serving %s on http://%s/", flag.Arg(0), *addr)
	log.Fatal(http.ListenAndServe(*addr, s.Middleware(http.FileServer(http.Dir(*dist)))))
}
