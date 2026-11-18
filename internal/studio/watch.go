package studio

import (
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

// watch reports changed files in the pack, relative to it, a short while
// after the last change, until stop is closed. Hidden files are left out,
// except under .templates/, since inkpress writes temporary diagram files
// next to the slides while it renders.
func (p *Project) watch(stop <-chan struct{}, changed func(paths []string)) error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	add := func(root string) {
		filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || !d.IsDir() {
				return nil
			}
			if path != p.Dir && p.hidden(path) {
				return filepath.SkipDir
			}
			w.Add(path)
			return nil
		})
	}
	add(p.Dir)

	go func() {
		defer w.Close()
		pending := map[string]bool{}
		timer := time.NewTimer(time.Hour)
		timer.Stop()
		for {
			select {
			case <-stop:
				return
			case ev, ok := <-w.Events:
				if !ok {
					return
				}
				if p.hidden(ev.Name) {
					continue
				}
				if ev.Has(fsnotify.Create) {
					add(ev.Name) // a new directory needs its own watch
				}
				if rel := p.Rel(ev.Name); rel != "" {
					pending[rel] = true
					timer.Reset(150 * time.Millisecond)
				}
			case <-w.Errors:
			case <-timer.C:
				paths := make([]string, 0, len(pending))
				for rel := range pending {
					paths = append(paths, rel)
				}
				sort.Strings(paths)
				clear(pending)
				changed(paths)
			}
		}
	}()
	return nil
}

// hidden reports whether path, inside the pack, has a hidden part other
// than a leading .templates.
func (p *Project) hidden(path string) bool {
	rel := p.Rel(path)
	if rel == "" || rel == "." {
		return false
	}
	for i, part := range strings.Split(rel, "/") {
		if strings.HasPrefix(part, ".") && !(i == 0 && part == ".templates") {
			return true
		}
	}
	return false
}
