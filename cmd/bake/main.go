// Package main is the Bake CLI: a minimal Make replacement with explicit DAG, typed args, and repo-scoped commands.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/evmac/go-bake/internal/config"
	"github.com/evmac/go-bake/internal/dsl"
	"github.com/evmac/go-bake/internal/env"
	"github.com/evmac/go-bake/internal/resolve"
	"github.com/evmac/go-bake/internal/runner"
)

func main() {
	code, err := RunMain(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "bake: %v\n", err)
		os.Exit(code)
	}
}

// RunMain runs the CLI with the given args (excluding program name). Returns exit code (1 = run failure, 2 = usage/config) and error.
func RunMain(args []string) (int, error) {
	fs := flag.NewFlagSet("bake", flag.ContinueOnError)
	list := fs.Bool("list", false, "List targets (suite-filtered)")
	ci := fs.Bool("ci", false, "Use CI suite for --list; run in CI mode")
	dryRun := fs.Bool("dry-run", false, "Print commands and dependency order, do not run")
	explain := fs.String("explain", "", "Show dependency chain, resolved vars, and commands for target")
	why := fs.String("why", "", "Explain why target would run or be skipped (incremental build)")
	debug := fs.Bool("debug", false, "Enable debug logging (or set BAKE_DEBUG=1)")
	if err := fs.Parse(args); err != nil {
		return 2, err
	}
	if os.Getenv("BAKE_DEBUG") == "1" || os.Getenv("BAKE_DEBUG") == "true" {
		*debug = true
	}

	if *list {
		if err := runList(*ci); err != nil {
			return 2, err
		}
		return 0, nil
	}
	posArgs := fs.Args()
	if *explain != "" {
		if err := runExplain(*explain, *debug); err != nil {
			return 2, err
		}
		return 0, nil
	}
	if *why != "" {
		if err := runWhy(*why, posArgs); err != nil {
			return 2, err
		}
		return 0, nil
	}
	if len(posArgs) == 0 {
		if err := runDefault(); err != nil {
			return 2, err
		}
		return 0, nil
	}
	target := posArgs[0]
	if target == "install" {
		if err := runInstall(); err != nil {
			return 2, err
		}
		return 0, nil
	}
	cfg, err := loadConfig()
	if err != nil {
		return 2, err
	}
	if su := cfg.SuiteByName(target); su != nil {
		for _, name := range su.Targets {
			tgt := cfg.TargetByName(name)
			if tgt == nil {
				continue
			}
			declared, live, passthrough, _ := resolve.ParseArgs(tgt, nil)
			if *dryRun {
				runDryRun(cfg, name, tgt, declared, live, passthrough)
				continue
			}
			if err := runTarget(cfg, name, tgt, declared, live, passthrough); err != nil {
				return 1, err
			}
		}
		return 0, nil
	}
	tgt := cfg.TargetByName(target)
	if tgt == nil {
		return 2, fmt.Errorf("unknown target %q (use 'bake --list')", target)
	}
	declared, live, passthrough, err := resolve.ParseArgs(tgt, posArgs[1:])
	if err != nil {
		return 2, err
	}
	if *dryRun {
		runDryRun(cfg, target, tgt, declared, live, passthrough)
		return 0, nil
	}
	if err := runTarget(cfg, target, tgt, declared, live, passthrough); err != nil {
		return 1, err
	}
	return 0, nil
}

func loadConfig() (*config.File, error) {
	_, path, err := config.FindBakefile(".")
	if err != nil {
		return nil, err
	}
	return dsl.LoadWithImports(path)
}

func runList(ciMode bool) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	suiteName := "local"
	if ciMode || os.Getenv("CI") == "true" || os.Getenv("CI") == "1" {
		suiteName = "ci"
	}
	targets := cfg.Targets
	if su := cfg.SuiteByName(suiteName); su != nil {
		// Filter to only targets in this suite
		names := make(map[string]bool)
		for _, n := range su.Targets {
			names[n] = true
		}
		var filtered []*config.Target
		for _, t := range cfg.Targets {
			if names[t.Name] {
				filtered = append(filtered, t)
			}
		}
		targets = filtered
	}
	for _, t := range targets {
		if t.Desc != "" {
			fmt.Printf("%s\t%s\n", t.Name, t.Desc)
		} else {
			fmt.Println(t.Name)
		}
	}
	return nil
}

func runDefault() error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	name := cfg.DefaultTargetName()
	if name == "" {
		return fmt.Errorf("no target specified (use 'bake <target>' or 'bake --list')")
	}
	tgt := cfg.TargetByName(name)
	return runTarget(cfg, name, tgt, nil, nil, nil)
}

