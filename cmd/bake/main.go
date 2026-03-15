// Package main is the Bake CLI: a minimal Make replacement with explicit DAG, typed args, and repo-scoped commands.
package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/evmac/go-bake/internal/cache"
	"github.com/evmac/go-bake/internal/watch"

	"github.com/evmac/go-bake/internal/config"
	"github.com/evmac/go-bake/internal/daemonclient"
	"github.com/evmac/go-bake/internal/daemonproto"
	"github.com/evmac/go-bake/internal/dsl"
	"github.com/evmac/go-bake/internal/env"
	"github.com/evmac/go-bake/internal/lifecycle"
	"github.com/evmac/go-bake/internal/lint"
	"github.com/evmac/go-bake/internal/resolve"
	"github.com/evmac/go-bake/internal/runner"
	"github.com/evmac/go-bake/internal/shim"
	"github.com/robfig/cron/v3"
)

// Version is set at build time via -ldflags "-X main.Version=...". Default "dev" for local builds.
var Version = "dev"

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
	jsonOut := fs.Bool("json", false, "Emit NDJSON event stream to stdout (run events); step output goes to stderr")
	maxParallel := fs.Int("max-parallel", 1, "Max targets to run at once (by level); 1 = sequential")
	timing := fs.Bool("timing", false, "Print per-target duration summary after run")
	artifacts := fs.Bool("artifacts", false, "Print output paths of targets that produced artifacts")
	watch := fs.Bool("watch", false, "Re-run target when inputs change (poll-based)")
	noDaemon := fs.Bool("no-daemon", false, "Run in-process only; do not start or use baked daemon")
	debug := fs.Bool("debug", false, "Enable debug logging (or set BAKE_DEBUG=1)")
	showVersion := fs.Bool("version", false, "Print version and exit")
	var setVals setFlags
	fs.Var(&setVals, "set", "Override env or args: --set env.FOO=bar or --set args.NAME=value (repeatable)")
	if err := fs.Parse(args); err != nil {
		return 2, err
	}
	if *showVersion {
		fmt.Println(Version)
		return 0, nil
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
		if target == "lint" {
			lintFs := flag.NewFlagSet("bake lint", flag.ContinueOnError)
			lintFix := lintFs.Bool("fix", false, "apply auto-fixes where possible")
			lintConfig := lintFs.String("config", "", "path to linter config (default: .bake/config in Bakefile dir)")
			lintJSON := lintFs.Bool("json", false, "output findings as NDJSON")
			lintDisable := lintFs.String("disable", "", "disable a lint rule by ID and persist to config")
			_ = lintFs.Parse(posArgs[1:])
			if *lintDisable != "" {
				code, err := runLintDisable(*lintDisable, *lintConfig)
				if err != nil {
					return code, err
				}
				return code, nil
			}
			code, err := runLint(*lintFix, *lintConfig, *lintJSON)
			if err != nil {
				return code, err
			}
			return code, nil
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
		if err := runDefault(cliEnv, cliArgs, *profileName, *showCmd, *jsonOut, *maxParallel, *timing, *artifacts); err != nil {
			return 2, err
		}
		return 0, nil
	}
	target := posArgs[0]
	useDaemon := !*noDaemon && os.Getenv("BAKE_DAEMON") != "0" && os.Getenv("BAKE_NO_DAEMON") == ""
	if useDaemon && (target == "up" || target == "down" || target != "install") {
		if code, err := tryDaemon(target, posArgs); err == nil {
			return code, nil
		}
		// Fall back to in-process
	}
	if *watch {
		if err := runWatch(context.Background(), target, posArgs[1:], cliEnv, cliArgs, *profileName, *showCmd, *maxParallel); err != nil {
			return 2, err
		}
		return 0, nil
	}
	if target == "install" {
		sub := ""
		if len(posArgs) > 1 {
			sub = posArgs[1]
		}
		if sub == "shims" {
			if err := runInstallShims(); err != nil {
				return 2, err
			}
			return 0, nil
		}
		if sub == "hooks" {
			if err := runInstallHooks(); err != nil {
				return 2, err
			}
			return 0, nil
		}
		if sub == "daemon" {
			if err := runInstallDaemon(); err != nil {
				return 2, err
			}
			return 0, nil
		}
		if sub != "" {
			return 2, fmt.Errorf("unknown install subcommand %q (use: install, install shims, install hooks, install daemon)", sub)
		}
		if err := runInstall(); err != nil {
			return 2, err
		}
		return 0, nil
	}
	if target == "up" {
		if err := runUp(nil, cliEnv, *profileName, *showCmd); err != nil {
			return 1, err
		}
		return 0, nil
	}
	if target == "down" {
		daemonName := ""
		if len(posArgs) > 1 {
			daemonName = posArgs[1]
		}
		if err := runDown(daemonName); err != nil {
			return 1, err
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
		for _, entry := range su.Targets {
			targetName, presetName := config.ParseSuiteEntry(entry)
			tgt := cfg.TargetByName(targetName)
			if tgt == nil {
				continue
			}
			var preset *config.Preset
			if presetName != "" {
				preset = tgt.PresetByName(presetName)
			}
			declared, live, passthroughByStep, _ := resolve.ParseArgs(tgt, nil)
			for k, v := range cliArgs {
				declared[k] = v
			}
			if *dryRun {
				runDryRun(cfg, targetName, tgt, declared, live, passthroughByStep, cliEnv, preset)
				continue
			}
			if err := runTarget(cfg, targetName, tgt, declared, live, passthroughByStep, cliEnv, profile, preset, *showCmd, *jsonOut, *maxParallel, *timing, *artifacts); err != nil {
				return 1, err
			}
		}
		return 0, nil
	}
	tgt := cfg.TargetByName(target)
	if tgt == nil {
		return 2, fmt.Errorf("unknown target %q (use 'bake --list')", target)
	}
	// Resolve optional preset: bake test cover => target test, preset cover; args start at posArgs[2]
	argsForTarget := posArgs[1:]
	var preset *config.Preset
	if len(posArgs) > 1 && !strings.HasPrefix(posArgs[1], "-") && tgt.PresetByName(posArgs[1]) != nil {
		preset = tgt.PresetByName(posArgs[1])
		argsForTarget = posArgs[2:]
	}
	declared, live, passthroughByStep, err := resolve.ParseArgs(tgt, argsForTarget)
	if err != nil {
		return 2, err
	}
	for k, v := range cliArgs {
		declared[k] = v
	}
	if *dryRun {
		runDryRun(cfg, target, tgt, declared, live, passthroughByStep, cliEnv, preset)
		return 0, nil
	}
	if err := runTarget(cfg, target, tgt, declared, live, passthroughByStep, cliEnv, profile, preset, *showCmd, *jsonOut, *maxParallel, *timing, *artifacts); err != nil {
		return 1, err
	}
	return 0, nil
}

func tryDaemon(target string, posArgs []string) (int, error) {
	rootDir, _, err := config.FindBakefile(".")
	if err != nil {
		return 0, err
	}
	if err := daemonclient.EnsureStarted(rootDir); err != nil {
		return 0, err
	}
	conn, err := daemonclient.Dial(rootDir)
	if err != nil {
		return 0, err
	}
	defer conn.Close()
	req := &daemonproto.Request{}
	switch target {
	case "up":
		req.Up = true
	case "down":
		req.Down = true
		if len(posArgs) > 1 {
			req.Daemon = posArgs[1]
		}
	default:
		req.Run = target
	}
	resp, err := daemonclient.SendRequest(conn, req)
	if err != nil {
		return 0, err
	}
	if resp.Output != "" {
		fmt.Fprint(os.Stdout, resp.Output)
	}
	if resp.Error != "" {
		fmt.Fprintf(os.Stderr, "bake: %s\n", resp.Error)
	}
	return resp.ExitCode, nil
}

func loadConfig() (*config.File, error) {
	_, path, err := config.FindBakefile(".")
	if err != nil {
		return nil, err
	}
	if err := ensureBakefileFormatLint(path); err != nil {
		return nil, err
	}
	return dsl.LoadWithImports(path)
}

// ensureBakefileFormatLint runs format -w and lint --fix on the Bakefile at path unless
// BAKE_NO_AUTOFORMAT or BAKE_NO_AUTOLINT are set. Used automatically when loading config.
// Lint applies fixes and only surfaces unfixable errors; does not fail the load.
func ensureBakefileFormatLint(path string) error {
	if os.Getenv("BAKE_NO_AUTOFORMAT") == "" {
		if _, err := runFormatAtPath(path, true, false); err != nil {
			return err
		}
	}
	if os.Getenv("BAKE_NO_AUTOLINT") == "" {
		if _, err := runLintAtPath(path, "", true, false); err != nil {
			return err
		}
	}
	return nil
}

func runLint(applyFix bool, configPath string, jsonOut bool) (int, error) {
	_, bakePath, err := config.FindBakefile(".")
	if err != nil {
		return 2, err
	}
	return runLintAtPath(bakePath, configPath, applyFix, jsonOut)
}

// runLintAtPath runs the linter on the Bakefile at path. When applyFix is true, applies fixes,
// writes the file, then reports only unfixable findings (fix all errors, surface only those it can't fix).
func runLintAtPath(bakePath, configPath string, applyFix, jsonOut bool) (int, error) {
	bakeDir := filepath.Dir(bakePath)
	if configPath == "" {
		configPath = lint.ConfigPath(bakeDir)
	}
	lintCfg, err := lint.LoadConfig(configPath)
	if err != nil {
		return 2, err
	}
	findings, ast, err := lint.RunFromPath(bakePath, lintCfg)
	if err != nil {
		return 2, err
	}
	if applyFix && len(findings) > 0 {
		fixableCount := 0
		for _, f := range findings {
			if f.Fixable {
				fixableCount++
			}
		}
		if fixableCount > 0 {
			lint.ApplyFixes(ast, findings)
			var buf bytes.Buffer
			if err := dsl.Format(ast, &buf); err != nil {
				return 2, err
			}
			if err := os.WriteFile(bakePath, buf.Bytes(), 0644); err != nil {
				return 2, err
			}
		}
		// After applying fixes, report only unfixable findings.
		var unfixable []lint.Finding
		for _, f := range findings {
			if !f.Fixable {
				unfixable = append(unfixable, f)
			}
		}
		findings = unfixable
	}
	if err := lint.PrintFindings(os.Stderr, findings, jsonOut); err != nil {
		return 2, err
	}
	if len(findings) > 0 {
		return 1, nil
	}
	return 0, nil
}

func runLintDisable(ruleID, configPath string) (int, error) {
	_, bakePath, err := config.FindBakefile(".")
	if err != nil {
		return 2, err
	}
	bakeDir := filepath.Dir(bakePath)
	if configPath == "" {
		configPath = lint.ConfigPath(bakeDir)
	}
	cfg, err := lint.LoadConfig(configPath)
	if err != nil {
		return 2, err
	}
	cfg.DisableRule(ruleID)
	if err := cfg.WriteConfig(configPath); err != nil {
		return 2, fmt.Errorf("write config: %w", err)
	}
	fmt.Fprintf(os.Stderr, "bake: disabled lint rule %q in %s\n", ruleID, configPath)
	return 0, nil
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
		// Filter to only targets in this suite (each entry is "target" or "target preset")
		names := make(map[string]bool)
		for _, entry := range su.Targets {
			targetName, _ := config.ParseSuiteEntry(entry)
			names[targetName] = true
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

func runDefault(cliEnv, cliArgs map[string]string, profileName string, showCmd, jsonMode bool, maxParallel int, timing, artifacts bool) error {
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
	return runTarget(cfg, name, tgt, declared, live, nil, cliEnv, profile, nil, showCmd, jsonMode, maxParallel, timing, artifacts)
}

// runUp runs the "up" target workflow (targets + daemons). If workflow has a schedule, re-runs workflow targets until ctx is done (or SIGINT when ctx is nil).
func runUp(ctx context.Context, cliEnv map[string]string, profileName string, showCmd bool) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	tgt := cfg.TargetByName("up")
	if tgt == nil {
		return fmt.Errorf("no target \"up\" defined (add 'target up { workflow { ... }; daemon name { ... } }' for 'bake up')")
	}
	if len(tgt.Workflow) == 0 {
		return fmt.Errorf("target \"up\" has no workflow (add workflow { target1 daemon1 ... } inside target up)")
	}
	rootDir := cfg.RootDir
	dotenvMap, err := env.LoadDotenv(rootDir, cfg.Dotenv)
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
	opts := runner.RunOptions{
		RootDir:     rootDir,
		Dotenv:      cfg.Dotenv,
		CLIEnv:      cliEnv,
		ShowCmd:     showCmd,
		MaxParallel: 1,
	}
	if profile != nil {
		opts.TargetEnv = profile.Env
	}
	daemonByName := func(name string) *config.Daemon {
		for _, d := range tgt.Daemons {
			if d != nil && d.Name == name {
				return d
			}
		}
		return nil
	}
	for _, name := range tgt.Workflow {
		if runTgt := cfg.TargetByName(name); runTgt != nil {
			opts.TargetEnv = runTgt.Env
			if profile != nil {
				opts.TargetEnv = mergeEnv(profile.Env, runTgt.Env)
			}
			if err := runner.Run(context.Background(), cfg, name, opts); err != nil {
				return fmt.Errorf("workflow target %q: %w", name, err)
			}
			continue
		}
		d := daemonByName(name)
		if d == nil {
			return fmt.Errorf("workflow references unknown target or daemon %q", name)
		}
		var envOverlay map[string]string
		if profile != nil {
			envOverlay = profile.Env
		}
		pid, containerID, err := lifecycle.StartDaemon(context.Background(), rootDir, d, dotenvMap, cliEnv, envOverlay, nil)
		if err != nil {
			return fmt.Errorf("start daemon %q: %w", d.Name, err)
		}
		if err := lifecycle.AddDaemon(rootDir, d.Name, pid, containerID); err != nil {
			return fmt.Errorf("save state: %w", err)
		}
		if containerID != "" {
			idShort := containerID
			if len(idShort) > 12 {
				idShort = idShort[:12]
			}
			fmt.Fprintf(os.Stderr, "bake: started daemon %q (container %s)\n", d.Name, idShort)
		} else {
			fmt.Fprintf(os.Stderr, "bake: started daemon %q (pid %d)\n", d.Name, pid)
		}
	}
	// Optional schedule: re-run workflow targets (not daemons) on cron/interval until ctx is done.
	if tgt.WorkflowSchedule == nil {
		return nil
	}
	sched := tgt.WorkflowSchedule
	var workflowTargets []string
	for _, name := range tgt.Workflow {
		if cfg.TargetByName(name) != nil {
			workflowTargets = append(workflowTargets, name)
		}
	}
	if len(workflowTargets) == 0 {
		return nil
	}
	if ctx == nil {
		var stop context.CancelFunc
		ctx, stop = signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()
	}
	if sched.Interval != "" {
		dur, err := time.ParseDuration(sched.Interval)
		if err != nil {
			return fmt.Errorf("workflow schedule interval %q: %w", sched.Interval, err)
		}
		ticker := time.NewTicker(dur)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return nil
			case <-ticker.C:
				for _, name := range workflowTargets {
					runTgt := cfg.TargetByName(name)
					opts.TargetEnv = runTgt.Env
					if profile != nil {
						opts.TargetEnv = mergeEnv(profile.Env, runTgt.Env)
					}
					if err := runner.Run(context.Background(), cfg, name, opts); err != nil {
						fmt.Fprintf(os.Stderr, "bake: schedule run %q: %v\n", name, err)
					}
				}
			}
		}
	}
	if sched.Cron != "" {
		cr := cron.New()
		_, err := cr.AddFunc(sched.Cron, func() {
			for _, name := range workflowTargets {
				runTgt := cfg.TargetByName(name)
				opts.TargetEnv = runTgt.Env
				if profile != nil {
					opts.TargetEnv = mergeEnv(profile.Env, runTgt.Env)
				}
				if err := runner.Run(context.Background(), cfg, name, opts); err != nil {
					fmt.Fprintf(os.Stderr, "bake: schedule run %q: %v\n", name, err)
				}
			}
		})
		if err != nil {
			return fmt.Errorf("workflow schedule cron %q: %w", sched.Cron, err)
		}
		cr.Start()
		defer cr.Stop()
		<-ctx.Done()
	}
	return nil
}

