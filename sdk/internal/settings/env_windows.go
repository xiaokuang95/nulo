//go:build windows

package settings

import (
	"os"
	"syscall"
)

// envVar reads one process environment variable without allocating for an
// unset or empty value on Windows, matching Pi's environment lookups.
type envVar struct {
	name string
	utf  *uint16
}

func newEnvVar(name string) envVar {
	return envVar{name: name, utf: syscall.StringToUTF16Ptr(name)}
}

func (v envVar) get() string {
	var buf [1]uint16
	if size, _ := syscall.GetEnvironmentVariable(v.utf, &buf[0], uint32(len(buf))); size == 0 {
		return ""
	}
	return os.Getenv(v.name)
}

func (v envVar) isOne() bool {
	var buf [2]uint16
	size, _ := syscall.GetEnvironmentVariable(v.utf, &buf[0], uint32(len(buf)))
	return size == 1 && buf[0] == '1'
}

var (
	clearOnShrinkEnv  = newEnvVar("PI_CLEAR_ON_SHRINK")
	hardwareCursorEnv = newEnvVar("PI_HARDWARE_CURSOR")
	visualEnv         = newEnvVar("VISUAL")
	editorEnv         = newEnvVar("EDITOR")
)
