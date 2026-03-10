package lifecycle

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStatePath(t *testing.T) {
	got := StatePath("/root")
	want := "/root/.bake/state.json"
	if got != want {
		t.Errorf("StatePath() = %q, want %q", got, want)
	}
}

func TestLoadSaveRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if s.Daemons == nil {
		t.Fatal("Load should return non-nil Daemons map")
	}
	s.Daemons["redis"] = DaemonEntry{PID: 12345}
	if err := Save(dir, s); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Daemons["redis"].PID != 12345 {
		t.Errorf("round trip: got PID %d", loaded.Daemons["redis"].PID)
	}
	// Round-trip container daemon entry
	s.Daemons["postgres"] = DaemonEntry{ContainerID: "abc123def456"}
	if err := Save(dir, s); err != nil {
		t.Fatal(err)
	}
	loaded2, _ := Load(dir)
	if loaded2.Daemons["postgres"].ContainerID != "abc123def456" {
		t.Errorf("round trip container: got %q", loaded2.Daemons["postgres"].ContainerID)
	}
}

func TestLoadMissingFile(t *testing.T) {
	dir := t.TempDir()
	s, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if s == nil || s.Daemons == nil {
		t.Fatal("Load of missing file should return empty state")
	}
}

func TestAddRemoveDaemon(t *testing.T) {
	dir := t.TempDir()
	if err := AddDaemon(dir, "redis", 999, ""); err != nil {
		t.Fatal(err)
	}
	s, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if s.Daemons["redis"].PID != 999 {
		t.Errorf("after AddDaemon: got %v", s.Daemons["redis"])
	}
	if err := RemoveDaemon(dir, "redis"); err != nil {
		t.Fatal(err)
	}
	s2, _ := Load(dir)
	if _, ok := s2.Daemons["redis"]; ok {
		t.Error("RemoveDaemon should remove entry")
	}
}

func TestSaveCreatesBakeDir(t *testing.T) {
	dir := t.TempDir()
	bakeDir := filepath.Join(dir, ".bake")
	if err := Save(dir, &State{Daemons: map[string]DaemonEntry{"x": {PID: 1}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(bakeDir); err != nil {
		t.Errorf(".bake dir should exist: %v", err)
	}
}

func TestLoadInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	p := StatePath(dir)
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("not json"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "parse") {
		t.Errorf("error should mention parse: %v", err)
	}
}
