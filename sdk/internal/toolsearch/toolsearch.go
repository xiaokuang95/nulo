// Package toolsearch ranks tools by relevance. PiG shares one BM25 ranker between
// the `tool_search` tool and codemode's `searchTools()`, so nulo does too.
package toolsearch

import (
	"encoding/json"
	"math"
	"regexp"
	"sort"
	"strings"
)

var stopWords = map[string]bool{
	"a": true, "an": true, "and": true, "are": true, "as": true, "at": true,
	"be": true, "by": true, "for": true, "from": true, "in": true, "is": true,
	"it": true, "of": true, "on": true, "or": true, "that": true, "the": true,
	"this": true, "to": true, "with": true,
}

var (
	camelLowerRe = regexp.MustCompile(`([a-z0-9])([A-Z])`)
	camelUpperRe = regexp.MustCompile(`([A-Z]+)([A-Z][a-z])`)
	nonAlnumRe   = regexp.MustCompile(`[^a-z0-9]+`)
)

// Stem is the naive singular form Pi uses, so "issues" matches "issue".
func Stem(term string) string {
	switch {
	case len(term) > 4 && strings.HasSuffix(term, "ies"):
		return term[:len(term)-3] + "y"
	case len(term) > 4 && (strings.HasSuffix(term, "ches") || strings.HasSuffix(term, "shes") ||
		strings.HasSuffix(term, "sses") || strings.HasSuffix(term, "xes") || strings.HasSuffix(term, "zes")):
		return term[:len(term)-2]
	case len(term) > 3 && strings.HasSuffix(term, "s") && !strings.HasSuffix(term, "ss"):
		return term[:len(term)-1]
	}
	return term
}

// Tokenize lowercases, splits at camelCase boundaries and non-alphanumerics,
// drops stop words, and stems.
func Tokenize(text string) []string {
	text = camelLowerRe.ReplaceAllString(text, "$1 $2")
	text = camelUpperRe.ReplaceAllString(text, "$1 $2")
	text = strings.ToLower(text)
	parts := nonAlnumRe.Split(text, -1)
	out := make([]string, 0, len(parts))
	for _, term := range parts {
		if term == "" || stopWords[term] {
			continue
		}
		out = append(out, Stem(term))
	}
	return out
}

// SchemaText collects schema descriptions and property names recursively.
func SchemaText(schema any, parts *[]string) {
	obj, ok := schema.(map[string]any)
	if !ok {
		return
	}
	if desc, ok := obj["description"].(string); ok {
		*parts = append(*parts, desc)
	}
	if props, ok := obj["properties"].(map[string]any); ok {
		for name, prop := range props {
			*parts = append(*parts, name)
			SchemaText(prop, parts)
		}
	}
	SchemaText(obj["items"], parts)
	for _, key := range []string{"anyOf", "oneOf", "allOf"} {
		if variants, ok := obj[key].([]any); ok {
			for _, variant := range variants {
				SchemaText(variant, parts)
			}
		}
	}
}

// Document is the searchable text of one tool: its name, the name with
// underscores as spaces, its description, its schema text, and its namespace.
func Document(name, description string, params json.RawMessage, extra ...string) string {
	parts := []string{name, strings.ReplaceAll(name, "_", " "), description}
	if len(params) > 0 {
		var schema any
		if json.Unmarshal(params, &schema) == nil {
			SchemaText(schema, &parts)
		}
	}
	parts = append(parts, extra...)
	kept := parts[:0]
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			kept = append(kept, part)
		}
	}
	return strings.Join(kept, " ")
}

// Match is one ranked document.
type Match struct {
	Index int
	Score float64
}

// Rank is Okapi BM25 with k1=1.2 and b=0.75; ties keep document order.
func Rank(query string, documents []string, limit int) []Match {
	queryTerms := []string{}
	seen := map[string]bool{}
	for _, term := range Tokenize(query) {
		if !seen[term] {
			seen[term] = true
			queryTerms = append(queryTerms, term)
		}
	}
	if len(queryTerms) == 0 || len(documents) == 0 || limit <= 0 {
		return nil
	}

	const k1, b = 1.2, 0.75
	counts := make([]map[string]int, len(documents))
	lengths := make([]int, len(documents))
	total := 0
	for i, doc := range documents {
		c := map[string]int{}
		for _, term := range Tokenize(doc) {
			c[term]++
		}
		counts[i] = c
		for _, n := range c {
			lengths[i] += n
		}
		total += lengths[i]
	}
	avgLength := float64(total) / float64(len(documents))
	if avgLength == 0 {
		avgLength = 1
	}

	idf := map[string]float64{}
	for _, term := range queryTerms {
		frequency := 0
		for _, c := range counts {
			if c[term] > 0 {
				frequency++
			}
		}
		idf[term] = math.Log(1 + (float64(len(documents))-float64(frequency)+0.5)/(float64(frequency)+0.5))
	}

	var matches []Match
	for i, c := range counts {
		score := 0.0
		for _, term := range queryTerms {
			count := c[term]
			if count == 0 {
				continue
			}
			norm := k1 * (1 - b + b*float64(lengths[i])/avgLength)
			score += idf[term] * (float64(count) * (k1 + 1) / (float64(count) + norm))
		}
		if score > 0 {
			matches = append(matches, Match{Index: i, Score: score})
		}
	}
	sort.SliceStable(matches, func(i, j int) bool { return matches[i].Score > matches[j].Score })
	if len(matches) > limit {
		matches = matches[:limit]
	}
	return matches
}
