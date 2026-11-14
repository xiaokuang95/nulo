package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"
	"github.com/xiaokuang95/nulo/sdk"
)

func (a *app) openConfigEditor(title, path, help string, after func() error) {
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		a.pendingToast = "Config: " + err.Error()
		return
	}
	text := string(data)
	if len(data) == 0 {
		text = "{\n}\n"
	}
	a.configTitle = title
	a.configPath = path
	a.configHelp = help
	a.configText = text
	a.configAfter = after
	a.configOpen = true
}

func openFolder(path string) error {
	if err := os.MkdirAll(path, 0o755); err != nil {
		return err
	}
	return mygo.Shell.OpenPath(path)
}

func (a *app) saveConfigEditor() {
	path := a.configPath
	if path == "" {
		a.configOpen = false
		return
	}
	if filepath.Base(path) == "mcp.json" {
		var value any
		if err := json.Unmarshal([]byte(a.configText), &value); err != nil {
			a.pendingToast = "MCP: invalid JSON: " + err.Error()
			return
		}
	}
	previous, readErr := os.ReadFile(path)
	existed := readErr == nil
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		a.pendingToast = "Config: " + readErr.Error()
		return
	}
	if err := writeConfigFile(path, []byte(a.configText)); err != nil {
		a.pendingToast = "Config: " + err.Error()
		return
	}
	if a.configAfter != nil {
		if err := a.configAfter(); err != nil {
			if existed {
				_ = writeConfigFile(path, previous)
			} else {
				_ = os.Remove(path)
			}
			a.pendingToast = "Config: " + err.Error()
			return
		}
	}
	a.configOpen = false
	a.pendingToast = "Saved " + filepath.Base(path)
}

func writeConfigFile(path string, data []byte) error {
	if path == "" {
		return errors.New("config path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".config-*.tmp")
	if err != nil {
		return err
	}
	name := file.Name()
	defer os.Remove(name)
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return err
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

func (h *desktopHost) validateConfigReload(a *app) error {
	if h == nil {
		return errors.New("SDK host is unavailable")
	}
	if a != nil {
		for _, s := range a.sessions {
			if s.Status == "running" || s.Status == "awaiting" {
				return errors.New("stop running sessions first")
			}
			if e := h.engine(s); e != nil && !s.Welcome && (s.Path == "" || !fileExists(s.Path)) {
				return errors.New("send the current session before reloading configuration")
			}
		}
	}
	return nil
}

func (h *desktopHost) reloadAfterConfig(a *app) error {
	if err := h.validateConfigReload(a); err != nil {
		return err
	}
	h.mu.Lock()
	services := make([]*sdk.AgentSessionServices, 0, len(h.services))
	for cwd, svcs := range h.services {
		services = append(services, svcs)
		delete(h.services, cwd)
	}
	h.mu.Unlock()
	for _, s := range a.sessions {
		h.dropEngine(s)
	}
	for _, svcs := range services {
		_ = svcs.Close()
	}
	if err := h.loadProviderCatalog(a); err != nil {
		return err
	}
	active := a.active()
	if active != nil {
		if _, err := h.ensureEngine(a, active); err != nil {
			active.Status = "error"
			active.Note = "reload failed · " + err.Error()
			return err
		}
		a.syncSessionMeta(active, h.engine(active).sess)
		a.refreshMCPInfo(active)
	}
	return nil
}

func (a *app) reloadConfig(action string) error {
	if a.host == nil {
		return fmt.Errorf("SDK host is unavailable")
	}
	if err := a.host.reloadAfterConfig(a); err != nil {
		return err
	}
	a.pendingToast = action
	return nil
}
