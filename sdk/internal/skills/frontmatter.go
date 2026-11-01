package skills

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// frontmatter extraction follows Pi's utils/frontmatter.ts exactly: newlines are
// normalised, a byte-order mark is dropped, the closing fence is the first
// "\n---" at or after offset 3, the YAML slice starts at offset 4, and the body
// is trimmed on both ends. A file with no frontmatter (or an unterminated one)
// yields an empty map and the untouched body.
func extractFrontmatter(content string) (yamlString string, body string, ok bool) {
	normalized := normalizeNewlines(stripBOM(content))
	if !strings.HasPrefix(normalized, "---") {
		return "", normalized, false
	}
	end := strings.Index(normalized[3:], "\n---")
	if end < 0 {
		return "", normalized, false
	}
	end += 3
	return normalized[4:end], strings.TrimSpace(normalized[end+4:]), true
}

func normalizeNewlines(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.ReplaceAll(s, "\r", "\n")
}

func stripBOM(s string) string { return strings.TrimPrefix(s, "\ufeff") }

// ParseFrontmatter splits a markdown file into its frontmatter map and body.
// Values are typed the way YAML 1.2 core types them, because callers depend on
// it: Pi only accepts a skill name or description that is actually a string,
// and only hides a skill when "disable-model-invocation" is the boolean true.
//
// This is a deliberate subset of YAML, covering what a SKILL.md frontmatter
// realistically contains: plain, single- and double-quoted scalars, literal (|)
// and folded (>) block scalars, comments, and booleans/numbers/null. Nested
// maps and sequences are parsed past rather than into, since nulo consumes no
// nested field (Pi's `metadata` is only carried, never read).
func ParseFrontmatter(content string) (map[string]any, string, error) {
	yamlString, body, ok := extractFrontmatter(content)
	if !ok {
		return map[string]any{}, body, nil
	}
	fm, err := parseYAMLMap(yamlString)
	if err != nil {
		return nil, body, err
	}
	return fm, body, nil
}

// StripFrontmatter returns the body with the frontmatter removed, mirroring
// Pi's stripFrontmatter.
func StripFrontmatter(content string) string {
	_, body, err := ParseFrontmatter(content)
	if err != nil {
		// A malformed frontmatter block is still a frontmatter block; drop it
		// rather than handing the fences to the model.
		if _, b, ok := extractFrontmatter(content); ok {
			return b
		}
	}
	return body
}

func parseYAMLMap(src string) (map[string]any, error) {
	fm := map[string]any{}
	lines := strings.Split(src, "\n")
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		// Indented lines belong to a nested value we do not model.
		if line != trimmed {
			continue
		}
		key, raw, found := strings.Cut(line, ":")
		if !found {
			return nil, fmt.Errorf("invalid frontmatter line %q", line)
		}
		key = strings.TrimSpace(key)
		if key == "" {
			return nil, fmt.Errorf("invalid frontmatter key in %q", line)
		}
		raw = strings.TrimSpace(raw)

		switch {
		case raw == "|" || raw == "|-" || raw == "|+" || raw == ">" || raw == ">-" || raw == ">+":
			block, next := readBlockScalar(lines, i+1, raw[0] == '>')
			fm[key] = block
			i = next - 1
		default:
			v, err := parseScalar(raw)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", key, err)
			}
			fm[key] = v
			if isBlockStart(raw) {
				// `key:` with nothing after it introduces a nested map/list.
				// Skip its indented body so its keys are not mistaken for ours.
				for i+1 < len(lines) {
					n := lines[i+1]
					if strings.TrimSpace(n) == "" || (n != strings.TrimSpace(n)) {
						i++
						continue
					}
					break
				}
			}
		}
	}
	return fm, nil
}

func isBlockStart(raw string) bool {
	return raw == "" || strings.HasPrefix(raw, "&") || strings.HasPrefix(raw, "!")
}

