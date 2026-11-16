package session

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unicode"
)

// ResolveCWD names the directory a session records in its header: an absolute
// cleaned path, with a leading ~ expanded.
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

// ProjectKey is nulo's per-project directory name: a readable slug derived
// from the directory name plus a short hash of the resolved CWD. It is not an
// encoded absolute path, so it stays short and avoids drive/separator
// collisions on every platform.
func ProjectKey(cwd string) string {
	resolved := ResolveCWD(cwd)
	return projectSlug(resolved) + "-" + projectHash(resolved)
}

func projectSlug(cwd string) string {
	base := filepath.Base(filepath.Clean(cwd))
	if base == "." || base == string(filepath.Separator) || base == "" {
		base = "root"
	}
	var b strings.Builder
	lastDash := false
	for _, r := range base {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '_' || r == '-' {
			b.WriteRune(unicode.ToLower(r))
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	slug := strings.Trim(b.String(), "-._")
	if slug == "" {
		slug = "root"
	}
	runes := []rune(slug)
	if len(runes) > 32 {
		slug = string(runes[:32])
	}
	return slug
}

func projectHash(cwd string) string {
	normalized := filepath.Clean(cwd)
	if runtime.GOOS == "windows" {
		normalized = strings.ToLower(normalized)
	}
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:6])
}

// SessionDirIn is the per-project session directory inside an explicit
// sessions root.
func SessionDirIn(root, cwd string) string {
	return filepath.Join(root, ProjectKey(cwd))
}

// SessionDir is the per-project session directory inside an agent directory's
// own sessions root, <agentDir>/sessions.
func SessionDir(agentDir, cwd string) string {
	return SessionDirIn(filepath.Join(agentDir, "sessions"), cwd)
}

// LegacySessionDirIn is the old Pi-style encoded-CWD directory. It exists
// only so old session files can still be listed and opened after the storage
// layout changed; new sessions are never written there.
func LegacySessionDirIn(root, cwd string) string {
	return filepath.Join(root, legacyEncodeCWD(ResolveCWD(cwd)))
}

func legacyEncodeCWD(cwd string) string {
	p := filepath.Clean(cwd)
	p = strings.TrimPrefix(p, "/")
	p = strings.TrimPrefix(p, `\`)
	replacer := strings.NewReplacer("/", "-", `\`, "-", ":", "-")
	return "--" + replacer.Replace(p) + "--"
}