// stopDaemonEntry stops a daemon recorded in state (either host process or container).
func stopDaemonEntry(ctx context.Context, _ string, entry lifecycle.DaemonEntry) error {
	if entry.ContainerID != "" {
		return lifecycle.StopDaemonContainer(ctx, entry.ContainerID)
	}
	return lifecycle.StopDaemon(entry.PID)
}

func runDown(daemonName string) error {
	rootDir, _, err := config.FindBakefile(".")
	if err != nil {
		return err
	}
	state, err := lifecycle.Load(rootDir)
	if err != nil {
		return err
	}
	if daemonName != "" {
		entry, ok := state.Daemons[daemonName]
		if !ok {
			return fmt.Errorf("daemon %q not in state (not running?)", daemonName)
		}
		if err := stopDaemonEntry(context.Background(), daemonName, entry); err != nil {
			return fmt.Errorf("stop daemon %q: %w", daemonName, err)
		}
		if err := lifecycle.RemoveDaemon(rootDir, daemonName); err != nil {
			return err
		}
		if entry.ContainerID != "" {
			fmt.Fprintf(os.Stderr, "bake: stopped daemon %q (container)\n", daemonName)
		} else {
			fmt.Fprintf(os.Stderr, "bake: stopped daemon %q (pid %d)\n", daemonName, entry.PID)
		}
		return nil
	}
	for name, entry := range state.Daemons {
		if err := stopDaemonEntry(context.Background(), name, entry); err != nil {
			if entry.ContainerID != "" {
				fmt.Fprintf(os.Stderr, "bake: warning stopping %q (container): %v\n", name, err)
			} else {
				fmt.Fprintf(os.Stderr, "bake: warning stopping %q (pid %d): %v\n", name, entry.PID, err)
			}
		}
		if err := lifecycle.RemoveDaemon(rootDir, name); err != nil {
			return err
		}
		if entry.ContainerID != "" {
			fmt.Fprintf(os.Stderr, "bake: stopped daemon %q (container)\n", name)
		} else {
			fmt.Fprintf(os.Stderr, "bake: stopped daemon %q (pid %d)\n", name, entry.PID)
		}
	}
	if len(state.Daemons) == 0 {
		fmt.Fprintf(os.Stderr, "bake: no daemons in state\n")
	}
	return nil
}

