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
	"strings"

	"github.com/inkcheck/inkpress/deck"
)

// An .inkdeck file is a zip of a whole pack, for sharing: settings.toml,
// slides/, assets/ and .templates/, inside one folder named after the deck.
// It is never edited in place: opening one unpacks a copy, a normal pack
// folder, and the app opens that.

// InkdeckExt is the extension of shared decks.
const InkdeckExt = ".inkdeck"

var deckLimits = limits{"deck", 5000, 1 << 30}

// shareSkip reports whether a file of the pack stays out of a shared deck:
// hidden files other than the templates, links, and what builds and
// exports leave at the top of the pack.
func shareSkip(rel string, d fs.DirEntry) bool {
	parts := strings.Split(rel, "/")
	for i, part := range parts {
		if strings.HasPrefix(part, ".") && !(i == 0 && part == deck.TemplatesDir) {
			return true
		}
	}
	if d.Type()&fs.ModeSymlink != 0 {
		return true
	}
	if len(parts) == 1 && !d.IsDir() {
		switch strings.ToLower(path.Ext(rel)) {
		case ".pdf", ".pptx", ".odp", ".html", InkdeckExt:
			return true
		}
	}
	return false
}

// ShareDeck zips the pack into an .inkdeck at out. It returns the number of
// files it packed.
func (p *Project) ShareDeck(out string) (int, error) {
	if p.Contains(filepath.Clean(out)) && filepath.Dir(out) != p.Dir {
		return 0, errors.New("save the .inkdeck outside the presentation's folders")
	}
	root := filepath.Base(p.Dir)
	tmp, err := os.CreateTemp(filepath.Dir(out), ".inkstudio-share-*")
	if err != nil {
		return 0, err
	}
	defer os.Remove(tmp.Name())
	zw := zip.NewWriter(tmp)
	count := 0
	err = filepath.WalkDir(p.Dir, func(file string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if file == p.Dir {
			return nil
		}
		rel := p.Rel(file)
		if shareSkip(rel, d) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		h, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		h.Name = root + "/" + rel
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
		count++
		return err
	})
	if cerr := zw.Close(); err == nil {
		err = cerr
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return 0, err
	}
	return count, os.Rename(tmp.Name(), out)
}

// deckRoot finds the folder in the zip that holds slides/: the root, or one
// folder at the root.
func deckRoot(files []*zip.File) (string, error) {
	roots := map[string]bool{}
	for _, f := range files {
		name := strings.TrimPrefix(f.Name, "./")
		if strings.HasPrefix(name, "__MACOSX/") {
			continue
		}
		i := strings.Index(name, deck.SlidesDir+"/")
		if i < 0 || (i > 0 && name[i-1] != '/') {
			continue
		}
		prefix := name[:i]
		if strings.Count(prefix, "/") <= 1 {
			roots[prefix] = true
		}
	}
	switch len(roots) {
	case 0:
		return "", errors.New("no slides/ folder in the file: an .inkdeck is a zip of an inkpress presentation")
	case 1:
		for r := range roots {
			return r, nil
		}
	}
	return "", errors.New("the file holds more than one presentation")
}

// DeckName is the folder name an .inkdeck unpacks to: its own name.
func DeckName(file string) string {
	name := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
	name = strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == ':' || r < 0x20 {
			return '-'
		}
		return r
	}, strings.TrimSpace(name))
	if name == "" || strings.HasPrefix(name, ".") {
		name = "Presentation" + name
	}
	return name
}

// UnpackDeck unpacks an .inkdeck into a new folder in dir, named after the
// file, with a number added when that name is taken. It returns the folder.
func UnpackDeck(file, dir string) (string, error) {
	zr, err := zip.OpenReader(file)
	if err != nil {
		return "", fmt.Errorf("%s is not an .inkdeck (a zip file): %w", filepath.Base(file), err)
	}
	defer zr.Close()
	root, err := deckRoot(zr.File)
	if err != nil {
		return "", err
	}

	name := DeckName(file)
	target := filepath.Join(dir, name)
	for i := 2; ; i++ {
		if _, err := os.Lstat(target); errors.Is(err, fs.ErrNotExist) {
			break
		}
		if i > 99 {
			return "", fmt.Errorf("%s already has folders called %s", dir, name)
		}
		target = filepath.Join(dir, fmt.Sprintf("%s %d", name, i))
	}

	// Unpack beside the target and rename, so a failure leaves nothing.
	work, err := os.MkdirTemp(dir, ".inkstudio-unpack-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(work)
	if err := unpack(zr.File, root, work, deckLimits); err != nil {
		return "", err
	}
	if st, err := os.Stat(filepath.Join(work, deck.SlidesDir)); err != nil || !st.IsDir() {
		return "", errors.New("the file has no slides/ folder")
	}
	if err := os.Chmod(work, 0o755); err != nil {
		return "", err
	}
	if err := os.Rename(work, target); err != nil {
		return "", err
	}
	return target, nil
}
