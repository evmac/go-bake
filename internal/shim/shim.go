package shim

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/evmac/go-bake/internal/config"
)

// WriteShims writes shims for every target and suite name in cfg to rootDir/.bake/bin.
// bakeExe is the path or name of the bake binary (e.g. "bake"); if empty, "bake" is used.
// Removes any existing shim in the bin dir that is not a current target or suite name.
func WriteShims(rootDir string, cfg *config.File, bakeExe string) error {
	if cfg == nil {
		return nil
	}
	binDir := filepath.Join(rootDir, ".bake", "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		return fmt.Errorf("create .bake/bin: %w", err)
	}
	if bakeExe == "" {
		bakeExe = "bake"
	}
	names := make(map[string]bool)
	for _, t := range cfg.Targets {
		names[t.Name] = true
	}
	for _, su := range cfg.Suites {
		names[su.Name] = true
	}

	// Remove shims that are no longer valid
	entries, err := os.ReadDir(binDir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !names[e.Name()] {
			if err := os.Remove(filepath.Join(binDir, e.Name())); err != nil {
				return fmt.Errorf("remove stale shim %s: %w", e.Name(), err)
			}
		}
	}

	for name := range names {
		shimPath := filepath.Join(binDir, name)
		script := fmt.Sprintf("#!/bin/sh\nexec %q %s \"$@\"\n", bakeExe, name)
		if err := os.WriteFile(shimPath, []byte(script), 0755); err != nil {
			return fmt.Errorf("write shim %s: %w", name, err)
		}
	}
	return nil
}