func runTarget(cfg *config.File, name string, tgt *config.Target, declared, live map[string]string, passthroughByStep map[int][]string, cliEnv map[string]string, profile *config.Profile, preset *config.Preset, showCmd, jsonMode bool, maxParallel int, timing, artifacts bool) error {
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
		RootDir:           cfg.RootDir,
		Dotenv:            dotenv,
		TargetEnv:         targetEnv,
		CLIEnv:            cliEnv,
		DeclaredArgs:      declared,
		LiveArgs:          live,
		PassthroughByStep: passthroughByStep,
		Preset:            preset,
		ShowCmd:           showCmd,
		MaxParallel:       maxParallel,
	}
	if jsonMode {
		opts.EventWriter = os.Stdout
		opts.StepStdout = os.Stderr
		opts.StepStderr = os.Stderr
	}
	var timings []struct {
		Name     string
		Duration time.Duration
	}
	var artifactList []struct {
		Target string
		Paths  []string
	}
	if timing {
		opts.TimingReporter = func(n string, d time.Duration) {
			timings = append(timings, struct {
				Name     string
				Duration time.Duration
			}{n, d})
		}
	}
	if artifacts {
		opts.ArtifactReporter = func(n string, p []string) {
			artifactList = append(artifactList, struct {
				Target string
				Paths  []string
			}{n, p})
		}
	}
	err := runner.Run(context.Background(), cfg, name, opts)
	if err != nil {
		return err
	}
	if timing && len(timings) > 0 {
		for _, t := range timings {
			fmt.Fprintf(os.Stderr, "%s\t%v\n", t.Name, t.Duration.Round(time.Millisecond))
		}
	}
	if artifacts && len(artifactList) > 0 {
		for _, a := range artifactList {
			for _, p := range a.Paths {
				fmt.Fprintf(os.Stderr, "%s\t%s\n", a.Target, p)
			}
		}
	}
	return nil
}

