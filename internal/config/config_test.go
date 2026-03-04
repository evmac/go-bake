package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultTargetName(t *testing.T) {
	// No targets -> empty
	cfg := &File{}
	if got := cfg.DefaultTargetName(); got != "" {
		t.Errorf("empty file: got %q", got)
	}
	// First target wins when no "default"
	cfg = &File{Targets: []*Target{{Name: "build"}, {Name: "test"}}}
	if got := cfg.DefaultTargetName(); got != "build" {
		t.Errorf("got %q", got)
	}
	// Explicit "default" wins
	cfg = &File{Targets: []*Target{{Name: "build"}, {Name: "default"}, {Name: "test"}}}
	if got := cfg.DefaultTargetName(); got != "default" {
		t.Errorf("got %q", got)
	}
}

func TestSuiteByName(t *testing.T) {
	cfg := &File{
		Suites: []*Suite{{Name: "local", Targets: []string{"build"}}, {Name: "ci", Targets: []string{"build", "test"}}},
	}
	if s := cfg.SuiteByName("local"); s == nil || s.Name != "local" || len(s.Targets) != 1 {
		t.Errorf("SuiteByName(local): %+v", s)
	}
	if s := cfg.SuiteByName("ci"); s == nil || len(s.Targets) != 2 {
		t.Errorf("SuiteByName(ci): %+v", s)
	}
	if s := cfg.SuiteByName("missing"); s != nil {
		t.Errorf("expected nil for missing suite, got %+v", s)
	}
}

func TestFindBakefile(t *testing.T) {
	dir := t.TempDir()
	// No Bakefile in dir or parents
	_, _, err := FindBakefile(dir)
	if err == nil {
		t.Fatal("expected error when no Bakefile")
	}
	// Create Bakefile in dir
	bakePath := filepath.Join(dir, "Bakefile")
	if err := os.WriteFile(bakePath, []byte("target build { steps { exec [\"true\"] } }\n"), 0644); err != nil {
		t.Fatal(err)
	}
	root, path, err := FindBakefile(dir)
	if err != nil {
		t.Fatal(err)
	}
	if root != dir || path != bakePath {
		t.Errorf("got root=%q path=%q", root, path)
	}
	// From subdir, should find parent Bakefile
	sub := filepath.Join(dir, "sub", "nested")
	os.MkdirAll(sub, 0755)
	root2, path2, err := FindBakefile(sub)
	if err != nil {
		t.Fatal(err)
	}
	if root2 != dir || path2 != bakePath {
		t.Errorf("from subdir: got root=%q path=%q", root2, path2)
	}
}
