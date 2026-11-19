package studio

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/inkcheck/inkpress/deck"
	"github.com/inkcheck/inkpress/templates"
)

// An .inktheme file is a zip of one inkpress template pack: template.toml,
// style.css, themes/, layouts/ and the rest. template.toml is at the root of
// the zip, or in a single folder at the root. The template's name is the
// file's name, without .inktheme.

// InkthemeExt is the extension of theme files.
const InkthemeExt = ".inktheme"

// limits caps what an unpack writes.
type limits struct {
	what  string // "theme" or "deck", for messages
	files int
	bytes int64
}

var themeLimits = limits{"theme", 2000, 200 << 20}

var templateNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)

// ThemeName is the template name an .inktheme file installs as.
func ThemeName(file string) (string, error) {
	base := filepath.Base(file)
	name := strings.TrimSuffix(strings.TrimSuffix(base, InkthemeExt), ".zip")
	name = strings.ReplaceAll(strings.TrimSpace(name), " ", "-")
	if !templateNameRe.MatchString(name) {
		return "", fmt.Errorf("%q is not a usable template name: use letters, digits, '-', '_' and '.'", name)
	}
	return name, nil
}

// TemplateExists reports whether the pack has a template called name.
func (p *Project) TemplateExists(name string) bool {
	_, err := os.Stat(filepath.Join(p.Dir, deck.TemplatesDir, name))
	return err == nil
}

// ImportTheme unpacks an .inktheme into .templates/<name>. It unpacks into
// a hidden folder first, checks that the template loads, and only then moves
// it into place, replacing a template of the same name when replace is set.
func (p *Project) ImportTheme(file, name string, replace bool) error {
	if !templateNameRe.MatchString(name) {
		return fmt.Errorf("%q is not a usable template name", name)
	}
	tdir := filepath.Join(p.Dir, deck.TemplatesDir)
	final := filepath.Join(tdir, name)
	if !replace && p.TemplateExists(name) {
		return fmt.Errorf("a template called %q already exists", name)
	}
	if err := os.MkdirAll(tdir, 0o755); err != nil {
		return err
	}

	zr, err := zip.OpenReader(file)
	if err != nil {
		return fmt.Errorf("%s is not an .inktheme (a zip file): %w", filepath.Base(file), err)
	}
	defer zr.Close()
	root, err := themeRoot(zr.File)
	if err != nil {
		return err
	}

	// Unpack beside the templates, so the final move is a rename, under a
	// plain name, since templates.Load refuses hidden ones.
	work, err := os.MkdirTemp(tdir, ".inkstudio-import-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	staging := filepath.Join(work, "theme")
	if err := os.Mkdir(staging, 0o755); err != nil {
		return err
	}
	if err := unpack(zr.File, root, staging, themeLimits); err != nil {
		return err
	}
	if _, err := templates.Load(work, "theme"); err != nil {
		return fmt.Errorf("the theme does not load: %w", err)
	}

	if replace && p.TemplateExists(name) {
		old := filepath.Join(tdir, ".inkstudio-old-"+name)
		os.RemoveAll(old)
		if err := os.Rename(final, old); err != nil {
			return err
		}
		if err := os.Rename(staging, final); err != nil {
			os.Rename(old, final) // put it back
			return err
		}
		return os.RemoveAll(old)
	}
	return os.Rename(staging, final)
}

// themeRoot finds the folder in the zip that holds template.toml: the root,
// or one folder at the root.
func themeRoot(files []*zip.File) (string, error) {
	roots := map[string]bool{}
	for _, f := range files {
		name := strings.TrimPrefix(f.Name, "./")
		if path.Base(name) != "template.toml" || f.FileInfo().IsDir() {
			continue
		}
		dir := path.Dir(name)
		if dir == "." {
			return "", nil
		}
		if !strings.Contains(dir, "/") && !strings.HasPrefix(dir, "__MACOSX") {
			roots[dir+"/"] = true
		}
	}
	switch len(roots) {
	case 0:
		return "", errors.New("no template.toml in the theme: an .inktheme is a zip of a template pack, with template.toml at its root")
	case 1:
		for r := range roots {
			return r, nil
		}
	}
	return "", errors.New("the theme holds more than one template; an .inktheme holds one")
}

// unpack writes the files under root in the zip to dir. It refuses paths
// that leave dir, links and devices, and stops at the limits.
func unpack(files []*zip.File, root, dir string, lim limits) error {
	count := 0
	var total int64
	for _, f := range files {
		name := strings.TrimPrefix(f.Name, "./")
		if !strings.HasPrefix(name, root) {
			continue
		}
		rel := strings.TrimPrefix(name, root)
		if rel == "" || strings.HasPrefix(rel, "__MACOSX/") || path.Base(rel) == ".DS_Store" {
			continue
		}
		clean := path.Clean(rel)
		if path.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, "../") || strings.Contains(rel, `\`) {
			return fmt.Errorf("the %s has a file outside its folder: %s", lim.what, f.Name)
		}
		target := filepath.Join(dir, filepath.FromSlash(clean))
		mode := f.Mode()
		if mode.IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if !mode.IsRegular() {
			return fmt.Errorf("the %s has a link or special file: %s", lim.what, f.Name)
		}
		count++
		total += int64(f.UncompressedSize64)
		if count > lim.files || total > lim.bytes {
			return fmt.Errorf("the %s is too big: more than %d files or %d MB", lim.what, lim.files, lim.bytes>>20)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := extract(f, target, lim.bytes-total+int64(f.UncompressedSize64)); err != nil {
			return err
		}
	}
	return nil
}

func extract(f *zip.File, target string, limit int64) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	// The header's size can lie; count what is really there.
	n, err := io.Copy(out, io.LimitReader(rc, limit+1))
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err == nil && n > limit {
		err = errors.New("the archive holds more than its headers say")
	}
	return err
}

// ExportTheme zips .templates/<name> into an .inktheme at out, with
// template.toml at the root. Hidden files are left out.
func (p *Project) ExportTheme(name, out string) error {
	if !templateNameRe.MatchString(name) || !p.TemplateExists(name) {
		return fmt.Errorf("no template called %q", name)
	}
	src := filepath.Join(p.Dir, deck.TemplatesDir, name)
	tmp, err := os.CreateTemp(filepath.Dir(out), ".inkstudio-export-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	zw := zip.NewWriter(tmp)
	err = filepath.WalkDir(src, func(file string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if strings.HasPrefix(d.Name(), ".") && file != src {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil // directories are implied; links are left out
		}
		rel, err := filepath.Rel(src, file)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		h, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		h.Name = filepath.ToSlash(rel)
		h.Method = zip.Deflate
		w, err := zw.CreateHeader(h)
		if err != nil {
			return err
		}
		f, err := os.Open(file)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(w, f)
		return err
	})
	if cerr := zw.Close(); err == nil {
		err = cerr
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	return os.Rename(tmp.Name(), out)
}