// inputPathsForRun returns resolved input paths (relative to cfg.RootDir) for target and all its deps.
func inputPathsForRun(cfg *config.File, targetName string) ([]string, error) {
	order, err := runner.TopoOrder(cfg, targetName)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool)
	var out []string
	data := resolve.TemplateData(nil, nil)
	for _, name := range order {
		t := cfg.TargetByName(name)
		if t == nil || len(t.Inputs) == 0 {
			continue
		}
		expanded, err := resolve.ExpandArgv(t.Inputs, data)
		if err != nil {
			return nil, err
		}
		resolved, err := cache.ResolveGlobs(cfg.RootDir, expanded)
		if err != nil {
			return nil, err
		}
		for _, p := range resolved {
			if !seen[p] {
				seen[p] = true
				out = append(out, p)
			}
		}
	}
	return out, nil
}

func runWatch(ctx context.Context, target string, restArgs []string, cliEnv, cliArgs map[string]string, profileName string, showCmd bool, maxParallel int) error {
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
	tgt := cfg.TargetByName(target)
	if tgt == nil {
		return fmt.Errorf("unknown target %q (use 'bake --list')", target)
	}
	argsForTarget := restArgs
	var preset *config.Preset
	if len(restArgs) > 0 && !strings.HasPrefix(restArgs[0], "-") && tgt.PresetByName(restArgs[0]) != nil {
		preset = tgt.PresetByName(restArgs[0])
		argsForTarget = restArgs[1:]
	}
	declared, live, passthroughByStep, err := resolve.ParseArgs(tgt, argsForTarget)
	if err != nil {
		return err
	}
	for k, v := range cliArgs {
		declared[k] = v
	}
	paths, err := inputPathsForRun(cfg, target)
	if err != nil {
		return err
	}
	if len(paths) == 0 {
		// No inputs to watch; run once and exit (or watch nothing and run on timer is odd)
		return runTarget(cfg, target, tgt, declared, live, passthroughByStep, cliEnv, profile, preset, showCmd, false, 1, false, false)
	}
	doRun := func() error {
		return runTarget(cfg, target, tgt, declared, live, passthroughByStep, cliEnv, profile, preset, showCmd, false, 1, false, false)
	}
	if err := doRun(); err != nil {
		return err
	}
	watch.Poll(ctx, cfg.RootDir, paths, 300*time.Millisecond, 200*time.Millisecond, func() {
		if err := doRun(); err != nil {
			fmt.Fprintf(os.Stderr, "bake: %v\n", err)
			os.Exit(1)
		}
	})
	return nil
}

