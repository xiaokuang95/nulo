package settings

import (
	"fmt"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unicode/utf8"
)

// normalizeSettingsPath expands ~ and converts a file:// URL, as Pi's
// normalizePath does. A leading ~ or ~/ expands to the home directory.
func normalizeSettingsPath(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	if strings.HasPrefix(path, "file://") {
		lower := strings.ToLower(path)
		if strings.Contains(lower, "%2f") || strings.Contains(lower, "%5c") {
			return "", fmt.Errorf("invalid file URL: %s", path)
		}
		parsed, err := url.Parse(path)
		if err != nil {
			return "", err
		}
		if parsed.Path == "" {
			return "", fmt.Errorf("invalid file URL: %s", path)
		}
		host := parsed.Host
		if runtime.GOOS == "windows" {
			if host == "" || strings.EqualFold(host, "localhost") {
				if len(parsed.Path) < 3 || parsed.Path[0] != '/' || parsed.Path[2] != ':' {
					return "", fmt.Errorf("invalid file URL: %s", path)
				}
				path = parsed.Path[1:]
				host = ""
			} else {
				path = parsed.Path
				host = idnaHost(host)
			}
		} else {
			path = parsed.Path
		}
		if host != "" && host != "localhost" {
			path = "//" + host + path
		}
		if runtime.GOOS == "windows" {
			if len(path) >= 4 && path[0] == '/' && path[2] == '/' && ((path[1] >= 'A' && path[1] <= 'Z') || (path[1] >= 'a' && path[1] <= 'z')) {
				drive := strings.ToUpper(path[1:2])
				path = drive + ":\\" + path[3:]
			}
			if len(path) >= 2 && ((path[0] >= 'A' && path[0] <= 'Z') || (path[0] >= 'a' && path[0] <= 'z')) && path[1] == ':' {
				path = strings.ToUpper(path[:1]) + path[1:]
			}
		}
		return filepath.FromSlash(path), nil
	}
	if path == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return home, nil
	}
	if strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, path[2:]), nil
	}
	if runtime.GOOS == "windows" {
		// MSYS drive forms: /c/tools/bash.exe -> C:	oolsash.exe.
		if len(path) >= 2 && path[0] == '/' && ((path[1] >= 'A' && path[1] <= 'Z') || (path[1] >= 'a' && path[1] <= 'z')) && (len(path) == 2 || path[2] == '/') {
			drive := strings.ToUpper(path[1:2])
			rest := ""
			if len(path) > 2 {
				rest = path[3:]
			}
			return drive + ":\\" + filepath.FromSlash(rest), nil
		}
		if len(path) >= 4 && path[0] == '/' && path[2] == '/' && ((path[1] >= 'A' && path[1] <= 'Z') || (path[1] >= 'a' && path[1] <= 'z')) {
			drive := strings.ToUpper(path[1:2])
			return drive + ":\\" + filepath.FromSlash(path[3:]), nil
		}
	}
	return path, nil
}

// decodePunycode implements RFC 3492's decoder for a single IDNA label.
func decodePunycode(encoded string) (string, bool) {
	const (
		base, damp, initialBias, initialN, skew, tmax, tmin = 36, 700, 72, 128, 38, 26, 1
	)
	pos := 1 + strings.LastIndex(encoded, "-")
	if pos == 1 {
		return "", false
	}
	if pos == len(encoded) {
		return encoded[:len(encoded)-1], true
	}
	output := make([]rune, 0, len(encoded))
	if pos != 0 {
		for _, r := range encoded[:pos-1] {
			output = append(output, r)
		}
	}
	i, n, bias := int32(0), int32(initialN), int32(initialBias)
	for pos < len(encoded) {
		oldI, w := i, int32(1)
		for k := int32(base); ; k += base {
			if pos == len(encoded) {
				return "", false
			}
			digit := int32(-1)
			switch c := encoded[pos]; {
			case c >= '0' && c <= '9':
				digit = int32(c - '0' + 26)
			case c >= 'A' && c <= 'Z':
				digit = int32(c - 'A')
			case c >= 'a' && c <= 'z':
				digit = int32(c - 'a')
			}
			if digit < 0 {
				return "", false
			}
			pos++
			product := int64(w) * int64(digit)
			if product > math.MaxInt32-int64(i) {
				return "", false
			}
			i += int32(product)
			t := k - bias
			if k <= bias {
				t = tmin
			} else if k >= bias+tmax {
				t = tmax
			}
			if digit < t {
				break
			}
			product = int64(w) * int64(base-t)
			if product > math.MaxInt32 {
				return "", false
			}
			w = int32(product)
		}
		if len(output) >= 1024 {
			return "", false
		}
		x := int32(len(output) + 1)
		delta := i - oldI
		if oldI == 0 {
			delta /= damp
		} else {
			delta /= 2
		}
		delta += delta / x
		k := int32(0)
		for delta > ((base-tmin)*tmax)/2 {
			delta /= base - tmin
			k += base
		}
		bias = k + (base-tmin+1)*delta/(delta+skew)
		n += i / x
		i %= x
		if n < 0 || n > utf8.MaxRune {
			return "", false
		}
		output = append(output, 0)
		copy(output[i+1:], output[i:])
		output[i] = n
		i++
	}
	return string(output), true
}

func idnaHost(host string) string {
	labels := strings.Split(strings.ToLower(host), ".")
	for index, label := range labels {
		if strings.HasPrefix(label, "xn--") {
			if decoded, ok := decodePunycode(label[4:]); ok {
				labels[index] = strings.ToLower(decoded)
			}
		}
	}
	return strings.Join(labels, ".")
}

// CONFIG_DIR_NAME is the per-project configuration directory name.
const CONFIG_DIR_NAME = ".nulo"

// DefaultAgentDir is ~/.nulo/agent, or $NULO_CODING_AGENT_DIR.
func DefaultAgentDir() string {
	if value := os.Getenv("NULO_CODING_AGENT_DIR"); value != "" {
		expanded, err := normalizeSettingsPath(value)
		if err == nil && expanded != "" {
			return expanded
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".nulo", "agent")
	}
	return filepath.Join(home, ".nulo", "agent")
}

// ResolveDefaultAgentDir is DefaultAgentDir with its home-directory error.
func ResolveDefaultAgentDir() (string, error) {
	if value := os.Getenv("NULO_CODING_AGENT_DIR"); value != "" {
		return normalizeSettingsPath(value)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".nulo", "agent"), nil
}
