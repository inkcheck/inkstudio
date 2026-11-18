package studio

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"
)

// The inspector changes settings.toml and slide frontmatter by editing the
// text, line by line, rather than decoding and encoding it, so comments,
// order and formatting survive. Each edit is checked by parsing the result.

// Change sets a key, or removes it when Value is nil.
type Change struct {
	// Table is the TOML table, such as "page"; "" is the top level. It is
	// ignored for frontmatter.
	Table string `json:"table"`
	Key   string `json:"key"`
	Value any    `json:"value"`
}

var keyRe = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// EditSettings applies changes to the text of a settings.toml.
func EditSettings(text string, changes []Change) (string, error) {
	lines := splitLines(strings.ReplaceAll(text, "\r\n", "\n"))
	for _, c := range changes {
		if !keyRe.MatchString(c.Key) || (c.Table != "" && !keyRe.MatchString(c.Table)) {
			return "", fmt.Errorf("bad key %q", c.Table+"."+c.Key)
		}
		var err error
		if lines, err = setTOML(lines, c); err != nil {
			return "", err
		}
	}
	out := joinLines(lines)
	var check map[string]any
	if _, err := toml.Decode(out, &check); err != nil {
		return "", fmt.Errorf("the edit would break settings.toml: %w", err)
	}
	return out, nil
}

var tableRe = regexp.MustCompile(`^\s*\[\s*([A-Za-z0-9_.-]+)\s*\]`)

// setTOML sets or removes a key in a table. The table's range runs from its
// header to the next header; the top level ends at the first header.
func setTOML(lines []string, c Change) ([]string, error) {
	start, end := 0, len(lines)
	found := c.Table == ""
	for i, l := range lines {
		m := tableRe.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		if found {
			end = i
			break
		}
		if m[1] == c.Table {
			found = true
			start = i + 1
		}
	}
	if !found {
		if c.Value == nil {
			return lines, nil
		}
		v, err := tomlValue(c.Value)
		if err != nil {
			return nil, err
		}
		for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
			lines = lines[:len(lines)-1]
		}
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		return append(lines, "["+c.Table+"]", c.Key+" = "+v), nil
	}

	keyLine := regexp.MustCompile(`^\s*` + regexp.QuoteMeta(c.Key) + `\s*=`)
	at := -1
	for i := start; i < end; i++ {
		if keyLine.MatchString(lines[i]) {
			at = i
			break
		}
	}
	if c.Value == nil {
		if at >= 0 {
			if valueSpansLines(lines[at]) {
				return nil, fmt.Errorf("%s spans several lines; change it in the editor", c.Key)
			}
			lines = append(lines[:at], lines[at+1:]...)
		}
		return lines, nil
	}
	v, err := tomlValue(c.Value)
	if err != nil {
		return nil, err
	}
	if at >= 0 {
		if valueSpansLines(lines[at]) {
			return nil, fmt.Errorf("%s spans several lines; change it in the editor", c.Key)
		}
		lines[at] = c.Key + " = " + v + trailingComment(lines[at])
		return lines, nil
	}
	// After the table's last key, so it joins its neighbours.
	insert := start
	for i := start; i < end; i++ {
		t := strings.TrimSpace(lines[i])
		if t != "" && !strings.HasPrefix(t, "#") {
			insert = i + 1
		}
	}
	return append(lines[:insert], append([]string{c.Key + " = " + v}, lines[insert:]...)...), nil
}

// valueSpansLines reports whether a key = value line opens a multi-line
// string or array.
func valueSpansLines(line string) bool {
	_, v, _ := strings.Cut(line, "=")
	v = strings.TrimSpace(v)
	if strings.HasPrefix(v, `"""`) || strings.HasPrefix(v, `'''`) {
		return strings.Count(v, v[:3]) < 2
	}
	if strings.HasPrefix(v, "[") || strings.HasPrefix(v, "{") {
		var x map[string]any
		_, err := toml.Decode("x = "+v, &x)
		return err != nil
	}
	return false
}

// trailingComment returns " # ..." at the end of a simple key = value line.
func trailingComment(line string) string {
	inString := byte(0)
	for i := 0; i < len(line); i++ {
		ch := line[i]
		switch {
		case inString != 0:
			if ch == '\\' && inString == '"' {
				i++
			} else if ch == inString {
				inString = 0
			}
		case ch == '"' || ch == '\'':
			inString = ch
		case ch == '#':
			return "  " + strings.TrimSpace(line[i:])
		}
	}
	return ""
}

