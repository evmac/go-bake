// Package main is the Bake CLI: a minimal Make replacement with explicit DAG, typed args, and repo-scoped commands.
package main

import (
	"bufio"
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/evmac/go-bake/internal/config"
	"github.com/evmac/go-bake/internal/dsl"
	"github.com/evmac/go-bake/internal/env"
	"github.com/evmac/go-bake/internal/resolve"
	"github.com/evmac/go-bake/internal/runner"
)

// setFlags collects repeated --set "key=value" (env.VAR=val or args.NAME=val).
type setFlags []string

func (s *setFlags) String() string { return strings.Join(*s, ",") }
func (s *setFlags) Set(v string) error {
	*s = append(*s, v)
	return nil
}

// parseSetFlags returns env overrides and declared-arg overrides from --set flags.
func parseSetFlags(flags []string) (cliEnv map[string]string, cliArgs map[string]string) {
	cliEnv = make(map[string]string)
	cliArgs = make(map[string]string)
	for _, f := range flags {
		eq := strings.Index(f, "=")
		if eq <= 0 {
			continue
		}
		key, val := strings.TrimSpace(f[:eq]), strings.TrimSpace(f[eq+1:])
		if strings.HasPrefix(key, "env.") {
			cliEnv[strings.TrimPrefix(key, "env.")] = val
		} else if strings.HasPrefix(key, "args.") {
			cliArgs[strings.TrimPrefix(key, "args.")] = val
		}
	}
	return cliEnv, cliArgs
}

