package baked

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/evmac/go-bake/internal/config"
	"github.com/evmac/go-bake/internal/dsl"
	"github.com/evmac/go-bake/internal/lint"
)

// EnsureFormatLint runs format -w and lint --fix on the Bakefile at path unless
// BAKE_NO_AUTOFORMAT or BAKE_NO_AUTOLINT are set. Same behavior as ensureBakefileFormatLint in cmd/bake.
func EnsureFormatLint(bakePath string) error {
	if os.Getenv("BAKE_NO_AUTOFORMAT") == "" {
		if err := formatAtPath(bakePath, true); err != nil {
			return err
		}
	}
	if os.Getenv("BAKE_NO_AUTOLINT") == "" {
		if err := lintAtPath(bakePath, true); err != nil {
			return err
		}
	}
	return nil
}

func formatAtPath(path string, write bool) error {
	ast, err := dsl.ParseFile(path)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := dsl.Format(ast, &buf); err != nil {
		return err
	}
	formatted := buf.Bytes()
	if !write {
		return nil
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".bakefmt-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(formatted); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func lintAtPath(bakePath string, applyFix bool) error {
	bakeDir := filepath.Dir(bakePath)
	configPath := lint.ConfigPath(bakeDir)
	cfg, err := lint.LoadConfig(configPath)
	if err != nil {
		return err
	}
	findings, ast, err := lint.RunFromPath(bakePath, cfg)
	if err != nil {
		return err
	}
	if applyFix && len(findings) > 0 {
		hasFixable := false
		for _, f := range findings {
			if f.Fixable {
				hasFixable = true
				break
			}
		}
		if hasFixable {
			lint.ApplyFixes(ast, findings)
			var buf bytes.Buffer
			if err := dsl.Format(ast, &buf); err != nil {
				return err
			}
			if err := os.WriteFile(bakePath, buf.Bytes(), 0644); err != nil {
				return err
			}
		}
		var unfixable []lint.Finding
		for _, f := range findings {
			if !f.Fixable {
				unfixable = append(unfixable, f)
			}
		}
		findings = unfixable
	}
	if len(findings) > 0 {
		// Log unfixable findings; do not fail load (same as bake main)
		_ = lint.PrintFindings(os.Stderr, findings, false)
	}
	return nil
}

// Load finds the Bakefile from dir (current dir or parent), runs format/lint, then loads config with imports.
// Returns rootDir, config, and error. rootDir is the directory containing the Bakefile.
func Load(dir string) (rootDir string, cfg *config.File, err error) {
	rootDir, bakePath, err := config.FindBakefile(dir)
	if err != nil {
		return "", nil, err
	}
	if err := EnsureFormatLint(bakePath); err != nil {
		return "", nil, err
	}
	cfg, err = dsl.LoadWithImports(bakePath)
	if err != nil {
		return "", nil, err
	}
	return rootDir, cfg, nil
}
