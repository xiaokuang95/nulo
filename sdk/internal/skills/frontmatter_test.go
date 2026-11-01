package skills

import (
	"testing"
)

func TestExtractFrontmatter(t *testing.T) {
	cases := []struct {
		name     string
		content  string
		wantYAML string
		wantBody string
		ok       bool
	}{
		{
			name:     "plain",
			content:  "---\nname: x\n---\nBody\n",
			wantYAML: "name: x",
			wantBody: "Body",
			ok:       true,
		},
		{
			name:     "crlf is normalised",
			content:  "---\r\nname: x\r\n---\r\nBody\r\n",
			wantYAML: "name: x",
			wantBody: "Body",
			ok:       true,
		},
		{
			name:     "byte-order mark",
			content:  "\ufeff---\nname: x\n---\nBody\n",
			wantYAML: "name: x",
			wantBody: "Body",
			ok:       true,
		},
		{
			name:     "body is trimmed",
			content:  "---\nname: x\n---\n\n\n  Body  \n\n",
			wantYAML: "name: x",
			wantBody: "Body",
			ok:       true,
		},
		{
			name:     "no frontmatter",
			content:  "just a body\n",
			wantBody: "just a body\n",
			ok:       false,
		},
		{
			name:     "unterminated",
			content:  "---\nname: x\nstill going\n",
			wantBody: "---\nname: x\nstill going\n",
			ok:       false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			yamlString, body, ok := extractFrontmatter(tc.content)
			if ok != tc.ok {
				t.Fatalf("ok=%v want %v", ok, tc.ok)
			}
			if ok && yamlString != tc.wantYAML {
				t.Fatalf("yaml %q want %q", yamlString, tc.wantYAML)
			}
			if body != tc.wantBody {
				t.Fatalf("body %q want %q", body, tc.wantBody)
			}
		})
	}
}

func TestParseFrontmatterScalars(t *testing.T) {
	content := `---
name: "quoted"
description: 'it''s single quoted'
disable-model-invocation: true
enabled: false
count: 42
tag: v1.2.3
empty:
# a comment
commented: value # trailing comment
---
Body
`
	fm, body, err := ParseFrontmatter(content)
	if err != nil {
		t.Fatal(err)
	}
	if body != "Body" {
		t.Fatalf("body %q", body)
	}
	want := map[string]any{
		"name":                     "quoted",
		"description":              "it's single quoted",
		"disable-model-invocation": true,
		"enabled":                  false,
		"count":                    int64(42),
		"tag":                      "v1.2.3",
		"empty":                    nil,
		"commented":                "value",
	}
	for k, v := range want {
		if fm[k] != v {
			t.Fatalf("%s = %#v, want %#v", k, fm[k], v)
		}
	}
}

// YAML 1.2 core types "yes" as a string, which is why it does not disable a
// skill — the same trap Pi has.
func TestParseFrontmatterYesIsString(t *testing.T) {
	fm, _, err := ParseFrontmatter("---\ndisable-model-invocation: yes\n---\n")
	if err != nil {
		t.Fatal(err)
	}
	if fm["disable-model-invocation"] == true {
		t.Fatal("yes must not become the boolean true")
	}
	if _, ok := fm["disable-model-invocation"].(string); !ok {
		t.Fatalf("got %#v", fm["disable-model-invocation"])
	}
}

func TestParseFrontmatterBlockScalars(t *testing.T) {
	literal := "---\ndescription: |\n  line one\n  line two\n---\n"
	fm, _, err := ParseFrontmatter(literal)
	if err != nil {
		t.Fatal(err)
	}
	if fm["description"] != "line one\nline two" {
		t.Fatalf("literal %q", fm["description"])
	}

	folded := "---\ndescription: >\n  line one\n  line two\n---\n"
	if fm, _, err = ParseFrontmatter(folded); err != nil {
		t.Fatal(err)
	}
	if fm["description"] != "line one line two" {
		t.Fatalf("folded %q", fm["description"])
	}
}

// A numeric name is not a string, so Pi falls back to the directory name.
func TestParseFrontmatterNonStringName(t *testing.T) {
	fm, _, err := ParseFrontmatter("---\nname: 42\n---\n")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := fm["name"].(string); ok {
		t.Fatalf("42 should not be a string: %#v", fm["name"])
	}
}

func TestParseFrontmatterNestedIsSkipped(t *testing.T) {
	content := "---\nname: x\nmetadata:\n  owner: someone\n  tags:\n    - a\ndescription: after the block\n---\n"
	fm, _, err := ParseFrontmatter(content)
	if err != nil {
		t.Fatal(err)
	}
	if fm["name"] != "x" || fm["description"] != "after the block" {
		t.Fatalf("%#v", fm)
	}
	if _, ok := fm["owner"]; ok {
		t.Fatal("nested keys must not surface at the top level")
	}
}

func TestParseFrontmatterErrors(t *testing.T) {
	for _, content := range []string{
		"---\nname: \"unterminated\n---\n",
		"---\nname: 'unterminated\n---\n",
		"---\nnot a mapping\n---\n",
	} {
		if _, _, err := ParseFrontmatter(content); err == nil {
			t.Fatalf("%q should be an error", content)
		}
	}
}

func TestParseFrontmatterNoBlock(t *testing.T) {
	fm, body, err := ParseFrontmatter("no frontmatter\n")
	if err != nil || len(fm) != 0 || body != "no frontmatter\n" {
		t.Fatalf("%#v %q %v", fm, body, err)
	}
}

func TestStripFrontmatter(t *testing.T) {
	if got := StripFrontmatter("---\nname: x\n---\n\nBody\n"); got != "Body" {
		t.Fatalf("%q", got)
	}
	if got := StripFrontmatter("plain\n"); got != "plain\n" {
		t.Fatalf("%q", got)
	}
}
