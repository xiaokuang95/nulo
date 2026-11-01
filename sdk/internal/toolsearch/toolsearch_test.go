package toolsearch

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestTokenize(t *testing.T) {
	got := Tokenize("SearchUsers  the issues, and PDFFiles!")
	want := []string{"search", "user", "issue", "pdf", "file"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%v, want %v", got, want)
	}
}

func TestStem(t *testing.T) {
	cases := map[string]string{
		"issues":   "issue",
		"stories":  "story",
		"searches": "search",
		"boxes":    "box",
		"glass":    "glass", // must not lose the double s
		"pdf":      "pdf",
	}
	for in, want := range cases {
		if got := Stem(in); got != want {
			t.Fatalf("Stem(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRank(t *testing.T) {
	docs := []string{
		"mcp__demo__ping ping the demo server",
		"mcp__demo__search search the demo index",
		"unrelated thing entirely",
	}
	matches := Rank("ping server", docs, 8)
	if len(matches) == 0 || matches[0].Index != 0 {
		t.Fatalf("%+v", matches)
	}
	if got := Rank("ping", docs, 1); len(got) != 1 {
		t.Fatalf("limit ignored: %+v", got)
	}
	if got := Rank("the and of", docs, 8); len(got) != 0 {
		t.Fatalf("only stop words should match nothing: %+v", got)
	}
	if got := Rank("ping", docs, 0); got != nil {
		t.Fatalf("limit 0 -> %+v", got)
	}
}

func TestDocumentIncludesSchema(t *testing.T) {
	params := json.RawMessage(`{"type":"object","properties":{"assignee":{"type":"string","description":"Who owns it"}}}`)
	doc := Document("mcp__demo__create_issue", "Create an issue", params, "mcp__demo", "Demo", "Be nice")
	for _, want := range []string{"create_issue", "create issue", "Create an issue", "assignee", "Who owns it", "mcp__demo", "Demo", "Be nice"} {
		if !strings.Contains(doc, want) {
			t.Fatalf("document missing %q: %q", want, doc)
		}
	}
}

func TestDocumentToleratesBadSchema(t *testing.T) {
	doc := Document("a", "b", json.RawMessage(`{`))
	if doc != "a a b" {
		t.Fatalf("%q", doc)
	}
}
