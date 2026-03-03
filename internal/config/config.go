package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// File is the top-level parsed Bakefile: dotenv list, targets, suites.
type File struct {
	Dotenv   []string
	Targets  []*Target
	Suites   []*Suite
	RootDir  string // directory containing Bakefile
	BakePath string // path to Bakefile
}

// Suite is a named entrypoint listing target names (e.g. local, ci).
type Suite struct {
	Name    string
	Targets []string

	// Line and Column are 1-based source positions for error reporting (0 = unknown).
	Line, Column int
}

// DefaultTargetName returns the default target: [target.default] or first target name.
func (f *File) DefaultTargetName() string {
	for _, t := range f.Targets {
		if t.Name == "default" {
			return "default"
		}
	}
	if len(f.Targets) > 0 {
		return f.Targets[0].Name
	}
	return ""
}

// TargetByName returns the target with the given name, or nil.
func (f *File) TargetByName(name string) *Target {
	for _, t := range f.Targets {
		if t.Name == name {
			return t
		}
	}
	return nil
}

// SuiteByName returns the suite with the given name, or nil.
func (f *File) SuiteByName(name string) *Suite {
	for _, s := range f.Suites {
		if s.Name == name {
			return s
		}
	}
	return nil
}

// FindBakefile looks for Bakefile in dir and then parent dirs; returns the directory containing Bakefile and the path to it.
func FindBakefile(dir string) (rootDir, bakefilePath string, err error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", "", fmt.Errorf("resolve path: %w", err)
	}
	d := abs
	for {
		p := filepath.Join(d, "Bakefile")
		if _, err := os.Stat(p); err == nil {
			return d, p, nil
		}
		parent := filepath.Dir(d)
		if parent == d {
			return "", "", fmt.Errorf("no Bakefile found in %s or parents", dir)
		}
		d = parent
	}
}
