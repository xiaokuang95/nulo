package session

import (
	"os"
	"path/filepath"
	"strings"
)

// ResolveCWD names the directory a session records in its header and encodes
// in its storage path: an absolute cleaned path, with a leading ~ expanded.
func ResolveCWD(cwd string) string {
	if cwd == "" {
		cwd = "."
	}
	if cwd == "~" || strings.HasPrefix(cwd, "~/") || strings.HasPrefix(cwd, `~\`) {
		if home, err := os.UserHomeDir(); err == nil {
			cwd = filepath.Join(home, cwd[1:])
		}
	}
	if abs, err := filepath.Abs(cwd); err == nil {
		return abs
	}
	return filepath.Clean(cwd)
}

func EncodeCWD(cwd string) string {
	p := filepath.Clean(cwd)
	p = strings.TrimPrefix(p, "/")
	p = strings.TrimPrefix(p, `\`)
	replacer := strings.NewReplacer("/", "-", `\`, "-", ":", "-")
	return "--" + replacer.Replace(p) + "--"
}

// SessionDirIn is the per-cwd session directory inside an explicit sessions root.
func SessionDirIn(root, cwd string) string {
	return filepath.Join(root, EncodeCWD(ResolveCWD(cwd)))
}

// SessionDir is the per-cwd session directory inside an agent directory's own
// sessions root, <agentDir>/sessions.
func SessionDir(agentDir, cwd string) string {
	return SessionDirIn(filepath.Join(agentDir, "sessions"), cwd)
}
