package session

import (
	"path/filepath"
	"strings"
)

func EncodeCWD(cwd string) string {
	p := filepath.Clean(cwd)
	p = strings.TrimPrefix(p, "/")
	p = strings.TrimPrefix(p, `\`)
	replacer := strings.NewReplacer("/", "-", `\`, "-", ":", "-")
	return "--" + replacer.Replace(p) + "--"
}

// SessionDirIn is the per-cwd session directory inside an explicit sessions root.
func SessionDirIn(root, cwd string) string {
	return filepath.Join(root, EncodeCWD(cwd))
}

// SessionDir is the per-cwd session directory inside an agent directory's own
// sessions root, <agentDir>/sessions.
func SessionDir(agentDir, cwd string) string {
	return SessionDirIn(filepath.Join(agentDir, "sessions"), cwd)
}
