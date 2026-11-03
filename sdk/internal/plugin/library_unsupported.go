//go:build !windows && !darwin && !freebsd && !linux && !netbsd

package plugin

import "fmt"

func openLibrary(path string) (nativeLibrary, error) {
	return nil, fmt.Errorf("native plugins are not supported on this platform")
}
