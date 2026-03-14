package shim

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/evmac/go-bake/internal/config"
)

func TestWriteShims(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.File{
		RootDir: dir,
		Targets: []*config.Target{{Name: "build"}, {Name: "test"}},
		Suites:  []*config.Suite{{Name: "ci"}},
	}
	if err := WriteShims(dir, cfg, "/usr/bin/bake"); err != nil {
		t.Fatalf("WriteShims: %v", err)
	}
	binDir := filepath.Join(dir, ".bake", "bin")
	for _, name := range []string{"build", "test", "ci"} {
		p := filepath.Join(binDir, name)
		data, err := os.ReadFile(p)
		if err != nil {
			t.Errorf("read shim %s: %v", name, err)
			continue
		}
		if len(data) < 10 || string(data)[:10] != "#!/bin/sh\n" {
			t.Errorf("shim %s wrong content: %s", name, data)
		}
	}
}

func TestWriteShimsRemovesStale(t *testing.T) {
	dir := t.TempDir()
	binDir := filepath.Join(dir, ".bake", "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "old"), []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}
	cfg := &config.File{
		RootDir: dir,
		Targets: []*config.Target{{Name: "build"}},
	}
	if err := WriteShims(dir, cfg, "bake"); err != nil {
		t.Fatalf("WriteShims: %v", err)
	}
	if _, err := os.Stat(filepath.Join(binDir, "old")); err == nil {
		t.Error("stale shim old should have been removed")
	}
	if _, err := os.Stat(filepath.Join(binDir, "build")); err != nil {
		t.Errorf("build shim missing: %v", err)
	}
}

func TestWriteShimsNilConfig(t *testing.T) {
	if err := WriteShims("/tmp/bogus", nil, "bake"); err != nil {
		t.Errorf("WriteShims(nil) should return nil, got %v", err)
	}
}

func TestWriteShimsDefaultBakeExe(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.File{
		RootDir: dir,
		Targets: []*config.Target{{Name: "build"}},
	}
	if err := WriteShims(dir, cfg, ""); err != nil {
		t.Fatalf("WriteShims with empty bakeExe: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, ".bake", "bin", "build"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"bake"`) {
		t.Errorf("expected shim to use 'bake', got: %s", data)
	}
}

func TestWriteShimsSkipsSubdirs(t *testing.T) {
	dir := t.TempDir()
	binDir := filepath.Join(dir, ".bake", "bin")
	os.MkdirAll(binDir, 0755)
	// Put a subdirectory in bin — should not be removed
	os.MkdirAll(filepath.Join(binDir, "subdir"), 0755)
	cfg := &config.File{
		RootDir: dir,
		Targets: []*config.Target{{Name: "build"}},
	}
	if err := WriteShims(dir, cfg, "bake"); err != nil {
		t.Fatalf("WriteShims: %v", err)
	}
	if _, err := os.Stat(filepath.Join(binDir, "subdir")); err != nil {
		t.Errorf("subdir should not have been removed: %v", err)
	}
}

func TestWriteShimsNoTargetsNoSuites(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.File{RootDir: dir}
	if err := WriteShims(dir, cfg, "bake"); err != nil {
		t.Fatalf("WriteShims with empty config: %v", err)
	}
	binDir := filepath.Join(dir, ".bake", "bin")
	entries, err := os.ReadDir(binDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 shims, got %d", len(entries))
	}
}

func TestWriteShimsOverwrites(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.File{
		RootDir: dir,
		Targets: []*config.Target{{Name: "build"}},
	}
	// First write
	WriteShims(dir, cfg, "bake")
	// Second write with different exe
	WriteShims(dir, cfg, "/new/bake")
	data, _ := os.ReadFile(filepath.Join(dir, ".bake", "bin", "build"))
	if !strings.Contains(string(data), "/new/bake") {
		t.Errorf("shim should contain new exe path: %s", data)
	}
}
