package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// File is the top-level parsed Bakefile: dotenv list, imports, profiles, targets, suites.
type File struct {
	Dotenv   []string
	Imports  []string // paths to other Bakefiles (relative or absolute); resolved and merged at load time
	Profiles []*Profile
	Targets  []*Target
	Suites   []*Suite
	RootDir  string // directory containing Bakefile
	BakePath string // path to Bakefile
}

// Daemon is a long-running unit (steps, env, cwd, image, networks, volumes); schedule in workflow, lifecycle in state.
type Daemon struct {
	Name     string
	Steps    []Step
	Env      map[string]string
	Cwd      string
	Image    string
	Unsafe   bool
	Networks []string
	Volumes  []VolumeRef
	Line     int
	Column   int
}

// Profile is a named overlay of dotenv files and env vars (activated by --profile or BAKE_PROFILE).
type Profile struct {
	Name   string
	Dotenv []string          // paths relative to root; loaded when profile is active
	Env    map[string]string // env overlay when profile is active
}

// Suite is a named entrypoint listing target names or "target preset" (e.g. build, test cover).
type Suite struct {
	Name    string
	Targets []string // each is "target" or "target preset" (space-separated)

	// Line and Column are 1-based source positions for error reporting (0 = unknown).
	Line, Column int
}

// ParseSuiteEntry splits a suite target entry "target" or "target preset" into (targetName, presetName).
// presetName is empty if the entry is just a target name.
func ParseSuiteEntry(entry string) (targetName, presetName string) {
	parts := strings.SplitN(entry, " ", 2)
	targetName = strings.TrimSpace(parts[0])
	if len(parts) == 2 {
		presetName = strings.TrimSpace(parts[1])
	}
	return targetName, presetName
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

// ProfileByName returns the profile with the given name, or nil.
func (f *File) ProfileByName(name string) *Profile {
	for _, p := range f.Profiles {
		if p.Name == name {
			return p
		}
	}
	return nil
}

// DaemonByName returns the daemon with the given name from any target's Daemons (e.g. target up), or nil.
func (f *File) DaemonByName(name string) *Daemon {
	for _, t := range f.Targets {
		for _, d := range t.Daemons {
			if d != nil && d.Name == name {
				return d
			}
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
