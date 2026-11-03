//go:build !windows

package settings

import "os"

type envVar string

func newEnvVar(name string) envVar { return envVar(name) }

func (v envVar) get() string { return os.Getenv(string(v)) }

func (v envVar) isOne() bool { return v.get() == "1" }

var (
	clearOnShrinkEnv  = newEnvVar("PI_CLEAR_ON_SHRINK")
	hardwareCursorEnv = newEnvVar("PI_HARDWARE_CURSOR")
	visualEnv         = newEnvVar("VISUAL")
	editorEnv         = newEnvVar("EDITOR")
)
