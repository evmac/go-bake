package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestBakeListExitCode(t *testing.T) {
	// Build bake binary and run bake --list from repo root; assert we get exit 0 or 2 (no Bakefile yet is ok for bootstrap)
	exe := buildBake(t)
	dir := repoRoot(t)
	cmd := exec.Command(exe, "--list")
	cmd.Dir = dir
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 2 {
			// Expected when no Bakefile or not implemented
			return
		}
		t.Logf("output: %s", out)
		t.Fatalf("bake --list: %v", err)
	}
	t.Logf("bake --list: %s", out)
}

func TestBakeNoTargetShowsError(t *testing.T) {
	exe := buildBake(t)
	// Run from a dir with no Bakefile so we get "no Bakefile" or "no target" error
	dir := t.TempDir()
	cmd := exec.Command(exe)
	cmd.Dir = dir
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected non-zero exit when no target and no Bakefile, got output: %s", out)
	}
	if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() != 2 {
		t.Logf("output: %s", out)
		t.Fatalf("expected exit 2, got: %v", err)
	}
}

func buildBake(t *testing.T) string {
	t.Helper()
	dir := repoRoot(t)
	exe := filepath.Join(dir, "bake.test.exe")
	cmd := exec.Command("go", "build", "-o", exe, ".")
	cmd.Dir = filepath.Join(dir, "cmd", "bake")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return exe
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	// From cmd/bake we need to go up to repo root
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repo root (go.mod)")
		}
		dir = parent
	}
}
