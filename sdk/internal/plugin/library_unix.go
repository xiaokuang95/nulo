//go:build darwin || freebsd || linux || netbsd

package plugin

import (
	"fmt"
	"sync"

	"github.com/ebitengine/purego"
)

type unixLibrary struct {
	handle uintptr
	mu     sync.Mutex
	procs  map[string]uintptr
}

func openLibrary(path string) (nativeLibrary, error) {
	handle, err := purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return nil, fmt.Errorf("load %s: %w", path, err)
	}
	return &unixLibrary{handle: handle, procs: map[string]uintptr{}}, nil
}

func (l *unixLibrary) proc(symbol string) (uintptr, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if p := l.procs[symbol]; p != 0 {
		return p, nil
	}
	p, err := purego.Dlsym(l.handle, symbol)
	if err != nil {
		return 0, fmt.Errorf("symbol %s: %w", symbol, err)
	}
	l.procs[symbol] = p
	return p, nil
}

func (l *unixLibrary) call(symbol string, args ...uintptr) (uintptr, error) {
	p, err := l.proc(symbol)
	if err != nil {
		return 0, err
	}
	r, _, callErr := purego.SyscallN(p, args...)
	if r == 0 && callErr != 0 {
		return 0, fmt.Errorf("%s: errno %d", symbol, callErr)
	}
	return r, nil
}

func (l *unixLibrary) close() error {
	if l.handle == 0 {
		return nil
	}
	err := purego.Dlclose(l.handle)
	l.handle = 0
	return err
}
