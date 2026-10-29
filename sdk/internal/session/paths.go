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

func SessionDir(agentDir, cwd string) string {
	return filepath.Join(agentDir, "sessions", EncodeCWD(cwd))
}
