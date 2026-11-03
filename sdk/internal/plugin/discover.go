package plugin

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// SourceKind is how a plugin is hosted.
type SourceKind string

const (
	SourceLibrary  SourceKind = "library"
	SourceProtocol SourceKind = "protocol"
)

// Source is one plugin discovered from a path, manifest, or directory.
// Path is the shared library for library plugins and the plugin.json manifest
// for protocol plugins.
type Source struct {
	Kind     SourceKind
	Path     string
	Manifest Manifest
}

// Discover turns a file, manifest, or directory into plugin sources. Unlike
// DiscoverPath, it understands protocol plugins declared by plugin.json.
// Directories are scanned one level deep in lexical order, with the same
// recursion and hidden-directory rules as DiscoverPath.
func Discover(path string) ([]Source, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		if strings.EqualFold(filepath.Base(path), "plugin.json") {
			return sourcesFromManifest(path)
		}
		if isSharedLibrary(path) {
			return []Source{{Kind: SourceLibrary, Path: path}}, nil
		}
		return nil, fmt.Errorf("%s is not a shared library or plugin.json", path)
	}
	return sourcesFromDir(path)
}

func sourcesFromDir(dir string) ([]Source, error) {
	if manifest := filepath.Join(dir, "plugin.json"); pathIsFile(manifest) {
		return sourcesFromManifest(manifest)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	var out []Source
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") || name == "node_modules" {
			continue
		}
		full := filepath.Join(dir, name)
		if entry.IsDir() {
			if manifest := filepath.Join(full, "plugin.json"); pathIsFile(manifest) {
				sources, err := sourcesFromManifest(manifest)
				if err != nil {
					return nil, err
				}
				out = append(out, sources...)
				continue
			}
			sources, err := sourcesFromDir(full)
			if err != nil {
				return nil, err
			}
			out = append(out, sources...)
			continue
		}
		if strings.EqualFold(name, "plugin.json") {
			sources, err := sourcesFromManifest(full)
			if err != nil {
				return nil, err
			}
			out = append(out, sources...)
			continue
		}
		if isSharedLibrary(name) {
			out = append(out, Source{Kind: SourceLibrary, Path: full})
		}
	}
	return out, nil
}

func sourcesFromManifest(path string) ([]Source, error) {
	manifest, err := readManifest(path)
	if err != nil {
		return nil, err
	}
	if manifest.Protocol != "" || manifest.Command != "" {
		if manifest.Protocol != "" && !strings.EqualFold(manifest.Protocol, "stdio") {
			return nil, fmt.Errorf("%s: unsupported plugin protocol %q", path, manifest.Protocol)
		}
		if strings.TrimSpace(manifest.Command) == "" {
			return nil, fmt.Errorf("%s: protocol plugin has no command", path)
		}
		return []Source{{Kind: SourceProtocol, Path: path, Manifest: manifest}}, nil
	}
	libs, err := discoverManifest(path)
	if err != nil {
		return nil, err
	}
	out := make([]Source, 0, len(libs))
	for _, lib := range libs {
		out = append(out, Source{Kind: SourceLibrary, Path: lib, Manifest: manifest})
	}
	return out, nil
}

func readManifest(path string) (Manifest, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, err
	}
	var manifest Manifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("%s: %w", path, err)
	}
	if manifest.APIVersion != 0 && manifest.APIVersion != APIVersion {
		return Manifest{}, fmt.Errorf("%s: plugin API version %d is not supported", path, manifest.APIVersion)
	}
	return manifest, nil
}
