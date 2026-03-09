package watch

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Poll watches paths (files or dirs) by polling mtime every interval. When any path's mtime
// changes, it calls onChange (debounced by debounce). Run until ctx is done.
func Poll(ctx context.Context, rootDir string, paths []string, interval, debounce time.Duration, onChange func()) {
	if interval <= 0 {
		interval = 300 * time.Millisecond
	}
	if debounce <= 0 {
		debounce = 200 * time.Millisecond
	}
	mtimes := make(map[string]int64)
	var mu sync.Mutex
	var debounceTimer *time.Timer
	var debounceMu sync.Mutex
	trigger := func() {
		debounceMu.Lock()
		if debounceTimer != nil {
			debounceTimer.Stop()
		}
		debounceTimer = time.AfterFunc(debounce, func() {
			onChange()
			debounceMu.Lock()
			debounceTimer = nil
			debounceMu.Unlock()
		})
		debounceMu.Unlock()
	}
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			changed := false
			for _, p := range paths {
				full := filepath.Join(rootDir, p)
				info, err := os.Stat(full)
				if err != nil {
					continue
				}
				m := info.ModTime().UnixNano()
				mu.Lock()
				old, ok := mtimes[p]
				mtimes[p] = m
				mu.Unlock()
				if ok && old != m {
					changed = true
				}
			}
			if changed {
				trigger()
			}
		}
	}
}
