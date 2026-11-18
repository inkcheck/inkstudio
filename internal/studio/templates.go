package studio

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/inkcheck/inkpress/deck"
	"github.com/inkcheck/inkpress/templates"
)

// TemplateInfo describes a template pack in the pack's .templates/.
type TemplateInfo struct {
	Name          string      `json:"name"`
	Description   string      `json:"description"`
	DefaultLayout string      `json:"defaultLayout"`
	DefaultTheme  string      `json:"defaultTheme"`
	Layouts       []string    `json:"layouts"`
	Themes        []ThemeInfo `json:"themes"`
	// Error is set when the template does not load; the rest may be empty.
	Error string `json:"error,omitempty"`
}

// ThemeInfo is a theme and the colours its CSS sets, for swatches.
type ThemeInfo struct {
	Name   string     `json:"name"`
	Colors []ThemeVar `json:"colors"`
}

// ThemeVar is a CSS custom property.
type ThemeVar struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// DeckSettings is what the inspector shows of settings.toml.
type DeckSettings struct {
	Title    string  `json:"title"`
	Author   string  `json:"author"`
	Date     string  `json:"date"`
	Template string  `json:"template"`
	Theme    string  `json:"theme"`
	Aspect   string  `json:"aspect"`
	Width    float64 `json:"width"`
	Height   float64 `json:"height"`
	Output   string  `json:"output"`
	// Error is set when settings.toml does not parse.
	Error string `json:"error,omitempty"`
}

// ParseDeckSettings reads settings.toml text for the inspector.
func ParseDeckSettings(text string) DeckSettings {
	s, err := deck.ParseSettings(text)
	if err != nil {
		return DeckSettings{Error: err.Error()}
	}
	return DeckSettings{
		Title: s.Title, Author: s.Author, Date: s.Date,
		Template: s.Template, Theme: s.Theme,
		Aspect: s.Page.Aspect, Width: s.Page.Width, Height: s.Page.Height,
		Output: s.Output,
	}
}

// Templates lists the template packs in the pack's .templates/.
func (p *Project) Templates() ([]TemplateInfo, error) {
	dir := filepath.Join(p.Dir, deck.TemplatesDir)
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return []TemplateInfo{}, nil
	}
	if err != nil {
		return nil, err
	}
	list := []TemplateInfo{}
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		info := TemplateInfo{Name: e.Name(), Layouts: []string{}, Themes: []ThemeInfo{}}
		t, err := templates.Load(dir, e.Name())
		if err != nil {
			info.Error = err.Error()
			list = append(list, info)
			continue
		}
		info.Description = t.Config.Description
		info.DefaultLayout = t.Config.Layout
		info.DefaultTheme = t.Config.Theme
		info.Layouts = t.Layouts()
		for _, th := range t.Themes() {
			css, _ := os.ReadFile(filepath.Join(t.Dir, "themes", th+".css"))
			info.Themes = append(info.Themes, ThemeInfo{Name: th, Colors: themeColors(string(css))})
		}
		list = append(list, info)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	return list, nil
}

var (
	cssVarRe     = regexp.MustCompile(`(--[A-Za-z0-9_-]+)\s*:\s*([^;{}]+?)\s*(?:;|})`)
	cssCommentRe = regexp.MustCompile(`(?s)/\*.*?\*/`)
	colorRe      = regexp.MustCompile(`^(#[0-9A-Fa-f]{3,8}|(rgb|rgba|hsl|hsla|oklch|oklab|lab|lch|color)\([^()]*\))$`)
)

// themeColors returns the custom properties in a theme's CSS whose values
// are plain colours, in order, without repeats.
func themeColors(css string) []ThemeVar {
	css = cssCommentRe.ReplaceAllString(css, "")
	seen := map[string]bool{}
	vars := []ThemeVar{}
	for _, m := range cssVarRe.FindAllStringSubmatch(css, -1) {
		if seen[m[1]] || !colorRe.MatchString(m[2]) {
			continue
		}
		seen[m[1]] = true
		vars = append(vars, ThemeVar{Name: m[1], Value: m[2]})
	}
	return vars
}