// readBlockScalar consumes the indented lines belonging to a `|` or `>` value,
// starting at index start, and returns the value plus the index of the first
// line after the block.
func readBlockScalar(lines []string, start int, folded bool) (string, int) {
	var block []string
	indent := -1
	i := start
	for ; i < len(lines); i++ {
		line := lines[i]
		if strings.TrimSpace(line) == "" {
			block = append(block, "")
			continue
		}
		cur := len(line) - len(strings.TrimLeft(line, " \t"))
		if indent < 0 {
			indent = cur
		}
		if cur < indent {
			break
		}
		block = append(block, line[indent:])
	}
	// Trailing blank lines are not part of the value.
	for len(block) > 0 && block[len(block)-1] == "" {
		block = block[:len(block)-1]
	}
	if folded {
		return strings.Join(block, " "), i
	}
	return strings.Join(block, "\n"), i
}

func parseScalar(raw string) (any, error) {
	if raw == "" {
		return nil, nil
	}
	if strings.HasPrefix(raw, "\"") {
		return parseDoubleQuoted(raw)
	}
	if strings.HasPrefix(raw, "'") {
		return parseSingleQuoted(raw)
	}
	// Strip a trailing comment only when it is unquoted and preceded by space.
	if idx := strings.Index(raw, " #"); idx >= 0 {
		raw = strings.TrimSpace(raw[:idx])
	}
	if idx := strings.Index(raw, "\t#"); idx >= 0 {
		raw = strings.TrimSpace(raw[:idx])
	}
	switch raw {
	case "true", "True", "TRUE":
		return true, nil
	case "false", "False", "FALSE":
		return false, nil
	case "null", "Null", "NULL", "~":
		return nil, nil
	}
	if n, err := strconv.ParseInt(raw, 10, 64); err == nil {
		return n, nil
	}
	if f, err := strconv.ParseFloat(raw, 64); err == nil && strings.ContainsAny(raw, ".eE") {
		return f, nil
	}
	return raw, nil
}

func parseDoubleQuoted(raw string) (string, error) {
	var b strings.Builder
	for i := 1; i < len(raw); i++ {
		c := raw[i]
		switch c {
		case '"':
			if rest := strings.TrimSpace(raw[i+1:]); rest != "" && !strings.HasPrefix(rest, "#") {
				return "", errors.New("trailing content after closing quote")
			}
			return b.String(), nil
		case '\\':
			if i+1 >= len(raw) {
				return "", errors.New("unterminated escape")
			}
			i++
			switch raw[i] {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case 'r':
				b.WriteByte('\r')
			case '0':
				b.WriteByte(0)
			case '"', '\\', '/':
				b.WriteByte(raw[i])
			case 'u':
				if i+4 >= len(raw) {
					return "", errors.New("truncated \\u escape")
				}
				code, err := strconv.ParseUint(raw[i+1:i+5], 16, 32)
				if err != nil {
					return "", errors.New("invalid \\u escape")
				}
				b.WriteRune(rune(code))
				i += 4
			default:
				b.WriteByte(raw[i])
			}
		default:
			b.WriteByte(c)
		}
	}
	return "", errors.New("unterminated double-quoted string")
}

func parseSingleQuoted(raw string) (string, error) {
	var b strings.Builder
	for i := 1; i < len(raw); i++ {
		if raw[i] == '\'' {
			if i+1 < len(raw) && raw[i+1] == '\'' {
				b.WriteByte('\'')
				i++
				continue
			}
			if rest := strings.TrimSpace(raw[i+1:]); rest != "" && !strings.HasPrefix(rest, "#") {
				return "", errors.New("trailing content after closing quote")
			}
			return b.String(), nil
		}
		b.WriteByte(raw[i])
	}
	return "", errors.New("unterminated single-quoted string")
}

// truncateRunes shortens s to at most max runes, for diagnostics that quote a
// value back to the user.
func truncateRunes(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	r := []rune(s)
	return string(r[:max]) + "…"
}