// mergeEnv overlays b onto a (b wins). Returns a new map.
func mergeEnv(a, b map[string]string) map[string]string {
	out := make(map[string]string)
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}

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
	listStatus := fs.Bool("status", false, "With --list, show per-target incremental status (would run / skipped)")
	ci := fs.Bool("ci", false, "Use CI suite for --list; run in CI mode")
	dryRun := fs.Bool("dry-run", false, "Print commands and dependency order, do not run")
	showCmd := fs.Bool("show-cmd", false, "Print each command to stderr before running (or set BAKE_SHOW_CMD=1)")
	explain := fs.String("explain", "", "Show dependency chain, resolved vars, and commands for target")
	why := fs.String("why", "", "Explain why target would run or be skipped (incremental build)")
	graph := fs.String("graph", "", "Emit dependency graph (format: dot or json; optional target for subgraph)")
	whatDependsOn := fs.String("what-depends-on", "", "List targets that depend on the given target")
	profileName := fs.String("profile", "", "Use named profile (env/dotenv overlay); or set BAKE_PROFILE")
	choose := fs.Bool("choose", false, "Interactive menu to pick a target; without TTY prints list (pipe to fzf)")
	debug := fs.Bool("debug", false, "Enable debug logging (or set BAKE_DEBUG=1)")
	var setVals setFlags
	fs.Var(&setVals, "set", "Override env or args: --set env.FOO=bar or --set args.NAME=value (repeatable)")
	if err := fs.Parse(args); err != nil {
		return 2, err
	}
	cliEnv, cliArgs := parseSetFlags(setVals)
	if *profileName == "" {
		*profileName = os.Getenv("BAKE_PROFILE")
	}
	if os.Getenv("BAKE_DEBUG") == "1" || os.Getenv("BAKE_DEBUG") == "true" {
		*debug = true
	}
	if os.Getenv("BAKE_SHOW_CMD") == "1" || os.Getenv("BAKE_SHOW_CMD") == "true" {
		*showCmd = true
	}

	posArgs := fs.Args()
	if len(posArgs) > 0 {
		target := posArgs[0]
		if target == "format" || target == "fmt" {
			// Parse format-specific flags from remaining args (e.g. "fmt -w" or "fmt --check").
			formatFs := flag.NewFlagSet("bake format", flag.ContinueOnError)
			writeFormat := formatFs.Bool("w", false, "write to file")
			checkFormat := formatFs.Bool("check", false, "exit 1 if would change")
			_ = formatFs.Parse(posArgs[1:])
			code, err := runFormat(*writeFormat, *checkFormat)
			if err != nil {
				return code, err
			}
			return 0, nil
		}
	}

	if *list {
		if err := runList(*ci, *listStatus); err != nil {
			return 2, err
		}
		return 0, nil
	}
	if *explain != "" {
		if err := runExplain(*explain, *debug); err != nil {
			return 2, err
		}
		return 0, nil
	}
	if *why != "" {
		if err := runWhy(*why, posArgs, cliEnv, cliArgs); err != nil {
			return 2, err
		}
		return 0, nil
	}
	if *graph != "" {
		if err := runGraph(*graph, posArgs); err != nil {
			return 2, err
		}
		return 0, nil
	}
	if *whatDependsOn != "" {
		if err := runWhatDependsOn(*whatDependsOn); err != nil {
			return 2, err
		}
		return 0, nil
	}
	if *choose {
		if err := runChoose(*ci, *profileName, *showCmd, cliEnv, cliArgs); err != nil {
			return 2, err
		}
		return 0, nil
	}
	if len(posArgs) == 0 {
		if err := runDefault(cliEnv, cliArgs, *profileName, *showCmd); err != nil {
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
	var profile *config.Profile
	if *profileName != "" {
		profile = cfg.ProfileByName(*profileName)
		if profile == nil {
			return 2, fmt.Errorf("unknown profile %q", *profileName)
		}
	}
	if su := cfg.SuiteByName(target); su != nil {
		for _, name := range su.Targets {
			tgt := cfg.TargetByName(name)
			if tgt == nil {
				continue
			}
			declared, live, passthrough, _ := resolve.ParseArgs(tgt, nil)
			for k, v := range cliArgs {
				declared[k] = v
			}
			if *dryRun {
				runDryRun(cfg, name, tgt, declared, live, passthrough, cliEnv)
				continue
			}
			if err := runTarget(cfg, name, tgt, declared, live, passthrough, cliEnv, profile, *showCmd); err != nil {
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
	for k, v := range cliArgs {
		declared[k] = v
	}
	if *dryRun {
		runDryRun(cfg, target, tgt, declared, live, passthrough, cliEnv)
		return 0, nil
	}
	if err := runTarget(cfg, target, tgt, declared, live, passthrough, cliEnv, profile, *showCmd); err != nil {
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

func runList(ciMode, withStatus bool) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	suiteName := "dev"
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
	// Exclude private targets from list
	var listTargets []*config.Target
	for _, t := range targets {
		if !t.Private {
			listTargets = append(listTargets, t)
		}
	}
	dotenvMap, _ := env.LoadDotenv(cfg.RootDir, cfg.Dotenv)
	for _, t := range listTargets {
		if withStatus {
			opts := runner.RunOptions{
				RootDir: cfg.RootDir, Dotenv: cfg.Dotenv, TargetEnv: t.Env,
				DeclaredArgs: nil, LiveArgs: nil,
			}
			reasons, err := runner.WhyReasons(cfg.RootDir, t, opts, dotenvMap)
			status := "?"
			if err == nil && len(reasons) > 0 {
				last := reasons[len(reasons)-1]
				if strings.Contains(last, "would skip") || strings.Contains(last, "up to date") {
					status = "skipped"
				} else {
					status = "would run"
				}
			}
			if t.Desc != "" {
				fmt.Printf("%s\t%s\t%s\n", t.Name, t.Desc, status)
			} else {
				fmt.Printf("%s\t%s\n", t.Name, status)
			}
		} else {
			if t.Desc != "" {
				fmt.Printf("%s\t%s\n", t.Name, t.Desc)
			} else {
				fmt.Println(t.Name)
			}
		}
	}
	return nil
}

func runDefault(cliEnv, cliArgs map[string]string, profileName string, showCmd bool) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	var profile *config.Profile
	if profileName != "" {
		profile = cfg.ProfileByName(profileName)
		if profile == nil {
			return fmt.Errorf("unknown profile %q", profileName)
		}
	}
	name := cfg.DefaultTargetName()
	if name == "" {
		return fmt.Errorf("no target specified (use 'bake <target>' or 'bake --list')")
	}
	tgt := cfg.TargetByName(name)
	declared, live, _, _ := resolve.ParseArgs(tgt, nil)
	if declared == nil {
		declared = make(map[string]string)
	}
	for k, v := range cliArgs {
		declared[k] = v
	}
	return runTarget(cfg, name, tgt, declared, live, nil, cliEnv, profile, showCmd)
}

func runTarget(cfg *config.File, name string, tgt *config.Target, declared, live map[string]string, passthrough []string, cliEnv map[string]string, profile *config.Profile, showCmd bool) error {
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
	dotenv := cfg.Dotenv
	targetEnv := tgt.Env
	if profile != nil {
		dotenv = append(append([]string(nil), cfg.Dotenv...), profile.Dotenv...)
		targetEnv = mergeEnv(profile.Env, tgt.Env)
	}
	opts := runner.RunOptions{
		RootDir:      cfg.RootDir,
		Dotenv:       dotenv,
		TargetEnv:    targetEnv,
		CLIEnv:       cliEnv,
		DeclaredArgs: declared,
		LiveArgs:     live,
		Passthrough:  passthrough,
		ShowCmd:      showCmd,
	}
	return runner.Run(context.Background(), cfg, name, opts)
}

func runDryRun(cfg *config.File, name string, tgt *config.Target, declared, live map[string]string, passthrough []string, cliEnv map[string]string) {
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

func runWhy(targetName string, args []string, cliEnv, cliArgs map[string]string) error {
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
	for k, v := range cliArgs {
		declared[k] = v
	}
	dotenvMap, err := env.LoadDotenv(cfg.RootDir, cfg.Dotenv)
	if err != nil {
		return err
	}
	opts := runner.RunOptions{
		RootDir:      cfg.RootDir,
		Dotenv:       cfg.Dotenv,
		TargetEnv:    tgt.Env,
		CLIEnv:       cliEnv,
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

func runGraph(format string, posArgs []string) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	edges := runner.AllEdges(cfg)
	if format == "" {
		format = "dot"
	}
	var nodes []string
	if len(posArgs) > 0 {
		targetName := posArgs[0]
		if cfg.TargetByName(targetName) == nil {
			return fmt.Errorf("unknown target %q", targetName)
		}
		order, err := runner.TopoOrder(cfg, targetName)
		if err != nil {
			return err
		}
		nodes = order
	} else {
		for name := range edges {
			nodes = append(nodes, name)
		}
		sort.Strings(nodes)
	}
	nodeSet := make(map[string]bool)
	for _, n := range nodes {
		nodeSet[n] = true
	}
	if format == "json" {
		fmt.Println(`{"nodes":[`)
		for i, n := range nodes {
			if i > 0 {
				fmt.Print(",")
			}
			fmt.Printf("%q", n)
		}
		fmt.Println("],\"edges\":[")
		first := true
		for _, from := range nodes {
			for _, to := range edges[from] {
				if nodeSet[to] {
					if !first {
						fmt.Print(",")
					}
					first = false
					fmt.Printf("[%q,%q]", from, to)
				}
			}
		}
		fmt.Println("]}")
		return nil
	}
	fmt.Println("digraph {")
	for _, from := range nodes {
		for _, to := range edges[from] {
			if nodeSet[to] {
				fmt.Printf("  %q -> %q;\n", from, to)
			}
		}
	}
	fmt.Println("}")
	return nil
}

func runChoose(ciMode bool, profileName string, showCmd bool, cliEnv, cliArgs map[string]string) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	suiteName := "dev"
	if ciMode || os.Getenv("CI") == "true" || os.Getenv("CI") == "1" {
		suiteName = "ci"
	}
	targets := cfg.Targets
	if su := cfg.SuiteByName(suiteName); su != nil {
		names := make(map[string]bool)
		for _, n := range su.Targets {
			names[n] = true
		}
		var filtered []*config.Target
		for _, t := range cfg.Targets {
			if names[t.Name] && !t.Private {
				filtered = append(filtered, t)
			}
		}
		targets = filtered
	} else {
		var listTargets []*config.Target
		for _, t := range cfg.Targets {
			if !t.Private {
				listTargets = append(listTargets, t)
			}
		}
		targets = listTargets
	}
	if len(targets) == 0 {
		return fmt.Errorf("no targets to choose from")
	}
	// When stdout is not a TTY, just print target names (for piping to fzf).
	if fi, err := os.Stdout.Stat(); err != nil || (fi.Mode()&os.ModeCharDevice) == 0 {
		for _, t := range targets {
			fmt.Println(t.Name)
		}
		return nil
	}
	// Interactive: show numbered menu and read choice.
	for i, t := range targets {
		if t.Desc != "" {
			fmt.Fprintf(os.Stderr, "  %d) %s\t%s\n", i+1, t.Name, t.Desc)
		} else {
			fmt.Fprintf(os.Stderr, "  %d) %s\n", i+1, t.Name)
		}
	}
	fmt.Fprint(os.Stderr, "choice (number or name): ")
	sc := bufio.NewScanner(os.Stdin)
	if !sc.Scan() {
		return nil
	}
	line := strings.TrimSpace(sc.Text())
	if line == "" {
		return nil
	}
	var chosen *config.Target
	if n, err := strconv.Atoi(line); err == nil && n >= 1 && n <= len(targets) {
		chosen = targets[n-1]
	} else {
		for _, t := range targets {
			if t.Name == line {
				chosen = t
				break
			}
		}
	}
	if chosen == nil {
		return fmt.Errorf("invalid choice %q", line)
	}
	var profile *config.Profile
	if profileName != "" {
		profile = cfg.ProfileByName(profileName)
		if profile == nil {
			return fmt.Errorf("unknown profile %q", profileName)
		}
	}
	declared, live, _, _ := resolve.ParseArgs(chosen, nil)
	for k, v := range cliArgs {
		declared[k] = v
	}
	return runTarget(cfg, chosen.Name, chosen, declared, live, nil, cliEnv, profile, showCmd)
}

func runWhatDependsOn(targetName string) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	if cfg.TargetByName(targetName) == nil {
		return fmt.Errorf("unknown target %q", targetName)
	}
	var out []string
	for _, t := range cfg.Targets {
		for _, d := range t.Deps {
			if d == targetName {
				out = append(out, t.Name)
				break
			}
		}
	}
	sort.Strings(out)
	for _, name := range out {
		fmt.Println(name)
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

// runFormat runs the Bakefile formatter. Returns (exitCode, error); exit code 1 when --check and file would change.
func runFormat(write, check bool) (int, error) {
	_, path, err := config.FindBakefile(".")
	if err != nil {
		return 2, err
	}
	ast, err := dsl.ParseFile(path)
	if err != nil {
		return 2, err
	}
	var buf bytes.Buffer
	if err := dsl.Format(ast, &buf); err != nil {
		return 2, err
	}
	formatted := buf.Bytes()
	if check {
		orig, err := os.ReadFile(path)
		if err != nil {
			return 2, fmt.Errorf("read Bakefile: %w", err)
		}
		if !bytes.Equal(orig, formatted) {
			return 1, fmt.Errorf("Bakefile is not formatted (run bake fmt -w)")
		}
		return 0, nil
	}
	if write {
		// Write to temp file then rename for atomicity.
		tmp, err := os.CreateTemp(filepath.Dir(path), ".bakefmt-*")
		if err != nil {
			return 2, fmt.Errorf("create temp file: %w", err)
		}
		tmpPath := tmp.Name()
		defer os.Remove(tmpPath)
		if _, err := tmp.Write(formatted); err != nil {
			tmp.Close()
			return 2, err
		}
		if err := tmp.Close(); err != nil {
			return 2, fmt.Errorf("close temp file: %w", err)
		}
		if err := os.Rename(tmpPath, path); err != nil {
			return 2, fmt.Errorf("replace Bakefile: %w", err)
		}
		return 0, nil
	}
	_, err = os.Stdout.Write(formatted)
	return 0, err
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
