package sdk_test

import (
	"go/build"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const modulePath = "github.com/xiaokuang95/nulo"

func TestImportDirection(t *testing.T) {
	t.Parallel()
	root := moduleRoot(t)
	ctx := build.Default
	ctx.UseAllFiles = true

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		base := d.Name()
		if base == "testdata" || base == "vendor" || strings.HasPrefix(base, ".") {
			return fs.SkipDir
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		pkg, err := ctx.ImportDir(path, 0)
		if err != nil {
			if _, noGo := err.(*build.NoGoError); noGo {
				return nil
			}
			return err
		}
		importPath := modulePath + "/" + filepath.ToSlash(rel)
		checkPackageImports(t, importPath, pkg.Imports, pkg.TestImports, pkg.XTestImports)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func checkPackageImports(t *testing.T, importPath string, groups ...[]string) {
	t.Helper()
	rel := strings.TrimPrefix(importPath, modulePath+"/")
	isSDK := rel == "sdk"
	isInternal := strings.HasPrefix(rel, "sdk/internal/")
	internalName := ""
	if isInternal {
		internalName = strings.Split(strings.TrimPrefix(rel, "sdk/internal/"), "/")[0]
	}

	for _, group := range groups {
		for _, imp := range group {
			if !strings.HasPrefix(imp, modulePath) {
				if isForbiddenHostDep(imp) {
					t.Errorf("%s imports host UI/server %s", importPath, imp)
				}
				continue
			}
			if imp == modulePath+"/sdk" {
				if !isSDK && !strings.HasPrefix(rel, "examples/") {
					t.Errorf("%s must not import %s (only sdk and examples may)", importPath, imp)
				}
				continue
			}
			if strings.HasPrefix(imp, modulePath+"/sdk/internal/") {
				if !isSDK && !isInternal {
					t.Errorf("host package %s imports internal %s", importPath, imp)
				}
				if isInternal {
					from := internalName
					to := strings.Split(strings.TrimPrefix(imp, modulePath+"/sdk/internal/"), "/")[0]
					if !allowedInternalImport(from, to) {
						t.Errorf("%s may not import %s", importPath, imp)
					}
				}
				continue
			}
			if strings.HasPrefix(imp, modulePath+"/") && imp != modulePath+"/sdk" && !strings.HasPrefix(imp, modulePath+"/sdk/internal/") {
				if !strings.HasPrefix(rel, "examples/") {
					t.Errorf("%s imports non-sdk public path %s", importPath, imp)
				}
			}
		}
	}
}

func allowedInternalImport(from, to string) bool {
	if from == to {
		return true
	}
	if from == "ai" {
		return false
	}
	if from == "agent" {
		return to == "ai"
	}
	return true
}

func isForbiddenHostDep(imp string) bool {
	switch {
	case strings.Contains(imp, "bubbletea"),
		strings.Contains(imp, "tcell"),
		strings.Contains(imp, "tview"),
		strings.Contains(imp, "wails"),
		strings.Contains(imp, "fyne"):
		return true
	}
	return false
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for dir := wd; ; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		if dir == filepath.Dir(dir) {
			t.Fatal("go.mod not found")
		}
	}
}
