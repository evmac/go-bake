package cache

import (
	"os"
	"path/filepath"
	"testing"
)

func TestKey(t *testing.T) {
	k1 := Key("build", []byte("hash1"), []byte("sig1"))
	k2 := Key("build", []byte("hash1"), []byte("sig1"))
	if k1 != k2 {
		t.Errorf("same inputs should give same key: %q vs %q", k1, k2)
	}
	k3 := Key("build", []byte("hash2"), []byte("sig1"))
	if k1 == k3 {
		t.Error("different input hash should give different key")
	}
}

func TestHashFiles(t *testing.T) {
	dir := t.TempDir()
	f1 := filepath.Join(dir, "a")
	f2 := filepath.Join(dir, "b")
	os.WriteFile(f1, []byte("x"), 0644)
	os.WriteFile(f2, []byte("y"), 0644)
	h, err := HashFiles(dir, []string{"a", "b"})
	if err != nil {
		t.Fatal(err)
	}
	if len(h) != 32 {
		t.Errorf("expected 32-byte hash, got %d", len(h))
	}
	// Same content order should give same hash
	h2, _ := HashFiles(dir, []string{"a", "b"})
	if string(h) != string(h2) {
		t.Error("HashFiles not deterministic")
	}
}

func TestHashFilesMap(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "f"), []byte("content"), 0644)
	combined, perFile, err := HashFilesMap(dir, []string{"f"})
	if err != nil {
		t.Fatal(err)
	}
	if len(combined) != 32 {
		t.Errorf("expected 32-byte combined hash, got %d", len(combined))
	}
	if perFile["f"] == "" {
		t.Error("perFile should have entry for f")
	}
}

func TestResolveGlobs(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "single"), nil, 0644)
	os.WriteFile(filepath.Join(dir, "a.go"), nil, 0644)
	os.WriteFile(filepath.Join(dir, "b.go"), nil, 0644)
	out, err := ResolveGlobs(dir, []string{"single", "*.go"})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 3 {
		t.Errorf("expected 3 paths, got %v", out)
	}
}

func TestCacheDir(t *testing.T) {
	dir := t.TempDir()
	d := CacheDir(dir)
	if d != filepath.Join(dir, ".bake", "cache") {
		t.Errorf("CacheDir: got %q", d)
	}
}

func TestLoadManifestNotFound(t *testing.T) {
	dir := t.TempDir()
	m, err := LoadManifest(dir, "nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if m != nil {
		t.Error("expected nil manifest for missing file")
	}
}

func TestSaveAndLoadManifest(t *testing.T) {
	dir := t.TempDir()
	m := &Manifest{
		TargetName:   "build",
		InputHash:    "abc",
		OutputPaths:  []string{"out"},
		OutputMTimes: map[string]int64{"out": 123},
	}
	key := Key("build", []byte("abc"), []byte("sig"))
	if err := SaveManifest(dir, key, m); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadManifest(dir, key)
	if err != nil {
		t.Fatal(err)
	}
	if loaded == nil || loaded.TargetName != "build" || loaded.InputHash != "abc" {
		t.Errorf("loaded: %+v", loaded)
	}
	if len(loaded.OutputPaths) != 1 || loaded.OutputPaths[0] != "out" {
		t.Errorf("OutputPaths: %v", loaded.OutputPaths)
	}
}

func TestLoadManifestForTarget(t *testing.T) {
	dir := t.TempDir()
	key := Key("mytarget", []byte("h"), []byte("s"))
	SaveManifest(dir, key, &Manifest{TargetName: "mytarget", InputHash: "h"})
	found, err := LoadManifestForTarget(dir, "mytarget")
	if err != nil {
		t.Fatal(err)
	}
	if found == nil || found.TargetName != "mytarget" {
		t.Errorf("LoadManifestForTarget: %+v", found)
	}
}

