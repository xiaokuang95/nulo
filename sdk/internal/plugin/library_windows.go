//go:build windows

package plugin

import (
	"fmt"
	"sync"
	"syscall"
)

type windowsLibrary struct {
	dll   *syscall.DLL
	mu    sync.Mutex
	procs map[string]*syscall.Proc
}

func openLibrary(path string) (nativeLibrary, error) {
	dll, err := syscall.LoadDLL(path)
	if err != nil {
		return nil, fmt.Errorf("load %s: %w", path, err)
	}
	return &windowsLibrary{dll: dll, procs: map[string]*syscall.Proc{}}, nil
}

func (l *windowsLibrary) proc(symbol string) (*syscall.Proc, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if p := l.procs[symbol]; p != nil {
		return p, nil
	}
	p, err := l.dll.FindProc(symbol)
	if err != nil {
		return nil, fmt.Errorf("symbol %s: %w", symbol, err)
	}
	l.procs[symbol] = p
	return p, nil
}

func (l *windowsLibrary) call(symbol string, args ...uintptr) (uintptr, error) {
	p, err := l.proc(symbol)
	if err != nil {
		return 0, err
	}
	r, _, callErr := p.Call(args...)
	if r == 0 && callErr != nil && callErr != syscall.Errno(0) {
		return 0, fmt.Errorf("%s: %w", symbol, callErr)
	}
	return r, nil
}

func (l *windowsLibrary) close() error {
	if l.dll == nil {
		return nil
	}
	err := l.dll.Release()
	l.dll = nil
	return err
}
