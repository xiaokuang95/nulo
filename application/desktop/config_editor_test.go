package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestConfigEditorRestoresAfterFailedReload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "models.json")
	if err := os.WriteFile(path, []byte("old\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	a := baseApp()
	a.openConfigEditor("Models", path, "help", func() error { return errors.New("invalid config") })
	a.configText = "new\n"
	a.saveConfigEditor()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "old\n" {
		t.Fatalf("file = %q, want old", data)
	}
	if !a.configOpen {
		t.Fatal("editor closed after a failed reload")
	}
}

func TestConfigEditorSavesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.json")
	a := baseApp()
	a.openConfigEditor("MCP", path, "help", func() error { return nil })
	a.configText = "{\"mcpServers\":{}}\n"
	a.saveConfigEditor()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "{\"mcpServers\":{}}\n" {
		t.Fatalf("file = %q", data)
	}
	if a.configOpen {
		t.Fatal("editor did not close")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o", info.Mode().Perm())
	}
}

func TestConfigEditorRejectsInvalidMCPJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.json")
	a := baseApp()
	a.openConfigEditor("MCP", path, "help", nil)
	a.configText = "{not json"
	a.saveConfigEditor()
	if !a.configOpen {
		t.Fatal("editor closed after invalid JSON")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("invalid file was written: %v", err)
	}
}
