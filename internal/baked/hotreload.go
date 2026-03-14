package baked

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/evmac/go-bake/internal/config"
	"github.com/evmac/go-bake/internal/env"
	"github.com/evmac/go-bake/internal/lifecycle"
)

// DaemonDiff computes which daemons were added, removed, or changed between
// oldCfg and newCfg by comparing the "up" target's daemon blocks.
func DaemonDiff(oldCfg, newCfg *config.File) (added, removed, changed []*config.Daemon) {
	oldDaemons := upDaemons(oldCfg)
	newDaemons := upDaemons(newCfg)

	oldMap := make(map[string]*config.Daemon, len(oldDaemons))
	for _, d := range oldDaemons {
		oldMap[d.Name] = d
	}
	newMap := make(map[string]*config.Daemon, len(newDaemons))
	for _, d := range newDaemons {
		newMap[d.Name] = d
	}

	for _, d := range newDaemons {
		if _, ok := oldMap[d.Name]; !ok {
			added = append(added, d)
		} else if !daemonEqual(oldMap[d.Name], d) {
			changed = append(changed, d)
		}
	}
	for _, d := range oldDaemons {
		if _, ok := newMap[d.Name]; !ok {
			removed = append(removed, d)
		}
	}
	return
}

func upDaemons(cfg *config.File) []*config.Daemon {
	if cfg == nil {
		return nil
	}
	tgt := cfg.TargetByName("up")
	if tgt == nil {
		return nil
	}
	return tgt.Daemons
}

func daemonEqual(a, b *config.Daemon) bool {
	if a.Image != b.Image {
		return false
	}
	if len(a.Steps) != len(b.Steps) {
		return false
	}
	for i := range a.Steps {
		if len(a.Steps[i].Argv) != len(b.Steps[i].Argv) {
			return false
		}
		for j := range a.Steps[i].Argv {
			if a.Steps[i].Argv[j] != b.Steps[i].Argv[j] {
				return false
			}
		}
	}
	return true
}

// ApplyDaemonDiff stops removed/changed daemons and starts added/changed daemons.
func ApplyDaemonDiff(ctx context.Context, rootDir string, cfg *config.File, added, removed, changed []*config.Daemon) {
	for _, d := range removed {
		log.Printf("hot-reload: stopping removed daemon %q", d.Name)
		stopAndRemove(rootDir, d.Name)
	}
	for _, d := range changed {
		log.Printf("hot-reload: restarting changed daemon %q", d.Name)
		stopAndRemove(rootDir, d.Name)
		startAndRecord(ctx, rootDir, cfg, d)
	}
	for _, d := range added {
		log.Printf("hot-reload: starting new daemon %q", d.Name)
		startAndRecord(ctx, rootDir, cfg, d)
	}
}

func stopAndRemove(rootDir, name string) {
	state, err := lifecycle.Load(rootDir)
	if err != nil {
		return
	}
	entry, ok := state.Daemons[name]
	if !ok {
		return
	}
	if entry.ContainerID != "" {
		_ = lifecycle.StopDaemonContainer(context.Background(), entry.ContainerID)
	} else if entry.PID > 0 {
		_ = lifecycle.StopDaemon(entry.PID)
	}
	_ = lifecycle.RemoveDaemon(rootDir, name)
}

func startAndRecord(ctx context.Context, rootDir string, cfg *config.File, d *config.Daemon) {
	dotenvMap, _ := env.LoadDotenv(rootDir, cfg.Dotenv)
	pid, containerID, err := lifecycle.StartDaemon(ctx, rootDir, d, dotenvMap, nil, nil, nil)
	if err != nil {
		log.Printf("hot-reload: failed to start daemon %q: %v", d.Name, err)
		return
	}
	if err := lifecycle.AddDaemon(rootDir, d.Name, pid, containerID); err != nil {
		log.Printf("hot-reload: failed to record daemon %q: %v", d.Name, err)
		return
	}
	if containerID != "" {
		idShort := containerID
		if len(idShort) > 12 {
			idShort = idShort[:12]
		}
		fmt.Fprintf(os.Stderr, "bake: hot-reload started daemon %q (container %s)\n", d.Name, idShort)
	} else {
		fmt.Fprintf(os.Stderr, "bake: hot-reload started daemon %q (pid %d)\n", d.Name, pid)
	}
}
