package studio

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/inkcheck/inkpress/deck"
	"github.com/inkcheck/inkpress/scaffold"
)

// Host is what the studio needs from the desktop shell.
type Host struct {
	// ChooseDir asks for a folder, starting in start when it is set; ""
	// means the user cancelled.
	ChooseDir func(title, start string, create bool) (string, error)
	// ChooseSave asks where to save a file, suggesting path, of the kind
	// filter names (such as "PDF") with the extension ext (such as "pdf");
	// "" means the user cancelled.
	ChooseSave func(title, path, filter, ext string) (string, error)
	// ChooseFile asks for a file to open, of the kind filter names, with
	// one of the extensions exts; "" means the user cancelled.
	ChooseFile func(title, filter string, exts []string) (string, error)
	// Confirm asks a yes-or-no question; ok labels the yes button.
	Confirm func(title, message, ok string) bool
	// SetFullscreen puts the window in or out of full screen, for presenting.
	SetFullscreen func(on bool)
	// Emit sends an event to the frontend.
	Emit func(name string, data any)
	// Opened is called after a pack opens.
	Opened func(info *ProjectInfo)
}

type statusRecorder struct {
	http.ResponseWriter
	status, n int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	n, err := r.ResponseWriter.Write(b)
	r.n += n
	return n, err
}

// Events sent to the frontend.
const (
	// EventOpened carries a ProjectInfo.
	EventOpened = "project:opened"
	// EventChanged carries {"paths": [...]}, relative to the pack.
	EventChanged = "project:changed"
)

// Studio holds the open pack and serves the frontend's API.
type Studio struct {
	host Host

	mu        sync.Mutex
	project   *Project
	stopWatch chan struct{}
	// previews keeps the last few renders, so a frame that is still loading
	// one finds it.
	previews map[int][]byte
	version  int
}

// New returns a studio with no pack open.
func New(host Host) *Studio {
	return &Studio{host: host, previews: map[int][]byte{}}
}

func (s *Studio) current() *Project {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.project
}

// OpenDir opens the pack in dir, closing the one that was open.
func (s *Studio) OpenDir(dir string) (*ProjectInfo, error) {
	p, err := Open(dir)
	if err != nil {
		return nil, err
	}
	info, err := p.Info()
	if err != nil {
		return nil, err
	}
	stop := make(chan struct{})
	if err := p.watch(stop, func(paths []string) {
		// Diagrams can use files from the pack, such as icons; the cache
		// keys only on their source.
		for _, rel := range paths {
			// Template CSS and layouts don't change how diagrams draw.
			if !isSlideOrSettings(rel) && !strings.HasPrefix(rel, deck.TemplatesDir+"/") {
				p.diagrams.Reset()
				break
			}
		}
		s.emit(EventChanged, map[string]any{"paths": paths})
	}); err != nil {
		return nil, err
	}

	s.mu.Lock()
	if s.stopWatch != nil {
		close(s.stopWatch)
	}
	s.project, s.stopWatch = p, stop
	clear(s.previews)
	s.mu.Unlock()

	if s.host.Opened != nil {
		s.host.Opened(info)
	}
	s.emit(EventOpened, info)
	return info, nil
}

// PromptOpen asks for a pack and opens it. It returns nil when the user
// cancels.
func (s *Studio) PromptOpen() (*ProjectInfo, error) {
	dir, err := s.host.ChooseDir("Open a presentation folder", "", false)
	if err != nil || dir == "" {
		return nil, err
	}
	return s.OpenDir(dir)
}

// PromptNew asks for an empty folder, writes inkpress's sample pack into it
// and opens it. It returns nil when the user cancels.
func (s *Studio) PromptNew() (*ProjectInfo, error) {
	dir, err := s.host.ChooseDir("Choose an empty folder for the new presentation", "", true)
	if err != nil || dir == "" {
		return nil, err
	}
	if entries, err := os.ReadDir(dir); err == nil {
		for _, e := range entries {
			if !strings.HasPrefix(e.Name(), ".") {
				return nil, fmt.Errorf("%s is not empty", dir)
			}
		}
	}
	if _, err := scaffold.New(dir, false); err != nil {
		return nil, err
	}
	return s.OpenDir(dir)
}

func (s *Studio) emit(name string, data any) {
	if s.host.Emit != nil {
		s.host.Emit(name, data)
	}
}

