package skills

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Pi honours a gitignore-style ignore file in every directory it walks, using
// the same three names the fd/find tooling does. Patterns are anchored to the
// directory that declares them by prefixing them with its path relative to the
// skill root, which is what Pi's prefixIgnorePattern does.
var ignoreFileNames = []string{".gitignore", ".ignore", ".fdignore"}

type ignoreMatcher struct {
	patterns []ignorePattern
}

type ignorePattern struct {
	negated bool
	dirOnly bool
	re      *regexp.Regexp
}

func (m *ignoreMatcher) addFromDir(dir, root string) {
	prefix := ""
	if rel, err := filepath.Rel(root, dir); err == nil && rel != "." && rel != "" {
		prefix = filepath.ToSlash(rel) + "/"
	}
	for _, name := range ignoreFileNames {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		for _, line := range strings.Split(normalizeNewlines(string(b)), "\n") {
			pat, ok := prefixIgnorePattern(line, prefix)
			if !ok {
				continue
			}
			if p, err := compileIgnorePattern(pat); err == nil {
				m.patterns = append(m.patterns, *p)
			}
		}
	}
}

// prefixIgnorePattern mirrors Pi: blank lines and comments are dropped, `\#`
// and `\!` unescape, a leading `!` negates, a leading `/` is stripped (Pi does
// not preserve anchoring there either), and the rest is prefixed with the
// declaring directory.
func prefixIgnorePattern(line, prefix string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return "", false
	}
	if strings.HasPrefix(trimmed, "#") && !strings.HasPrefix(trimmed, `\#`) {
		return "", false
	}
	pattern := line
	negated := false
	switch {
	case strings.HasPrefix(pattern, "!"):
		negated = true
		pattern = pattern[1:]
	case strings.HasPrefix(pattern, `\!`):
		pattern = pattern[1:]
	}
	if strings.HasPrefix(pattern, "/") {
		pattern = pattern[1:]
	}
	if prefix != "" {
		pattern = prefix + pattern
	}
	if negated {
		return "!" + pattern, true
	}
	return pattern, true
}

func compileIgnorePattern(pat string) (*ignorePattern, error) {
	p := &ignorePattern{}
	if strings.HasPrefix(pat, "!") {
		p.negated = true
		pat = pat[1:]
	}
	pat = strings.TrimRight(pat, " \t")
	if strings.HasSuffix(pat, "/") {
		p.dirOnly = true
		pat = strings.TrimSuffix(pat, "/")
	}
	if pat == "" {
		return nil, os.ErrInvalid
	}
	body := globToRegex(pat)
	expr := "^" + body + "$"
	if !strings.Contains(pat, "/") {
		// A bare name matches at any depth, the way gitignore treats it.
		expr = "(?:^|.*/)" + body + "$"
	}
	re, err := regexp.Compile(expr)
	if err != nil {
		return nil, err
	}
	p.re = re
	return p, nil
}

func globToRegex(glob string) string {
	var b strings.Builder
	for i := 0; i < len(glob); i++ {
		switch c := glob[i]; c {
		case '*':
			if i+1 < len(glob) && glob[i+1] == '*' {
				i++
				if i+1 < len(glob) && glob[i+1] == '/' {
					i++
					b.WriteString("(?:.*/)?")
				} else {
					b.WriteString(".*")
				}
			} else {
				b.WriteString("[^/]*")
			}
		case '?':
			b.WriteString("[^/]")
		case '[':
			j := i + 1
			if j < len(glob) && (glob[j] == '!' || glob[j] == '^') {
				j++
			}
			if j < len(glob) && glob[j] == ']' {
				j++
			}
			for j < len(glob) && glob[j] != ']' {
				j++
			}
			if j >= len(glob) {
				b.WriteString(`\[`)
				continue
			}
			class := glob[i+1 : j]
			if strings.HasPrefix(class, "!") {
				class = "^" + class[1:]
			}
			b.WriteString("[" + class + "]")
			i = j
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	return b.String()
}

// ignores reports whether rel is excluded. Every ancestor is evaluated as a
// directory because gitignore excludes the contents of a matched directory, and
// the last matching pattern wins so a later `!` can re-include a path.
func (m *ignoreMatcher) ignores(rel string, isDir bool) bool {
	rel = strings.Trim(rel, "/")
	if rel == "" || len(m.patterns) == 0 {
		return false
	}
	segs := strings.Split(rel, "/")
	ignored := false
	for i := 1; i <= len(segs); i++ {
		prefix := strings.Join(segs[:i], "/")
		segIsDir := i < len(segs) || isDir
		for _, p := range m.patterns {
			if p.dirOnly && !segIsDir {
				continue
			}
			if p.re.MatchString(prefix) {
				ignored = !p.negated
			}
		}
	}
	return ignored
}