func runDryRun(cfg *config.File, name string, tgt *config.Target, declared, live map[string]string, passthroughByStep map[int][]string, cliEnv map[string]string, preset *config.Preset) {
	order, _ := runner.TopoOrder(cfg, name)
	for _, n := range order {
		fmt.Printf("target %s\n", n)
		t := cfg.TargetByName(n)
		if t == nil {
			continue
		}
		var stepPreset *config.Preset
		if n == name && preset != nil {
			stepPreset = preset
		}
		passthroughStep := t.PassthroughStep
		if len(t.Passthrough) > 0 {
			passthroughStep = t.Passthrough[0].Step
		}
		if passthroughStep <= 0 && len(t.Steps) > 0 {
			passthroughStep = len(t.Steps)
		}
		data := resolve.TemplateData(declared, live)
		for i, step := range t.Steps {
			argv := step.Argv
			if len(data) > 0 {
				argv, _ = resolve.ExpandArgv(argv, data)
			}
			stepNum := i + 1
			if stepNum == passthroughStep {
				if stepPreset != nil && len(stepPreset.Argv) > 0 {
					argv = append(argv, stepPreset.Argv...)
				}
				if passthroughByStep != nil && len(passthroughByStep[stepNum]) > 0 {
					argv = append(argv, passthroughByStep[stepNum]...)
				}
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
		for _, ref := range su.Targets {
			targetName, _ := config.ParseSuiteEntry(ref)
			names[targetName] = true
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
	return runTarget(cfg, chosen.Name, chosen, declared, live, nil, cliEnv, profile, nil, showCmd, false, 1, false, false) // maxParallel 1 for choose
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
	return runFormatAtPath(path, write, check)
}

// runFormatAtPath formats the Bakefile at path. Used by runFormat and ensureBakefileFormatLint.
func runFormatAtPath(path string, write, check bool) (int, error) {
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

// runInstall installs all components: ensures a minimal Bakefile exists, then installs shims and hooks.
func runInstall() error {
	if err := ensureMinimalBakefile(); err != nil {
		return err
	}
	if err := runInstallShims(); err != nil {
		return err
	}
	if err := runInstallHooks(); err != nil {
		// Not a git repo or other hooks error: warn but don't fail
		fmt.Fprintf(os.Stderr, "bake: %v\n", err)
	}
	return nil
}

func ensureMinimalBakefile() error {
	_, _, err := config.FindBakefile(".")
	if err == nil {
		return nil
	}
	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getwd: %w", err)
	}
	// Prefer repo root if we're in a git repo
	if root := findGitRoot(dir); root != "" {
		dir = root
	}
	bakePath := filepath.Join(dir, "Bakefile")
	// Minimal Bakefile: one target, one suite
	const minimal = `target build { desc "build" steps { exec ["true"] } }
suite dev { build }
`
	if err := os.WriteFile(bakePath, []byte(minimal), 0644); err != nil {
		return fmt.Errorf("create Bakefile: %w", err)
	}
	fmt.Fprintf(os.Stderr, "bake: created minimal Bakefile at %s\n", bakePath)
	return nil
}

func findGitRoot(dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return ""
	}
	d := abs
	for {
		if st, err := os.Stat(filepath.Join(d, ".git")); err == nil && st.IsDir() {
			return d
		}
		parent := filepath.Dir(d)
		if parent == d {
			return ""
		}
		d = parent
	}
}

func runInstallShims() error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	bakeExe := "bake"
	if exe, err := os.Executable(); err == nil {
		bakeExe = exe
	}
	if err := shim.WriteShims(cfg.RootDir, cfg, bakeExe); err != nil {
		return err
	}
	binDir := filepath.Join(cfg.RootDir, ".bake", "bin")
	fmt.Fprintf(os.Stderr, "bake: installed shims in %s (add to PATH: export PATH=\"%s:$PATH\")\n", binDir, binDir)
	return nil
}

func runInstallHooks() error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	if cfg.SuiteByName("precommit") == nil {
		return fmt.Errorf("no suite precommit defined; add 'suite precommit { ... }' to your Bakefile to install hooks")
	}
	gitDir := filepath.Join(cfg.RootDir, ".git")
	if st, err := os.Stat(gitDir); err != nil || !st.IsDir() {
		return fmt.Errorf("not a git repo (no .git in %s)", cfg.RootDir)
	}
	rootAbs, err := filepath.Abs(cfg.RootDir)
	if err != nil {
		return fmt.Errorf("resolve root dir: %w", err)
	}
	hooksDir := filepath.Join(gitDir, "hooks")
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		return fmt.Errorf("create hooks dir: %w", err)
	}
	bakeExe, err := os.Executable()
	if err != nil {
		bakeExe = "bake"
	}
	script := fmt.Sprintf("#!/bin/sh\nset -e\ncd %s\n%q precommit\n", strconv.Quote(rootAbs), bakeExe)
	hookPath := filepath.Join(hooksDir, "pre-commit")
	if err := os.WriteFile(hookPath, []byte(script), 0755); err != nil {
		return fmt.Errorf("write pre-commit hook: %w", err)
	}
	fmt.Fprintf(os.Stderr, "bake: installed pre-commit hook at %s (runs: bake precommit)\n", hookPath)
	return nil
}

