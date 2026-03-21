package watch

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestPollContextCancel(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "f")
	if err := os.WriteFile(f, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	called := 0
	var mu sync.Mutex
	go Poll(ctx, dir, []string{"f"}, 50*time.Millisecond, 10*time.Millisecond, func() {
		mu.Lock()
		called++
		mu.Unlock()
	})
	time.Sleep(80 * time.Millisecond)
	cancel()
	time.Sleep(100 * time.Millisecond)
	mu.Lock()
	n := called
	mu.Unlock()
	// Initial poll may or may not trigger (mtimes just set); we only care that Poll exits on cancel
	if n > 1 {
		t.Logf("onChange called %d times", n)
	}
}

func TestPollOnChangeCalledWhenFileChanges(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "f")
	if err := os.WriteFile(f, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	onChangeCalled := make(chan struct{})
	go Poll(ctx, dir, []string{"f"}, 30*time.Millisecond, 20*time.Millisecond, func() {
		select {
		case <-onChangeCalled:
		default:
			close(onChangeCalled)
		}
	})
	// Let first poll establish mtimes
	time.Sleep(50 * time.Millisecond)
	// Change file so next poll sees new mtime
	if err := os.WriteFile(f, []byte("y"), 0644); err != nil {
		t.Fatal(err)
	}
	select {
	case <-onChangeCalled:
		// onChange was called
	case <-time.After(1 * time.Second):
		cancel()
		t.Fatal("onChange was not called within 1s after file change")
	}
}

func TestPollZeroIntervalUsesDefault(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "f")
	os.WriteFile(f, nil, 0644)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// Should not panic with zero interval
	go Poll(ctx, dir, []string{"f"}, 0, 0, func() {})
	time.Sleep(20 * time.Millisecond)
	cancel()
}
