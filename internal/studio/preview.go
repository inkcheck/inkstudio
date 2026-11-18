package studio

import (
	"bytes"
	"os"
	"path/filepath"

	"github.com/inkcheck/inkpress/deck"
	"github.com/inkcheck/inkpress/render"
)

// FSPrefix is where the asset server serves the pack's files. inkpress
// writes file: URLs, which a page served by the webview cannot load, so the
// preview rewrites them to this prefix.
const FSPrefix = "/@fs/"

// SlideInfo describes one rendered slide.
type SlideInfo struct {
	Number int    `json:"number"`
	Source string `json:"source"`
	Title  string `json:"title"`
	// Template, Layout and Theme are what the slide got, defaults included.
	Template string `json:"template"`
	Layout   string `json:"layout"`
	Theme    string `json:"theme"`
	// Meta is the frontmatter, and Notes the tailmatter.
	Meta  map[string]any `json:"meta"`
	Notes string         `json:"notes"`
}

// Preview is a rendered deck.
type Preview struct {
	HTML     []byte      `json:"-"`
	Width    float64     `json:"width"`
	Height   float64     `json:"height"`
	Slides   []SlideInfo `json:"slides"`
	Warnings []string    `json:"warnings"`
}

// Render renders the pack. overlay maps paths relative to the pack to
// unsaved text, which is used instead of the file on disk.
func (p *Project) Render(overlay map[string]string) (*Preview, error) {
	abs := map[string]string{}
	for rel, text := range overlay {
		if a, err := p.Abs(rel); err == nil {
			abs[a] = text
		}
	}
	d, err := deck.LoadFunc(p.Dir, func(path string) ([]byte, error) {
		if text, ok := abs[path]; ok {
			return []byte(text), nil
		}
		return os.ReadFile(path)
	})
	if err != nil {
		return nil, err
	}
	res, err := render.HTMLWith(d, render.Options{Diagrams: p.diagrams})
	if err != nil {
		return nil, err
	}
	pv := &Preview{
		HTML:     previewHTML(res.HTML),
		Width:    res.Width,
		Height:   res.Height,
		Warnings: res.Warnings,
	}
	if pv.Warnings == nil {
		pv.Warnings = []string{}
	}
	for i, r := range res.Slides {
		s := d.Slides[i]
		pv.Slides = append(pv.Slides, SlideInfo{
			Number:   r.Number,
			Source:   filepath.ToSlash(r.Source),
			Title:    r.Title,
			Template: r.Template,
			Layout:   r.Layout,
			Theme:    r.Theme,
			Meta:     s.Meta,
			Notes:    s.Notes,
		})
	}
	return pv, nil
}

// previewHTML points file: URLs at the asset server and adds the script
// that shows one slide at a time, scaled to fit the frame.
func previewHTML(doc []byte) []byte {
	doc = bytes.ReplaceAll(doc, []byte("file:///"), []byte(FSPrefix))
	return bytes.Replace(doc, []byte("</head>"), []byte(previewHead+"</head>"), 1)
}

