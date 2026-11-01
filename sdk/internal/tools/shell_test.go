package tools

import (
	"strings"
	"testing"
)

func TestTailTruncate(t *testing.T) {
	cases := []struct {
		name        string
		content     string
		maxLines    int
		maxBytes    int
		wantContent string
		wantTrunc   bool
		wantBy      string
		wantLines   int
		wantPartial bool
	}{
		{
			name:        "short output is untouched",
			content:     "a\nb\nc\n",
			maxLines:    5,
			maxBytes:    100,
			wantContent: "a\nb\nc\n",
			wantLines:   3,
		},
		{
			name:        "keeps the last lines",
			content:     "1\n2\n3\n4\n5",
			maxLines:    2,
			maxBytes:    100,
			wantContent: "4\n5",
			wantTrunc:   true,
			wantBy:      "lines",
			wantLines:   2,
		},
		{
			name:        "byte limit keeps the tail",
			content:     "aaaa\nbbbb\ncccc",
			maxLines:    10,
			maxBytes:    9,
			wantContent: "bbbb\ncccc",
			wantTrunc:   true,
			wantBy:      "bytes",
			wantLines:   2,
		},
		{
			name:        "a single huge line yields its own terminus",
			content:     "x\n" + strings.Repeat("a", 100),
			maxLines:    10,
			maxBytes:    10,
			wantContent: strings.Repeat("a", 10),
			wantTrunc:   true,
			wantBy:      "bytes",
			wantLines:   1,
			wantPartial: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tailTruncate(tc.content, tc.maxLines, tc.maxBytes)
			if got.content != tc.wantContent {
				t.Fatalf("content %q, want %q", got.content, tc.wantContent)
			}
			if got.truncated != tc.wantTrunc {
				t.Fatalf("truncated %v, want %v", got.truncated, tc.wantTrunc)
			}
			if got.truncatedBy != tc.wantBy {
				t.Fatalf("truncatedBy %q, want %q", got.truncatedBy, tc.wantBy)
			}
			if got.outputLines != tc.wantLines {
				t.Fatalf("outputLines %d, want %d", got.outputLines, tc.wantLines)
			}
			if got.lastLinePartial != tc.wantPartial {
				t.Fatalf("lastLinePartial %v, want %v", got.lastLinePartial, tc.wantPartial)
			}
		})
	}
}

func TestTailTruncateCountsWholeOutput(t *testing.T) {
	got := tailTruncate("1\n2\n3\n4\n5", 2, 100)
	if got.totalLines != 5 {
		t.Fatalf("totalLines %d, want 5", got.totalLines)
	}
	if note := truncationNote(got, "/tmp/full.log"); note != "\n\n[Showing lines 4-5 of 5. Full output: /tmp/full.log]" {
		t.Fatalf("note %q", note)
	}

	// A partial last line reports the whole line's size, not the kept fragment's.
	got = tailTruncate("x\n"+strings.Repeat("a", 100), 10, 10)
	if got.lastLineBytes != 100 {
		t.Fatalf("lastLineBytes %d, want 100", got.lastLineBytes)
	}
	if note := truncationNote(got, "/tmp/full.log"); !strings.Contains(note, "of line 2 (line is 100B)") {
		t.Fatalf("note %q", note)
	}
}

func TestFormatSize(t *testing.T) {
	for _, tc := range []struct {
		in   int
		want string
	}{{0, "0B"}, {1023, "1023B"}, {1024, "1.0KB"}, {50 * 1024, "50.0KB"}, {1024 * 1024, "1.0MB"}} {
		if got := formatSize(tc.in); got != tc.want {
			t.Fatalf("formatSize(%d) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestResolveShellTimeout(t *testing.T) {
	if got, err := resolveShellTimeout(nil); err != nil || got.duration != 0 {
		t.Fatalf("absent timeout: %v %v", got, err)
	}
	if got, err := resolveShellTimeout(1.5); err != nil || got.raw != "1.5" {
		t.Fatalf("1.5s: %v %v", got, err)
	}
	for _, bad := range []any{0.0, -1.0} {
		if _, err := resolveShellTimeout(bad); err == nil {
			t.Fatalf("timeout %v should be rejected", bad)
		}
	}
	if _, err := resolveShellTimeout(float64(maxShellTimeoutMS)); err == nil {
		t.Fatal("a timeout beyond the ceiling should be rejected")
	}
}