func tomlValue(v any) (string, error) {
	switch x := v.(type) {
	case string:
		return jsonString(x), nil
	case bool:
		return strconv.FormatBool(x), nil
	case float64:
		if x == float64(int64(x)) {
			return strconv.FormatInt(int64(x), 10), nil
		}
		return strconv.FormatFloat(x, 'f', -1, 64), nil
	case int:
		return strconv.Itoa(x), nil
	}
	return "", fmt.Errorf("cannot write %T to settings", v)
}

// jsonString quotes s as JSON, which is also a TOML basic string and a YAML
// double-quoted string.
func jsonString(s string) string {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.Encode(s)
	return strings.TrimSpace(b.String())
}

// EditFrontmatter applies changes to a slide's frontmatter, adding a YAML
// frontmatter block when the slide has none.
func EditFrontmatter(text string, changes []Change) (string, error) {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	lines := splitLines(text)
	fence := ""
	end := -1
	if len(lines) > 0 && (lines[0] == "---" || lines[0] == "+++") {
		fence = lines[0]
		for i := 1; i < len(lines); i++ {
			if strings.TrimRight(lines[i], " \t") == fence {
				end = i
				break
			}
		}
		if end < 0 {
			return "", fmt.Errorf("the frontmatter has no closing %s", fence)
		}
	}
	if fence == "" {
		if allRemovals(changes) {
			return text, nil
		}
		lines = append([]string{"---", "---"}, lines...)
		fence, end = "---", 1
	}
	fm := append([]string(nil), lines[1:end]...)
	for _, c := range changes {
		if !keyRe.MatchString(c.Key) {
			return "", fmt.Errorf("bad key %q", c.Key)
		}
		var err error
		if fence == "+++" {
			fm, err = setTOML(fm, Change{Key: c.Key, Value: c.Value})
		} else {
			fm, err = setYAML(fm, c)
		}
		if err != nil {
			return "", err
		}
	}
	var check map[string]any
	var err error
	if fence == "+++" {
		_, err = toml.Decode(strings.Join(fm, "\n"), &check)
	} else {
		err = yaml.Unmarshal([]byte(strings.Join(fm, "\n")), &check)
	}
	if err != nil {
		return "", fmt.Errorf("the edit would break the frontmatter: %w", err)
	}
	rest := lines[end+1:]
	if len(fm) == 0 {
		// Nothing left: drop the empty block, and the blank line after it.
		for len(rest) > 0 && strings.TrimSpace(rest[0]) == "" {
			rest = rest[1:]
		}
		return joinLines(rest), nil
	}
	out := append(append(append([]string{fence}, fm...), fence), rest...)
	return joinLines(out), nil
}

func allRemovals(changes []Change) bool {
	for _, c := range changes {
		if c.Value != nil {
			return false
		}
	}
	return true
}

// setYAML sets or removes a top-level key. A value that continues on
// indented lines is replaced with it.
func setYAML(lines []string, c Change) ([]string, error) {
	keyLine := regexp.MustCompile(`^` + regexp.QuoteMeta(c.Key) + `\s*:`)
	at, next := -1, -1
	for i, l := range lines {
		if keyLine.MatchString(l) {
			at, next = i, i+1
			for next < len(lines) && (strings.HasPrefix(lines[next], " ") || strings.HasPrefix(lines[next], "\t") || strings.HasPrefix(lines[next], "- ")) {
				next++
			}
			break
		}
	}
	if c.Value == nil {
		if at >= 0 {
			lines = append(lines[:at], lines[next:]...)
		}
		return lines, nil
	}
	v, err := yamlValue(c.Value)
	if err != nil {
		return nil, err
	}
	line := c.Key + ": " + v
	if at >= 0 {
		return append(lines[:at], append([]string{line}, lines[next:]...)...), nil
	}
	return append(lines, line), nil
}

var plainYAML = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_ ./()+-]*$`)

func yamlValue(v any) (string, error) {
	switch x := v.(type) {
	case string:
		switch strings.ToLower(x) {
		case "true", "false", "yes", "no", "on", "off", "null", "~", "y", "n":
			return jsonString(x), nil
		}
		if plainYAML.MatchString(x) && !strings.HasSuffix(x, " ") {
			return x, nil
		}
		return jsonString(x), nil
	case bool:
		return strconv.FormatBool(x), nil
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64), nil
	}
	return "", fmt.Errorf("cannot write %T to frontmatter", v)
}

func splitLines(text string) []string {
	text = strings.TrimSuffix(text, "\n")
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}

func joinLines(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}