// Middleware serves the API, the previews and the pack's files, and passes
// everything else to next: the frontend's own assets.
func (s *Studio) Middleware(next http.Handler) http.Handler {
	debug := os.Getenv("INKSTUDIO_DEBUG") != ""
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if debug {
			rec := &statusRecorder{ResponseWriter: w, status: 200}
			w = rec
			start := time.Now()
			defer func() {
				log.Printf("inkstudio: %s %s -> %d (%d bytes, %s)", r.Method, r.URL.String(), rec.status, rec.n, time.Since(start).Round(time.Millisecond))
			}()
		}
		switch {
		case strings.HasPrefix(path, "/api/"):
			s.api(w, r, strings.TrimPrefix(path, "/api/"))
		case strings.HasPrefix(path, "/preview/"):
			s.preview(w, r, strings.TrimPrefix(path, "/preview/"))
		case strings.HasPrefix(path, FSPrefix):
			s.file(w, r, strings.TrimPrefix(path, FSPrefix))
		default:
			next.ServeHTTP(w, r)
		}
	})
}

type renderRequest struct {
	// Overlay maps paths in the pack to unsaved text.
	Overlay map[string]string `json:"overlay"`
}

type renderResponse struct {
	*Preview
	Version int    `json:"version"`
	URL     string `json:"url"`
}

