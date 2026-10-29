package tools

import (
	"path/filepath"
	"sync"
)

type mutationQueue struct {
	mu    sync.Mutex
	files map[string]chan struct{}
}

func newMutationQueue() *mutationQueue {
	return &mutationQueue{files: map[string]chan struct{}{}}
}

func (q *mutationQueue) do(path string, fn func() error) error {
	key := filepath.Clean(path)
	q.mu.Lock()
	ch, ok := q.files[key]
	if !ok {
		ch = make(chan struct{}, 1)
		ch <- struct{}{}
		q.files[key] = ch
	}
	q.mu.Unlock()
	<-ch
	defer func() { ch <- struct{}{} }()
	return fn()
}