func TestLoadManifestForTargetNoCacheDir(t *testing.T) {
	dir := t.TempDir()
	// No .bake/cache created - ReadDir returns IsNotExist, so (nil, nil)
	found, err := LoadManifestForTarget(dir, "any")
	if err != nil {
		t.Fatal(err)
	}
	if found != nil {
		t.Errorf("expected nil when cache dir missing, got %+v", found)
	}
}

func TestLoadManifestForTargetWrongTarget(t *testing.T) {
	dir := t.TempDir()
	key := Key("other", []byte("h"), []byte("s"))
	SaveManifest(dir, key, &Manifest{TargetName: "other", InputHash: "h"})
	found, err := LoadManifestForTarget(dir, "wanted")
	if err != nil {
		t.Fatal(err)
	}
	if found != nil {
		t.Errorf("expected nil when no matching target, got %+v", found)
	}
}

func TestLoadManifestForTargetSkipsInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	cacheDir := filepath.Join(dir, ".bake", "cache")
	os.MkdirAll(cacheDir, 0755)
	os.WriteFile(filepath.Join(cacheDir, "bad.json"), []byte("not json"), 0644)
	key := Key("t", []byte("h"), []byte("s"))
	SaveManifest(dir, key, &Manifest{TargetName: "t", InputHash: "h"})
	found, err := LoadManifestForTarget(dir, "t")
	if err != nil {
		t.Fatal(err)
	}
	if found == nil || found.TargetName != "t" {
		t.Errorf("should skip bad.json and find t: %+v", found)
	}
}

func TestLoadManifestForTargetSkipsDirs(t *testing.T) {
	dir := t.TempDir()
	key := Key("t", []byte("h"), []byte("s"))
	SaveManifest(dir, key, &Manifest{TargetName: "t", InputHash: "h"})
	// Add a directory named like a .json file so we hit e.IsDir() continue path
	cacheDir := filepath.Join(dir, ".bake", "cache")
	os.MkdirAll(filepath.Join(cacheDir, "ignore.json"), 0755)
	found, err := LoadManifestForTarget(dir, "t")
	if err != nil {
		t.Fatal(err)
	}
	if found == nil || found.TargetName != "t" {
		t.Errorf("should skip dir and find t: %+v", found)
	}
}

func TestOutputsExist(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a"), nil, 0644)
	ok, missing := OutputsExist(dir, []string{"a", "b"})
	if ok {
		t.Error("expected false when one output missing")
	}
	if len(missing) != 1 || missing[0] != "b" {
		t.Errorf("missing: %v", missing)
	}
	os.WriteFile(filepath.Join(dir, "b"), nil, 0644)
	ok, _ = OutputsExist(dir, []string{"a", "b"})
	if !ok {
		t.Error("expected true when all exist")
	}
}

func TestRecordOutputMTimes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f")
	os.WriteFile(path, nil, 0644)
	mtimes, err := RecordOutputMTimes(dir, []string{"f"})
	if err != nil {
		t.Fatal(err)
	}
	if mtimes["f"] == 0 {
		t.Error("expected non-zero mtime")
	}
}

func TestResolveGlobsDirectory(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "subdir")
	os.MkdirAll(subdir, 0755)
	os.WriteFile(filepath.Join(subdir, "a"), nil, 0644)
	os.WriteFile(filepath.Join(subdir, "b"), nil, 0644)
	out, err := ResolveGlobs(dir, []string{"subdir"})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 {
		t.Errorf("expected 2 files under subdir, got %v", out)
	}
}

func TestLoadManifestInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	key := Key("x", []byte("h"), []byte("s"))
	cacheDir := filepath.Join(dir, ".bake", "cache")
	os.MkdirAll(cacheDir, 0755)
	os.WriteFile(filepath.Join(cacheDir, key+".json"), []byte("not valid json"), 0644)
	_, err := LoadManifest(dir, key)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestHashFilesMissingFile(t *testing.T) {
	dir := t.TempDir()
	_, err := HashFiles(dir, []string{"nonexistent"})
	if err == nil {
		t.Error("expected error for missing file")
	}
}
