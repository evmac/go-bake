package baked

import (
	"path/filepath"
	"sync"
	"time"

	"github.com/evmac/go-bake/internal/config"
	"github.com/fsnotify/fsnotify"
)

const debounceDur = 150 * time.Millisecond

// Watcher watches the Bakefile (and optionally imported paths) and calls OnReload when they change.
type Watcher struct {
	paths    []string
	watcher  *fsnotify.Watcher
	debounce *time.Timer
	mu       sync.Mutex
	onReload func() (*config.File, error) // called with mu held; returns new config
	lastCfg  *config.File
}

// NewWatcher creates a watcher. OnReload is called after debounced file events; it should load config and return it.
func NewWatcher(onReload func() (*config.File, error)) (*Watcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	return &Watcher{
		watcher:  w,
		onReload: onReload,
	}, nil
}

// Add adds paths to watch (Bakefile and any imported files). Paths are normalized to absolute.
func (w *Watcher) Add(paths []string) error {
	for _, p := range paths {
		abs, err := filepath.Abs(p)
		if err != nil {
			return err
		}
		if err := w.watcher.Add(abs); err != nil {
			return err
		}
		w.paths = append(w.paths, abs)
	}
	return nil
}

// Run runs the watch loop. It returns when Close is called or when a fatal error occurs.
func (w *Watcher) Run() error {
	for {
		select {
		case event, ok := <-w.watcher.Events:
			if !ok {
				return nil
			}
			if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove) != 0 {
				w.debouncedReload()
			}
		case err, ok := <-w.watcher.Errors:
			if !ok {
				return nil
			}
			return err
		}
	}
}

func (w *Watcher) debouncedReload() {
	w.mu.Lock()
	if w.debounce != nil {
		w.debounce.Stop()
	}
	w.debounce = time.AfterFunc(debounceDur, func() {
		if w.onReload != nil {
			cfg, err := w.onReload()
			if err != nil {
				return // caller can log
			}
			w.mu.Lock()
			w.lastCfg = cfg
			w.mu.Unlock()
		}
	})
	w.mu.Unlock()
}

// Config returns the last successfully reloaded config (or nil).
func (w *Watcher) Config() *config.File {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.lastCfg
}

// SetConfig updates the last-known config (e.g. after initial load).
func (w *Watcher) SetConfig(cfg *config.File) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.lastCfg = cfg
}

// ForceReload triggers an immediate config reload (e.g. from SIGUSR1).
func (w *Watcher) ForceReload() {
	w.debouncedReload()
}

// Close closes the watcher.
func (w *Watcher) Close() error {
	w.mu.Lock()
	if w.debounce != nil {
		w.debounce.Stop()
	}
	w.mu.Unlock()
	return w.watcher.Close()
}
