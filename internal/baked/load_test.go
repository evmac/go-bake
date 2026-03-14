package baked

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	bf := []byte(`target build { steps { exec ["true"] } }
suite dev { build }
`)
	bakePath := filepath.Join(dir, "Bakefile")
	if err := os.WriteFile(bakePath, bf, 0644); err != nil {
		t.Fatal(err)
	}

	rootDir, cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if rootDir != dir {
		t.Errorf("rootDir = %q, want %q", rootDir, dir)
	}
	if cfg == nil {
		t.Fatal("cfg is nil")
	}
	if len(cfg.Targets) != 1 || cfg.Targets[0].Name != "build" {
		t.Errorf("expected one target build, got %d targets", len(cfg.Targets))
	}
}

func TestLoadNoBakefile(t *testing.T) {
	dir := t.TempDir()
	_, _, err := Load(dir)
	if err == nil {
		t.Fatal("expected error when no Bakefile")
	}
}