func runInstallDaemon() error {
	rootDir, _, err := config.FindBakefile(".")
	if err != nil {
		return fmt.Errorf("find Bakefile: %w", err)
	}
	rootAbs, err := filepath.Abs(rootDir)
	if err != nil {
		return fmt.Errorf("resolve workspace root: %w", err)
	}
	bakedExe, err := exec.LookPath("baked")
	if err != nil {
		// Prefer same dir as bake
		bakeExe, _ := os.Executable()
		if bakeExe != "" {
			bakedExe = filepath.Join(filepath.Dir(bakeExe), "baked")
			if _, err := os.Stat(bakedExe); err != nil {
				bakedExe = ""
			}
		}
		if bakedExe == "" {
			return fmt.Errorf("baked not found in PATH and not next to bake binary; install baked first")
		}
	}
	id := shortID(rootAbs)
	switch runtime.GOOS {
	case "linux":
		return installDaemonSystemd(rootAbs, bakedExe, id)
	case "darwin":
		return installDaemonLaunchd(rootAbs, bakedExe, id)
	default:
		return fmt.Errorf("install daemon is supported only on Linux (systemd) and macOS (launchd), not %s", runtime.GOOS)
	}
}

func shortID(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])[:8]
}

func installDaemonSystemd(rootAbs, bakedExe, id string) error {
	configDir := filepath.Join(os.Getenv("HOME"), ".config", "systemd", "user")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("create systemd user dir: %w", err)
	}
	name := "baked-" + id + ".service"
	path := filepath.Join(configDir, name)
	unit := fmt.Sprintf(`[Unit]
Description=Baked daemon for %s
After=network.target

[Service]
Type=simple
WorkingDirectory=%s
ExecStart=%s
Restart=on-failure
RestartSec=2

[Install]
WantedBy=default.target
`, rootAbs, rootAbs, bakedExe)
	if err := os.WriteFile(path, []byte(unit), 0644); err != nil {
		return fmt.Errorf("write systemd unit: %w", err)
	}
	fmt.Fprintf(os.Stderr, "bake: wrote %s\n", path)
	// Reload and enable so it starts on login; start now
	cmd := exec.Command("systemctl", "--user", "daemon-reload")
	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "bake: systemctl daemon-reload: %v\n%s", err, out)
	} else {
		cmd = exec.Command("systemctl", "--user", "enable", name)
		if out, err := cmd.CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "bake: systemctl enable: %v\n%s", err, out)
		} else {
			cmd = exec.Command("systemctl", "--user", "start", name)
			if out, err := cmd.CombinedOutput(); err != nil {
				fmt.Fprintf(os.Stderr, "bake: systemctl start: %v\n%s", err, out)
			} else {
				fmt.Fprintf(os.Stderr, "bake: enabled and started %s\n", name)
			}
		}
	}
	fmt.Fprintf(os.Stderr, "bake: to stop: systemctl --user stop %s\n", name)
	return nil
}

func installDaemonLaunchd(rootAbs, bakedExe, id string) error {
	agentsDir := filepath.Join(os.Getenv("HOME"), "Library", "LaunchAgents")
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		return fmt.Errorf("create LaunchAgents dir: %w", err)
	}
	label := "com.bake.baked." + id
	path := filepath.Join(agentsDir, label+".plist")
	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>%s</string>
  <key>ProgramArguments</key>
  <array>
    <string>%s</string>
  </array>
  <key>WorkingDirectory</key>
  <string>%s</string>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <true/>
</dict>
</plist>
`, label, bakedExe, rootAbs)
	if err := os.WriteFile(path, []byte(plist), 0644); err != nil {
		return fmt.Errorf("write launchd plist: %w", err)
	}
	fmt.Fprintf(os.Stderr, "bake: wrote %s\n", path)
	cmd := exec.Command("launchctl", "load", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "bake: launchctl load: %v\n%s", err, out)
	} else {
		fmt.Fprintf(os.Stderr, "bake: loaded and started %s\n", label)
	}
	fmt.Fprintf(os.Stderr, "bake: to stop: launchctl unload %s\n", path)
	return nil
}