// previewHead runs inside the preview frame, which has two modes.
//
// Normally it shows one slide, scaled to fit. In strip mode it shows every
// slide as a column of thumbnails, marks the current one, posts
// {inkstudio: "select", slide: n} to the parent on a click and
// {inkstudio: "key", key} for arrow keys.
//
// The app sets window.studioMode ("slide" or "strip") and window.studioSlide
// before this runs, and calls window.studioShow(n) to change slides. A page
// opened by URL reads ?strip and #n instead.
const previewHead = `<style>
html, body { background: transparent !important; padding: 0 !important; margin: 0 !important; }
body.studio { overflow: hidden; }
body.studio .page { display: none; margin: 0 !important; box-shadow: none !important;
  position: absolute; left: 0; top: 0; transform-origin: 0 0; }
body.studio .page.studio-current { display: block; }
html:has(body.studio-strip) { scroll-padding: 12px 0; }
body.studio-strip { overflow-x: hidden; overflow-y: auto; padding: 12px 12px 12px 34px !important; }
/* A thumbnail is a box the size of the scaled slide; the slide inside is
   scaled with a transform, which every engine draws the same way. */
.studio-thumb { position: relative; margin: 0 0 12px; border-radius: 2px; cursor: default;
  box-shadow: 0 1px 4px rgb(0 0 0 / 0.35); outline: 2px solid transparent; outline-offset: 3px; }
.studio-thumb::before { content: attr(data-n); position: absolute; right: calc(100% + 10px); top: 0;
  font: 600 12px / 1 -apple-system, system-ui, sans-serif; color: #8a8a93; }
.studio-thumb.studio-current { outline-color: #4f8bff; }
.studio-thumb.studio-current::before { color: #4f8bff; }
.studio-thumb-clip { position: absolute; inset: 0; overflow: hidden; border-radius: 2px; }
body.studio-strip .page { position: absolute !important; left: 0; top: 0; margin: 0 !important;
  box-shadow: none !important; transform-origin: 0 0; }
body.studio-strip .page > .slide { pointer-events: none; }
</style>
<script>
(function () {
  var strip = window.studioMode ? window.studioMode === 'strip' : new URLSearchParams(location.search).has('strip');
  function current() {
    var pages = document.querySelectorAll('.page');
    var n = window.studioSlide || parseInt(location.hash.slice(1), 10) || 1;
    return { pages: pages, n: Math.min(Math.max(n, 1), pages.length) };
  }
  function show() {
    var c = current();
    if (!c.pages.length) return;
    var page = c.pages[c.n - 1];
    c.pages.forEach(function (p) { p.classList.toggle('studio-current', p === page); });
    if (strip) {
      var root = getComputedStyle(document.documentElement);
      var w = parseFloat(root.getPropertyValue('--slide-width')) || 1280;
      var h = parseFloat(root.getPropertyValue('--slide-height')) || 720;
      var s = Math.max((document.documentElement.clientWidth - 46) / w, 0.02);
      c.pages.forEach(function (p) {
        var thumb = p.parentNode.parentNode;
        thumb.style.width = w * s + 'px';
        thumb.style.height = h * s + 'px';
        thumb.classList.toggle('studio-current', p === page);
        p.style.transform = 'scale(' + s + ')';
      });
      page.parentNode.parentNode.scrollIntoView({ block: 'nearest' });
      return;
    }
    var w = page.offsetWidth, h = page.offsetHeight;
    var s = Math.min(innerWidth / w, innerHeight / h);
    page.style.transform = 'translate(' + (innerWidth - w * s) / 2 + 'px,' + (innerHeight - h * s) / 2 + 'px) scale(' + s + ')';
  }
  document.addEventListener('DOMContentLoaded', function () {
    document.body.classList.add(strip ? 'studio-strip' : 'studio');
    if (strip) {
      // Wrap each slide: a thumbnail box, and a clip inside it, so the
      // slide number can sit outside the box.
      document.querySelectorAll('.page').forEach(function (p, i) {
        var thumb = document.createElement('div');
        thumb.className = 'studio-thumb';
        thumb.setAttribute('data-n', i + 1);
        var clip = document.createElement('div');
        clip.className = 'studio-thumb-clip';
        p.parentNode.insertBefore(thumb, p);
        thumb.appendChild(clip);
        clip.appendChild(p);
      });
    }
    show();
    if (!strip) return;
    document.addEventListener('click', function (e) {
      var thumb = e.target.closest && e.target.closest('.studio-thumb');
      if (!thumb) return;
      var n = Array.prototype.indexOf.call(document.querySelectorAll('.studio-thumb'), thumb) + 1;
      parent.postMessage({ inkstudio: 'select', slide: n }, '*');
    });
    document.addEventListener('keydown', function (e) {
      if (/^(Arrow|Page|Home|End)/.test(e.key)) {
        e.preventDefault();
        parent.postMessage({ inkstudio: 'key', key: e.key }, '*');
      }
    });
  });
  window.studioShow = function (n) { window.studioSlide = n; show(); };
  addEventListener('hashchange', function () { window.studioSlide = 0; show(); });
  addEventListener('resize', show);
})();
</script>
`
