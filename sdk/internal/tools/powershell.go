package tools

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/xiaokuang95/nulo/sdk/internal/agent"
)

// powershellUTF8Prefix is prepended to every command so non-ASCII output survives the
// console code page, matching Pi's UTF8_OUTPUT_PREFIX.
const powershellUTF8Prefix = "try { [Console]::OutputEncoding=[System.Text.Encoding]::UTF8 } catch {}\n"

// powershellArgs is Pi's POWERSHELL_ARGS: no profile, no interaction, no policy prompt.
var powershellArgs = []string{"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command"}

// These messages are Pi's verbatim, because they reach the model as tool output.
var (
	errPowerShellNotWindows = errors.New("The powershell tool is only available on Windows.")
	errPowerShellNotFound   = errors.New("No PowerShell executable found. Install PowerShell or add powershell.exe/pwsh.exe to PATH.")
)

// PowerShell returns the Windows shell tool. It shares shellTool's behaviour with bash
// but keeps its own name, prompt and launch arguments, exactly as Pi's
// createPowerShellTool reuses createShellToolDefinition.
func PowerShell(cwd string) agent.AgentTool {
	return shellTool(cwd, shellConfig{
		name:          "powershell",
		label:         "powershell",
		shellName:     "PowerShell",
		tempPrefix:    "nulo-powershell",
		commandPrefix: powershellUTF8Prefix,
		resolve:       resolvePowerShell,
	})
}

// resolvePowerShell mirrors Pi's getPowerShellConfig: PowerShell 7 (pwsh) wherever it
// can be found, then Windows PowerShell, then the System32 default.
func resolvePowerShell() (shellResolveResult, error) {
	if runtime.GOOS != "windows" {
		return shellResolveResult{}, errPowerShellNotWindows
	}
	shell := findExecutableOnPath("pwsh.exe")
	if shell == "" {
		// An installation directory may be on any drive and need not be on PATH.
		if programFiles := os.Getenv("ProgramFiles"); programFiles != "" {
			if p := filepath.Join(programFiles, "PowerShell", "7", "pwsh.exe"); fileExists(p) {
				shell = p
			}
		}
	}
	if shell == "" {
		shell = findExecutableOnPath("powershell.exe")
	}
	if shell == "" {
		if systemRoot := os.Getenv("SystemRoot"); systemRoot != "" {
			if p := filepath.Join(systemRoot, "System32", "WindowsPowerShell", "v1.0", "powershell.exe"); fileExists(p) {
				shell = p
			}
		}
	}
	if shell == "" {
		return shellResolveResult{}, errPowerShellNotFound
	}
	return shellResolveResult{path: shell, args: append([]string{}, powershellArgs...)}, nil
}

// findExecutableOnPath looks a name up on PATH. Pi shells out to `where`; Go's
// LookPath performs the same search without spawning a process.
func findExecutableOnPath(name string) string {
	p, err := exec.LookPath(name)
	if err != nil || !fileExists(p) {
		return ""
	}
	return p
}

// fileExists is Pi's shellPathExists: a plain access check rather than a stat, because
// on Windows a stat rejects runnable Store aliases.
func fileExists(p string) bool {
	f, err := os.Open(p)
	if err != nil {
		return false
	}
	f.Close()
	return true
}
