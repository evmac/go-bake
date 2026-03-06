package env

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadDotenv(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".env", "A=1\nB=2\n# comment\n\nC=3\n")
	got, err := LoadDotenv(dir, []string{".env"})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"A": "1", "B": "2", "C": "3"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestLoadDotenvOrder(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".env", "X=first\n")
	writeFile(t, dir, ".env.local", "X=second\n")
	got, err := LoadDotenv(dir, []string{".env", ".env.local"})
	if err != nil {
		t.Fatal(err)
	}
	if got["X"] != "second" {
		t.Errorf("got X=%q want second", got["X"])
	}
}

func TestLoadDotenvMissingFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".env", "A=1\n")
	got, err := LoadDotenv(dir, []string{".env", ".env.missing"})
	if err != nil {
		t.Fatal(err)
	}
	if got["A"] != "1" {
		t.Errorf("got A=%q want 1", got["A"])
	}
}

func TestMerge(t *testing.T) {
	processEnv := []string{"P=process", "SHARED=from_process"}
	dotenv := map[string]string{"D": "dot", "SHARED": "from_dotenv"}
	targetEnv := map[string]string{"T": "target", "SHARED": "from_target"}
	got := Merge(processEnv, dotenv, targetEnv, nil)
	want := map[string]string{
		"P":      "process",
		"D":      "dot",
		"T":      "target",
		"SHARED": "from_target",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
	// CLI env overrides target
	cliEnv := map[string]string{"SHARED": "from_cli", "C": "cli"}
	got = Merge(processEnv, dotenv, targetEnv, cliEnv)
	want = map[string]string{
		"P":      "process",
		"D":      "dot",
		"T":      "target",
		"SHARED": "from_cli",
		"C":      "cli",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("with cliEnv: got %v want %v", got, want)
	}
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
