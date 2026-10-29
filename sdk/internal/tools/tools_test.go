package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadWriteEdit(t *testing.T) {
	dir := t.TempDir()
	ts := Default(dir)
	byName := map[string]int{}
	for i, x := range ts {
		byName[x.Name] = i
	}
	ctx := context.Background()
	_, err := ts[byName["write"]].Execute(ctx, "1", map[string]any{"path": "a.txt", "content": "hello world"})
	if err != nil {
		t.Fatal(err)
	}
	res, err := ts[byName["read"]].Execute(ctx, "2", map[string]any{"path": "a.txt"})
	if err != nil || res.IsError {
		t.Fatalf("%v %+v", err, res)
	}
	if res.Content[0].Text != "hello world" {
		t.Fatalf("%q", res.Content[0].Text)
	}
	res, err = ts[byName["edit"]].Execute(ctx, "3", map[string]any{
		"path":  "a.txt",
		"edits": []any{map[string]any{"oldText": "world", "newText": "nulo"}},
	})
	if err != nil || res.IsError {
		t.Fatalf("edit %v %+v", err, res)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "a.txt"))
	if string(got) != "hello nulo" {
		t.Fatalf("%q", got)
	}
}

func TestEditNotUnique(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("x x"), 0o644)
	q := newMutationQueue()
	ed := Edit(dir, q)
	res, _ := ed.Execute(context.Background(), "1", map[string]any{
		"path":  "a.txt",
		"edits": []any{map[string]any{"oldText": "x", "newText": "y"}},
	})
	if !res.IsError {
		t.Fatal("expected unique error")
	}
}

func TestBash(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "f.txt"), []byte("hi"), 0o644)
	b := Bash(dir)
	res, err := b.Execute(context.Background(), "1", map[string]any{"command": "cat f.txt"})
	if err != nil || res.IsError {
		t.Fatalf("%v %+v", err, res)
	}
	if !strings.Contains(res.Content[0].Text, "hi") {
		t.Fatalf("%q", res.Content[0].Text)
	}
}