func runTarget(cfg *config.File, name string, tgt *config.Target, declared, live map[string]string, passthrough []string) error {
	if tgt == nil {
		return fmt.Errorf("unknown target %q (use 'bake --list')", name)
	}
	if len(tgt.Steps) == 0 {
		return nil
	}
	if declared == nil {
		declared = make(map[string]string)
	}
	if live == nil {
		live = make(map[string]string)
	}
	opts := runner.RunOptions{
		RootDir:      cfg.RootDir,
		Dotenv:       cfg.Dotenv,
		TargetEnv:    tgt.Env,
		DeclaredArgs: declared,
		LiveArgs:     live,
		Passthrough:  passthrough,
	}
	return runner.Run(context.Background(), cfg, name, opts)
}

func runDryRun(cfg *config.File, name string, tgt *config.Target, declared, live map[string]string, passthrough []string) {
	order, _ := runner.TopoOrder(cfg, name)
	for _, n := range order {
		fmt.Printf("target %s\n", n)
		t := cfg.TargetByName(n)
		if t == nil {
			continue
		}
		data := resolve.TemplateData(declared, live)
		for i, step := range t.Steps {
			argv := step.Argv
			if len(data) > 0 {
				argv, _ = resolve.ExpandArgv(argv, data)
			}
			if i+1 == t.PassthroughStep || (t.PassthroughStep <= 0 && i+1 == len(t.Steps)) {
				argv = append(argv, passthrough...)
			}
			if len(argv) > 0 {
				fmt.Printf("  step %d: %v\n", i+1, argv)
			}
		}
	}
}

func runWhy(targetName string, args []string) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	tgt := cfg.TargetByName(targetName)
	if tgt == nil {
		return fmt.Errorf("unknown target %q", targetName)
	}
	declared, live, _, err := resolve.ParseArgs(tgt, nil)
	if err != nil {
		return err
	}
	if len(args) > 1 {
		declared, live, _, err = resolve.ParseArgs(tgt, args[1:])
		if err != nil {
			return err
		}
	}
	dotenvMap, err := env.LoadDotenv(cfg.RootDir, cfg.Dotenv)
	if err != nil {
		return err
	}
	opts := runner.RunOptions{
		RootDir:      cfg.RootDir,
		Dotenv:       cfg.Dotenv,
		TargetEnv:    tgt.Env,
		DeclaredArgs: declared,
		LiveArgs:     live,
	}
	reasons, err := runner.WhyReasons(cfg.RootDir, tgt, opts, dotenvMap)
	if err != nil {
		return err
	}
	for _, r := range reasons {
		fmt.Println(r)
	}
	return nil
}

func runExplain(targetName string, debug bool) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	tgt := cfg.TargetByName(targetName)
	if tgt == nil {
		return fmt.Errorf("unknown target %q", targetName)
	}
	order, err := runner.TopoOrder(cfg, targetName)
	if err != nil {
		return err
	}
	fmt.Printf("target: %s\n", targetName)
	fmt.Printf("deps (order): %v\n", order)
	if tgt.Desc != "" {
		fmt.Printf("desc: %s\n", tgt.Desc)
	}
	if len(tgt.Args) > 0 {
		fmt.Printf("args: %v\n", tgt.Args)
	}
	fmt.Println("steps:")
	data := resolve.TemplateData(nil, nil)
	for i, step := range tgt.Steps {
		argv := step.Argv
		if len(data) > 0 {
			argv, _ = resolve.ExpandArgv(argv, data)
		}
		fmt.Printf("  %d: %v\n", i+1, argv)
	}
	if len(tgt.Env) > 0 {
		fmt.Printf("env: %v\n", tgt.Env)
	}
	return nil
}

func runInstall() error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	binDir := filepath.Join(cfg.RootDir, ".bake", "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		return fmt.Errorf("create .bake/bin: %w", err)
	}
	bakeExe, err := os.Executable()
	if err != nil {
		bakeExe = "bake"
	}
	names := make(map[string]bool)
	for _, t := range cfg.Targets {
		names[t.Name] = true
	}
	for _, su := range cfg.Suites {
		names[su.Name] = true
	}
	for name := range names {
		shimPath := filepath.Join(binDir, name)
		script := fmt.Sprintf("#!/bin/sh\nexec %q %s \"$@\"\n", bakeExe, name)
		if err := os.WriteFile(shimPath, []byte(script), 0755); err != nil {
			return fmt.Errorf("write shim %s: %w", name, err)
		}
	}
	fmt.Fprintf(os.Stderr, "bake: installed shims in %s (add to PATH: export PATH=\"%s:$PATH\")\n", binDir, binDir)
	return nil
}
