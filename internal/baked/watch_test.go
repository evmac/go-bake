package baked

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/evmac/go-bake/internal/config"
)

func TestWatcherReload(t *testing.T) {
	dir := t.TempDir()
	bakePath := filepath.Join(dir, "Bakefile")
	if err := os.WriteFile(bakePath, []byte("target build { steps { exec [\"true\"] } }\n"), 0644); err != nil {
		t.Fatal(err)
	}

	reloadCount := 0
	w, err := NewWatcher(func() (*config.File, error) {
		reloadCount++
		_, cfg, err := Load(dir)
		return cfg, err
	})
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	defer w.Close()

	if err := w.Add([]string{bakePath}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	go func() {
		_ = w.Run()
	}()

	// Trigger write
	if err := os.WriteFile(bakePath, []byte("target build { desc \"x\" steps { exec [\"true\"] } }\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Wait for debounce + reload
	time.Sleep(debounceDur + 100*time.Millisecond)

	if reloadCount < 1 {
		t.Errorf("expected at least 1 reload, got %d", reloadCount)
	}
	cfg := w.Config()
	if cfg == nil {
		t.Fatal("config nil after reload")
	}
	if len(cfg.Targets) != 1 || cfg.Targets[0].Desc != "x" {
		t.Errorf("expected target with desc x, got %+v", cfg.Targets)
	}
}

func TestForceReload(t *testing.T) {
	dir := t.TempDir()
	bakePath := filepath.Join(dir, "Bakefile")
	os.WriteFile(bakePath, []byte("target build { steps { exec [\"true\"] } }\n"), 0644)

	reloadCount := 0
	w, err := NewWatcher(func() (*config.File, error) {
		reloadCount++
		_, cfg, err := Load(dir)
		return cfg, err
	})
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	w.ForceReload()
	time.Sleep(debounceDur + 100*time.Millisecond)
	if reloadCount < 1 {
		t.Errorf("ForceReload: expected at least 1 reload, got %d", reloadCount)
	}
}
