// Package studio is inkstudio's backend: it opens an inkpress pack, reads and
// writes its files, renders previews and watches the pack for changes. The
// frontend talks to it over HTTP on the Wails asset server (see server.go).
package studio

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/inkcheck/inkpress/deck"
	"github.com/inkcheck/inkpress/render"
)

// Project is an open pack.
type Project struct {
	// Dir is the pack's absolute path.
	Dir string
	// diagrams keeps rendered diagrams between previews.
	diagrams *render.DiagramCache
}

// File is a file the editor can open, with a path relative to the pack.
type File struct {
	Path string `json:"path"`
	Name string `json:"name"`
	// Group places the file in the sidebar: "pack" or "slides".
	Group string `json:"group"`
}

// ProjectInfo is what the frontend shows about a pack.
type ProjectInfo struct {
	Dir   string `json:"dir"`
	Title string `json:"title"`
	Files []File `json:"files"`
}

// Open checks that dir looks like a pack: it needs a slides directory.
func Open(dir string) (*Project, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	st, err := os.Stat(filepath.Join(abs, deck.SlidesDir))
	if err != nil || !st.IsDir() {
		return nil, fmt.Errorf("%s is not an inkpress pack: it has no %s/ folder", abs, deck.SlidesDir)
	}
	return &Project{Dir: abs, diagrams: render.NewDiagramCache()}, nil
}

// Info lists the pack's editable files.
func (p *Project) Info() (*ProjectInfo, error) {
	info := &ProjectInfo{Dir: p.Dir, Title: filepath.Base(p.Dir)}
	if text, err := os.ReadFile(filepath.Join(p.Dir, deck.SettingsFile)); err == nil {
		if s, err := deck.ParseSettings(string(text)); err == nil && s.Title != "" {
			info.Title = s.Title
		}
	}
	for _, name := range []string{deck.SettingsFile, "README.md"} {
		if _, err := os.Stat(filepath.Join(p.Dir, name)); err == nil {
			info.Files = append(info.Files, File{Path: name, Name: name, Group: "pack"})
		}
	}
	entries, err := os.ReadDir(filepath.Join(p.Dir, deck.SlidesDir))
	if err != nil {
		return nil, err
	}
	var slides []string
	for _, e := range entries {
		if !e.IsDir() && strings.EqualFold(filepath.Ext(e.Name()), ".md") && !strings.HasPrefix(e.Name(), ".") {
			slides = append(slides, e.Name())
		}
	}
	sort.Strings(slides)
	for _, name := range slides {
		info.Files = append(info.Files, File{Path: deck.SlidesDir + "/" + name, Name: name, Group: "slides"})
	}
	return info, nil
}

// Abs turns a slash-separated path relative to the pack into an absolute
// one, refusing paths that leave the pack.
func (p *Project) Abs(rel string) (string, error) {
	if rel == "" || filepath.IsAbs(rel) || strings.Contains(rel, `\`) {
		return "", fmt.Errorf("bad path %q", rel)
	}
	clean := filepath.Clean(filepath.FromSlash(rel))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("bad path %q", rel)
	}
	return filepath.Join(p.Dir, clean), nil
}

// Rel turns an absolute path into a slash-separated path relative to the
// pack, or "" when it is outside the pack.
func (p *Project) Rel(abs string) string {
	rel, err := filepath.Rel(p.Dir, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return ""
	}
	return filepath.ToSlash(rel)
}

// Contains reports whether abs is inside the pack.
func (p *Project) Contains(abs string) bool {
	return p.Rel(filepath.Clean(abs)) != ""
}

// editable reports whether the editor may read and write rel: text files of
// the pack, and nothing hidden except the templates.
func editable(rel string) bool {
	switch strings.ToLower(filepath.Ext(rel)) {
	case ".md", ".toml", ".css", ".html", ".txt":
	default:
		return false
	}
	for i, part := range strings.Split(rel, "/") {
		if strings.HasPrefix(part, ".") && !(i == 0 && part == deck.TemplatesDir) {
			return false
		}
	}
	return true
}

// ReadFile returns a text file of the pack.
func (p *Project) ReadFile(rel string) (string, error) {
	abs, err := p.Abs(rel)
	if err != nil {
		return "", err
	}
	if !editable(rel) {
		return "", fmt.Errorf("%s: not an editable file", rel)
	}
	b, err := os.ReadFile(abs)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// WriteFile saves a text file of the pack. It writes a temporary file and
// renames it, so a crash cannot leave half a slide.
func (p *Project) WriteFile(rel, text string) error {
	abs, err := p.Abs(rel)
	if err != nil {
		return err
	}
	if !editable(rel) {
		return fmt.Errorf("%s: not an editable file", rel)
	}
	mode := fs.FileMode(0o644)
	if st, err := os.Stat(abs); err == nil {
		mode = st.Mode().Perm()
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(abs), ".inkstudio-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(text); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), mode); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), abs)
}
