package baked

import (
	"context"
	"fmt"
	"os"

	"github.com/evmac/go-bake/internal/config"
	"github.com/evmac/go-bake/internal/env"
	"github.com/evmac/go-bake/internal/lifecycle"
	"github.com/evmac/go-bake/internal/runner"
)

// RunTarget runs a single target. Uses cfg.RootDir, no profile, empty args.
func RunTarget(ctx context.Context, cfg *config.File, targetName string) error {
	tgt := cfg.TargetByName(targetName)
	if tgt == nil {
		return fmt.Errorf("unknown target %q", targetName)
	}
	opts := runner.RunOptions{
		RootDir:     cfg.RootDir,
		Dotenv:      cfg.Dotenv,
		TargetEnv:   tgt.Env,
		CLIEnv:      nil,
		MaxParallel: 1,
	}
	return runner.Run(ctx, cfg, targetName, opts)
}

// RunSuite runs all targets in the suite in order.
func RunSuite(ctx context.Context, cfg *config.File, suiteName string) error {
	su := cfg.SuiteByName(suiteName)
	if su == nil {
		return fmt.Errorf("unknown suite %q", suiteName)
	}
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
		opts := runner.RunOptions{
			RootDir:     cfg.RootDir,
			Dotenv:      cfg.Dotenv,
			TargetEnv:   tgt.Env,
			CLIEnv:      nil,
			Preset:      preset,
			MaxParallel: 1,
		}
		if err := runner.Run(ctx, cfg, targetName, opts); err != nil {
			return fmt.Errorf("%s: %w", entry, err)
		}
	}
	return nil
}

// RunUp runs the "up" target workflow once (targets + daemons). No schedule.
func RunUp(ctx context.Context, cfg *config.File) error {
	tgt := cfg.TargetByName("up")
	if tgt == nil {
		return fmt.Errorf("no target \"up\" defined")
	}
	if len(tgt.Workflow) == 0 {
		return fmt.Errorf("target \"up\" has no workflow")
	}
	rootDir := cfg.RootDir
	dotenvMap, err := env.LoadDotenv(rootDir, cfg.Dotenv)
	if err != nil {
		return err
	}
	opts := runner.RunOptions{
		RootDir:     rootDir,
		Dotenv:      cfg.Dotenv,
		CLIEnv:      nil,
		MaxParallel: 1,
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
			if err := runner.Run(ctx, cfg, name, opts); err != nil {
				return fmt.Errorf("workflow target %q: %w", name, err)
			}
			continue
		}
		d := daemonByName(name)
		if d == nil {
			return fmt.Errorf("workflow references unknown target or daemon %q", name)
		}
		pid, containerID, err := lifecycle.StartDaemon(ctx, rootDir, d, dotenvMap, nil, nil, nil)
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
	return nil
}

// RunDown stops daemon(s) and updates state.
func RunDown(rootDir, daemonName string) error {
	state, err := lifecycle.Load(rootDir)
	if err != nil {
		return err
	}
	stopEntry := func(name string, entry lifecycle.DaemonEntry) error {
		if entry.ContainerID != "" {
			return lifecycle.StopDaemonContainer(context.Background(), entry.ContainerID)
		}
		return lifecycle.StopDaemon(entry.PID)
	}
	if daemonName != "" {
		entry, ok := state.Daemons[daemonName]
		if !ok {
			return fmt.Errorf("daemon %q not in state (not running?)", daemonName)
		}
		if err := stopEntry(daemonName, entry); err != nil {
			return fmt.Errorf("stop daemon %q: %w", daemonName, err)
		}
		if err := lifecycle.RemoveDaemon(rootDir, daemonName); err != nil {
			return err
		}
		return nil
	}
	for name, entry := range state.Daemons {
		_ = stopEntry(name, entry)
		_ = lifecycle.RemoveDaemon(rootDir, name)
	}
	return nil
}
