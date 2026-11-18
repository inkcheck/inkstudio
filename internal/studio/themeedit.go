package studio

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/inkcheck/inkpress/deck"
)

// The theme editor changes a theme's CSS custom properties: the colours and
// other values that a template's style.css reads. Like the other editors, it
// edits the text in place and leaves the rest of the file alone.

// ThemeProp is a custom property in a theme's CSS.
type ThemeProp struct {
	Name  string `json:"name"`
	Value string `json:"value"`
	// Color is set when the value is a plain colour.
	Color bool `json:"color"`
	// Group is the comment above it, if any: themes group their properties
	// under comments such as "Callout and box tones".
	Group string `json:"group"`
}

// PropChange sets a custom property.
type PropChange struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

var (
	propDeclRe   = regexp.MustCompile(`(--[A-Za-z0-9_-]+)(\s*:\s*)([^;{}]*?)(\s*)(;|\})`)
	propNameRe   = regexp.MustCompile(`^--[A-Za-z0-9_-]+$`)
	commentSpans = regexp.MustCompile(`(?s)/\*.*?\*/`)
)

// inComment reports whether offset i falls inside a CSS comment.
func inComment(spans [][]int, i int) bool {
	for _, s := range spans {
		if i >= s[0] && i < s[1] {
			return true
		}
	}
	return false
}

// ThemeProps lists the custom properties in a theme's CSS, in order, each
// once (the first declaration wins, as the editor changes the first).
func ThemeProps(css string) []ThemeProp {
	comments := commentSpans.FindAllStringIndex(css, -1)
	seen := map[string]bool{}
	props := []ThemeProp{}
	group := ""
	last := 0
	for _, m := range propDeclRe.FindAllStringSubmatchIndex(css, -1) {
		if inComment(comments, m[0]) {
			continue
		}
		// The nearest comment since the previous declaration names a group.
		for _, c := range comments {
			if c[0] >= last && c[1] <= m[0] {
				group = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(css[c[0]:c[1]], "/*"), "*/"))
				if i := strings.IndexAny(group, ".:"); i > 0 {
					group = group[:i]
				}
			}
		}
		last = m[1]
		name, value := css[m[2]:m[3]], css[m[6]:m[7]]
		if seen[name] {
			continue
		}
		seen[name] = true
		props = append(props, ThemeProp{Name: name, Value: value, Color: colorRe.MatchString(value), Group: group})
	}
	return props
}

// EditTheme sets custom properties in a theme's CSS. A property the file
// does not declare is added to the first rule.
func EditTheme(css string, changes []PropChange) (string, error) {
	for _, c := range changes {
		if !propNameRe.MatchString(c.Name) {
			return "", fmt.Errorf("bad property name %q", c.Name)
		}
		v := strings.TrimSpace(c.Value)
		if v == "" || strings.ContainsAny(v, ";{}") || strings.Contains(v, "/*") {
			return "", fmt.Errorf("%s: %q is not a value this editor can write", c.Name, c.Value)
		}
		comments := commentSpans.FindAllStringIndex(css, -1)
		done := false
		for _, m := range propDeclRe.FindAllStringSubmatchIndex(css, -1) {
			if inComment(comments, m[0]) || css[m[2]:m[3]] != c.Name {
				continue
			}
			css = css[:m[6]] + v + css[m[7]:]
			done = true
			break
		}
		if done {
			continue
		}
		open := -1
		for i := 0; i < len(css); i++ {
			if css[i] == '{' && !inComment(comments, i) {
				open = i
				break
			}
		}
		if open < 0 {
			css = strings.TrimRight(css, "\n") + "\n.slide {\n  " + c.Name + ": " + v + ";\n}\n"
			continue
		}
		// After the rule's last declaration, with its indentation.
		end := strings.IndexByte(css[open:], '}')
		if end < 0 {
			return "", fmt.Errorf("the theme's CSS has an unclosed rule")
		}
		end += open
		body := strings.TrimRight(css[open+1:end], " \t\n")
		indent := "  "
		if lines := strings.Split(body, "\n"); len(lines) > 1 {
			l := lines[len(lines)-1]
			indent = l[:len(l)-len(strings.TrimLeft(l, " \t"))]
		}
		sep := ""
		if t := strings.TrimSpace(body); t != "" && !strings.HasSuffix(t, ";") && !strings.HasSuffix(t, "*/") {
			sep = ";"
		}
		css = css[:open+1] + body + sep + "\n" + indent + c.Name + ": " + v + ";\n" + css[end:]
	}
	return css, nil
}

// DuplicateTheme copies a template's theme to a new theme called name.
func (p *Project) DuplicateTheme(template, theme, name string) error {
	for _, n := range []string{template, theme, name} {
		if !templateNameRe.MatchString(n) {
			return fmt.Errorf("%q is not a usable name: use letters, digits, '-', '_' and '.'", n)
		}
	}
	dir := filepath.Join(p.Dir, deck.TemplatesDir, template, "themes")
	src, err := os.ReadFile(filepath.Join(dir, theme+".css"))
	if err != nil {
		return err
	}
	out := filepath.Join(dir, name+".css")
	f, err := os.OpenFile(out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if os.IsExist(err) {
		return fmt.Errorf("the %s template already has a theme called %q", template, name)
	}
	if err != nil {
		return err
	}
	header := fmt.Sprintf("/* %s: made from %s in inkstudio. */\n", name, theme)
	if _, err := f.WriteString(header + string(src)); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