func (s *Studio) api(w http.ResponseWriter, r *http.Request, route string) {
	var (
		result any
		err    error
	)
	switch {
	case route == "fullscreen" && r.Method == http.MethodPost:
		var req struct {
			On bool `json:"on"`
		}
		if err = decode(r, &req); err == nil {
			if s.host.SetFullscreen == nil {
				err = errors.New("full screen is not available here")
				break
			}
			s.host.SetFullscreen(req.On)
			result = map[string]bool{"ok": true}
		}
	case route == "log" && r.Method == http.MethodPost:
		// The frontend reports its errors here, so they reach the terminal.
		b, _ := io.ReadAll(io.LimitReader(r.Body, 16<<10))
		log.Printf("inkstudio: frontend: %s", b)
		result = map[string]bool{"ok": true}
	case route == "project" && r.Method == http.MethodGet:
		if p := s.current(); p != nil {
			result, err = p.Info()
		}
	case route == "open" && r.Method == http.MethodPost:
		result, err = s.PromptOpen()
	case route == "new" && r.Method == http.MethodPost:
		result, err = s.PromptNew()
	case route == "file" && r.Method == http.MethodGet:
		p := s.current()
		if p == nil {
			err = errNoProject
			break
		}
		var text string
		if text, err = p.ReadFile(r.URL.Query().Get("path")); err == nil {
			result = map[string]string{"text": text}
		}
	case route == "file" && r.Method == http.MethodPut:
		p := s.current()
		if p == nil {
			err = errNoProject
			break
		}
		var body []byte
		if body, err = io.ReadAll(io.LimitReader(r.Body, 16<<20)); err == nil {
			err = p.WriteFile(r.URL.Query().Get("path"), string(body))
			result = map[string]bool{"ok": err == nil}
		}
	case route == "render" && r.Method == http.MethodPost:
		result, err = s.render(r)
	case strings.HasPrefix(route, "export/") && r.Method == http.MethodPost:
		f, ok := FormatByExt(strings.TrimPrefix(route, "export/"))
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		result, err = s.export(r, f)
	case route == "templates" && r.Method == http.MethodGet:
		p := s.current()
		if p == nil {
			err = errNoProject
			break
		}
		result, err = p.Templates()
	case route == "settings/parse" && r.Method == http.MethodPost:
		var req struct {
			Text string `json:"text"`
		}
		if err = decode(r, &req); err == nil {
			result = ParseDeckSettings(req.Text)
		}
	case (route == "edit/settings" || route == "edit/frontmatter") && r.Method == http.MethodPost:
		var req struct {
			Text    string   `json:"text"`
			Changes []Change `json:"changes"`
		}
		if err = decode(r, &req); err != nil {
			break
		}
		var text string
		if route == "edit/settings" {
			text, err = EditSettings(req.Text, req.Changes)
		} else {
			text, err = EditFrontmatter(req.Text, req.Changes)
		}
		result = map[string]string{"text": text}
	case route == "theme/props" && r.Method == http.MethodPost:
		var req struct {
			Text string `json:"text"`
		}
		if err = decode(r, &req); err == nil {
			result = ThemeProps(req.Text)
		}
	case route == "edit/theme" && r.Method == http.MethodPost:
		var req struct {
			Text    string       `json:"text"`
			Changes []PropChange `json:"changes"`
		}
		if err = decode(r, &req); err != nil {
			break
		}
		var text string
		text, err = EditTheme(req.Text, req.Changes)
		result = map[string]string{"text": text}
	case route == "theme/duplicate" && r.Method == http.MethodPost:
		p := s.current()
		if p == nil {
			err = errNoProject
			break
		}
		var req struct {
			Template, Theme, Name string
		}
		if err = decode(r, &req); err == nil {
			err = p.DuplicateTheme(req.Template, req.Theme, req.Name)
			result = map[string]string{"name": req.Name}
		}
	case route == "deck/share" && r.Method == http.MethodPost:
		result, err = s.shareDeck()
	case route == "deck/open" && r.Method == http.MethodPost:
		if s.host.ChooseFile == nil {
			err = errors.New("opening files is not available here")
			break
		}
		var file string
		file, err = s.host.ChooseFile("Open a shared presentation", "InkStudio presentation", []string{"inkdeck", "zip"})
		if err == nil && file != "" {
			var info *ProjectInfo
			info, err = s.OpenDeckFile(file)
			result = info
		}
	case route == "theme/import" && r.Method == http.MethodPost:
		result, err = s.importTheme()
	case route == "theme/export" && r.Method == http.MethodPost:
		var req struct {
			Name string `json:"name"`
		}
		if err = decode(r, &req); err == nil {
			result, err = s.exportTheme(req.Name)
		}
	case route == "reveal" && r.Method == http.MethodPost:
		var req struct {
			Path string `json:"path"`
		}
		if err = json.NewDecoder(r.Body).Decode(&req); err == nil {
			err = Reveal(req.Path)
			result = map[string]bool{"ok": err == nil}
		}
	default:
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err != nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(result)
}

var errNoProject = errors.New("no presentation is open")

func decode(r *http.Request, v any) error {
	return json.NewDecoder(io.LimitReader(r.Body, 16<<20)).Decode(v)
}

func (s *Studio) importTheme() (map[string]string, error) {
	if s.current() == nil {
		return nil, errNoProject
	}
	if s.host.ChooseFile == nil {
		return nil, errors.New("opening files is not available here")
	}
	file, err := s.host.ChooseFile("Import a theme", "InkStudio theme", []string{"inktheme", "zip"})
	if err != nil || file == "" {
		return map[string]string{"name": ""}, err
	}
	name, err := s.ImportThemeFile(file)
	return map[string]string{"name": name}, err
}

// EventThemeImported carries {"name": ...} after a theme is imported from
// outside the app's own dialog, such as a double-click in the Finder.
const EventThemeImported = "theme:imported"

// ImportThemeFile imports an .inktheme into the open pack, asking before it
// replaces a template. It returns the template's name, or "" when the user
// cancels.
func (s *Studio) ImportThemeFile(file string) (string, error) {
	p := s.current()
	if p == nil {
		return "", errors.New("open a presentation first, then import the theme into it")
	}
	name, err := ThemeName(file)
	if err != nil {
		return "", err
	}
	replace := false
	if p.TemplateExists(name) {
		if s.host.Confirm == nil || !s.host.Confirm("Replace the template?",
			fmt.Sprintf("This presentation already has a template called “%s”. Replace it with the one in %s?", name, filepath.Base(file)),
			"Replace") {
			return "", nil
		}
		replace = true
	}
	if err := p.ImportTheme(file, name, replace); err != nil {
		return "", err
	}
	p.diagrams.Reset()
	return name, nil
}

// OpenPath opens what the Finder hands the app: a pack folder, or an
// .inktheme to import into the open pack.
func (s *Studio) OpenPath(path string) error {
	st, err := os.Stat(path)
	if err != nil {
		return err
	}
	if st.IsDir() {
		_, err := s.OpenDir(path)
		return err
	}
	ext := strings.ToLower(filepath.Ext(path))
	if ext == InkdeckExt {
		_, err := s.OpenDeckFile(path)
		return err
	}
	if ext != InkthemeExt && ext != ".zip" {
		return fmt.Errorf("InkStudio opens presentation folders, %s and %s files, not %s", InkdeckExt, InkthemeExt, filepath.Base(path))
	}
	name, err := s.ImportThemeFile(path)
	if err == nil && name != "" {
		s.emit(EventThemeImported, map[string]string{"name": name})
	}
	return err
}

// OpenDeckFile asks where to unpack a shared .inkdeck, next to it by
// default, unpacks a copy there and opens it. It returns nil when the user
// cancels.
func (s *Studio) OpenDeckFile(file string) (*ProjectInfo, error) {
	if s.host.ChooseDir == nil {
		return nil, errors.New("choosing folders is not available here")
	}
	dir, err := s.host.ChooseDir(
		fmt.Sprintf("Choose where to put “%s”. InkStudio makes a folder there to edit; the .inkdeck file stays as it is.", DeckName(file)),
		filepath.Dir(file), true)
	if err != nil || dir == "" {
		return nil, err
	}
	folder, err := UnpackDeck(file, dir)
	if err != nil {
		return nil, err
	}
	return s.OpenDir(folder)
}

func (s *Studio) shareDeck() (map[string]any, error) {
	p := s.current()
	if p == nil {
		return nil, errNoProject
	}
	if s.host.ChooseSave == nil {
		return nil, errors.New("saving files is not available here")
	}
	home, _ := os.UserHomeDir()
	name := filepath.Base(p.Dir)
	if info, err := p.Info(); err == nil && info.Title != "" {
		name = DeckName(info.Title + InkdeckExt)
	}
	out, err := s.host.ChooseSave("Share the presentation as one file", filepath.Join(home, "Desktop", name+InkdeckExt), "InkStudio presentation", "inkdeck")
	if err != nil || out == "" {
		return map[string]any{"path": ""}, err
	}
	if !strings.HasSuffix(out, InkdeckExt) {
		out += InkdeckExt
	}
	n, err := p.ShareDeck(out)
	if err != nil {
		return nil, err
	}
	return map[string]any{"path": out, "files": n}, nil
}

func (s *Studio) exportTheme(name string) (map[string]string, error) {
	p := s.current()
	if p == nil {
		return nil, errNoProject
	}
	if s.host.ChooseSave == nil {
		return nil, errors.New("saving files is not available here")
	}
	home, _ := os.UserHomeDir()
	out, err := s.host.ChooseSave("Export the template as a theme", filepath.Join(home, "Desktop", name+InkthemeExt), "InkStudio theme", "inktheme")
	if err != nil || out == "" {
		return map[string]string{"path": ""}, err
	}
	if !strings.HasSuffix(out, InkthemeExt) {
		out += InkthemeExt
	}
	if err := p.ExportTheme(name, out); err != nil {
		return nil, err
	}
	return map[string]string{"path": out}, nil
}

func isSlideOrSettings(rel string) bool {
	return rel == deck.SettingsFile || (strings.HasPrefix(rel, deck.SlidesDir+"/") && strings.HasSuffix(rel, ".md"))
}

// EventExportProgress carries {"done": n, "total": n} while slides render.
const EventExportProgress = "export:progress"

func (s *Studio) export(r *http.Request, f Format) (*Exported, error) {
	p := s.current()
	if p == nil {
		return nil, errNoProject
	}
	if s.host.ChooseSave == nil {
		return nil, errors.New("saving files is not available here")
	}
	out, err := s.host.ChooseSave("Export "+f.Name, p.DefaultExport(f), f.Name, f.Ext)
	if err != nil || out == "" {
		return &Exported{Warnings: []string{}}, err
	}
	if filepath.Ext(out) == "" {
		out += "." + f.Ext
	}
	if f == FormatPDF {
		return p.ExportPDF(r.Context(), out)
	}
	return p.ExportSlides(r.Context(), f, out, func(done, total int) {
		s.emit(EventExportProgress, map[string]int{"done": done, "total": total})
	})
}

func (s *Studio) render(r *http.Request) (*renderResponse, error) {
	p := s.current()
	if p == nil {
		return nil, errNoProject
	}
	var req renderRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<20)).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	pv, err := p.Render(req.Overlay)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.project != p {
		return nil, errors.New("the presentation was closed")
	}
	s.version++
	v := s.version
	s.previews[v] = pv.HTML
	delete(s.previews, v-4)
	return &renderResponse{Preview: pv, Version: v, URL: fmt.Sprintf("/preview/%d.html", v)}, nil
}

func (s *Studio) preview(w http.ResponseWriter, r *http.Request, name string) {
	v, err := strconv.Atoi(strings.TrimSuffix(name, ".html"))
	s.mu.Lock()
	doc, ok := s.previews[v]
	s.mu.Unlock()
	if err != nil || !ok {
		http.Error(w, "preview not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(doc)
}

// file serves a file of the open pack, for the preview's images, fonts and
// styles. Nothing outside the pack is served.
func (s *Studio) file(w http.ResponseWriter, r *http.Request, rest string) {
	p := s.current()
	path := filepath.FromSlash(rest)
	if runtime.GOOS != "windows" {
		path = string(filepath.Separator) + path
	}
	path = filepath.Clean(path)
	if p == nil || !p.Contains(path) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	f, err := os.Open(path)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || st.IsDir() {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeContent(w, r, st.Name(), st.ModTime().Truncate(time.Second), f)
}
