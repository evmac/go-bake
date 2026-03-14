package baked

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/evmac/go-bake/internal/config"
	"github.com/evmac/go-bake/internal/lifecycle"
)

func TestDaemonDiffBothNil(t *testing.T) {
	added, removed, changed := DaemonDiff(nil, nil)
	if len(added)+len(removed)+len(changed) != 0 {
		t.Errorf("expected no diff for nil configs")
	}
}

func TestDaemonDiffNoUpTarget(t *testing.T) {
	cfg := &config.File{
		Targets: []*config.Target{{Name: "build"}},
	}
	added, removed, changed := DaemonDiff(cfg, cfg)
	if len(added)+len(removed)+len(changed) != 0 {
		t.Errorf("expected no diff when no up target")
	}
}

func TestDaemonDiffAddedDaemon(t *testing.T) {
	oldCfg := &config.File{
		Targets: []*config.Target{{
			Name:    "up",
			Daemons: []*config.Daemon{{Name: "db", Steps: []config.Step{{Argv: []string{"postgres"}}}}},
		}},
	}
	newCfg := &config.File{
		Targets: []*config.Target{{
			Name: "up",
			Daemons: []*config.Daemon{
				{Name: "db", Steps: []config.Step{{Argv: []string{"postgres"}}}},
				{Name: "redis", Steps: []config.Step{{Argv: []string{"redis-server"}}}},
			},
		}},
	}
	added, removed, changed := DaemonDiff(oldCfg, newCfg)
	if len(added) != 1 || added[0].Name != "redis" {
		t.Errorf("expected 1 added (redis), got %v", added)
	}
	if len(removed) != 0 {
		t.Errorf("expected 0 removed, got %d", len(removed))
	}
	if len(changed) != 0 {
		t.Errorf("expected 0 changed, got %d", len(changed))
	}
}

func TestDaemonDiffRemovedDaemon(t *testing.T) {
	oldCfg := &config.File{
		Targets: []*config.Target{{
			Name: "up",
			Daemons: []*config.Daemon{
				{Name: "db", Steps: []config.Step{{Argv: []string{"postgres"}}}},
				{Name: "redis", Steps: []config.Step{{Argv: []string{"redis-server"}}}},
			},
		}},
	}
	newCfg := &config.File{
		Targets: []*config.Target{{
			Name:    "up",
			Daemons: []*config.Daemon{{Name: "db", Steps: []config.Step{{Argv: []string{"postgres"}}}}},
		}},
	}
	added, removed, _ := DaemonDiff(oldCfg, newCfg)
	if len(removed) != 1 || removed[0].Name != "redis" {
		t.Errorf("expected 1 removed (redis), got %v", removed)
	}
	if len(added) != 0 {
		t.Errorf("expected 0 added, got %d", len(added))
	}
}

func TestDaemonDiffChangedDaemon(t *testing.T) {
	oldCfg := &config.File{
		Targets: []*config.Target{{
			Name:    "up",
			Daemons: []*config.Daemon{{Name: "db", Steps: []config.Step{{Argv: []string{"postgres"}}}}},
		}},
	}
	newCfg := &config.File{
		Targets: []*config.Target{{
			Name:    "up",
			Daemons: []*config.Daemon{{Name: "db", Steps: []config.Step{{Argv: []string{"mysql"}}}}},
		}},
	}
	added, removed, changed := DaemonDiff(oldCfg, newCfg)
	if len(changed) != 1 || changed[0].Name != "db" {
		t.Errorf("expected 1 changed (db), got %v", changed)
	}
	if len(added) != 0 || len(removed) != 0 {
		t.Errorf("expected 0 added/removed")
	}
}

func TestDaemonDiffImageChange(t *testing.T) {
	oldCfg := &config.File{
		Targets: []*config.Target{{
			Name:    "up",
			Daemons: []*config.Daemon{{Name: "db", Image: "postgres:14", Steps: []config.Step{{Argv: []string{"postgres"}}}}},
		}},
	}
	newCfg := &config.File{
		Targets: []*config.Target{{
			Name:    "up",
			Daemons: []*config.Daemon{{Name: "db", Image: "postgres:15", Steps: []config.Step{{Argv: []string{"postgres"}}}}},
		}},
	}
	_, _, changed := DaemonDiff(oldCfg, newCfg)
	if len(changed) != 1 {
		t.Errorf("expected 1 changed for image update, got %d", len(changed))
	}
}

func TestDaemonDiffStepCountChange(t *testing.T) {
	oldCfg := &config.File{
		Targets: []*config.Target{{
			Name:    "up",
			Daemons: []*config.Daemon{{Name: "db", Steps: []config.Step{{Argv: []string{"postgres"}}}}},
		}},
	}
	newCfg := &config.File{
		Targets: []*config.Target{{
			Name:    "up",
			Daemons: []*config.Daemon{{Name: "db", Steps: []config.Step{{Argv: []string{"postgres"}}, {Argv: []string{"init"}}}}},
		}},
	}
	_, _, changed := DaemonDiff(oldCfg, newCfg)
	if len(changed) != 1 {
		t.Errorf("expected 1 changed for step count change, got %d", len(changed))
	}
}

func TestDaemonDiffNoDiff(t *testing.T) {
	cfg := &config.File{
		Targets: []*config.Target{{
			Name:    "up",
			Daemons: []*config.Daemon{{Name: "db", Image: "pg:14", Steps: []config.Step{{Argv: []string{"postgres"}}}}},
		}},
	}
	added, removed, changed := DaemonDiff(cfg, cfg)
	if len(added)+len(removed)+len(changed) != 0 {
		t.Errorf("expected no diff for identical configs, got added=%d removed=%d changed=%d", len(added), len(removed), len(changed))
	}
}

func TestDaemonEqualArgvDifference(t *testing.T) {
	a := &config.Daemon{Name: "db", Steps: []config.Step{{Argv: []string{"pg", "--flag"}}}}
	b := &config.Daemon{Name: "db", Steps: []config.Step{{Argv: []string{"pg", "--other"}}}}
	if daemonEqual(a, b) {
		t.Error("expected not equal for different argv")
	}
}

func TestStopAndRemoveNoState(t *testing.T) {
	dir := t.TempDir()
	stopAndRemove(dir, "nonexistent")
}

func TestApplyDaemonDiffNoOp(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.File{RootDir: dir}
	ApplyDaemonDiff(nil, dir, cfg, nil, nil, nil)
}

func TestApplyDaemonDiffRemoved(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".bake"), 0755)
	cfg := &config.File{RootDir: dir}
	removed := []*config.Daemon{{Name: "old-daemon"}}
	ApplyDaemonDiff(context.Background(), dir, cfg, nil, removed, nil)
}

func TestApplyDaemonDiffAdded(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".bake"), 0755)
	cfg := &config.File{RootDir: dir}
	added := []*config.Daemon{{Name: "new-daemon", Steps: []config.Step{{Argv: []string{"sleep", "0.1"}}}}}
	ApplyDaemonDiff(context.Background(), dir, cfg, added, nil, nil)
}

func TestApplyDaemonDiffChanged(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".bake"), 0755)
	cfg := &config.File{RootDir: dir}
	changed := []*config.Daemon{{Name: "svc", Steps: []config.Step{{Argv: []string{"sleep", "0.1"}}}}}
	ApplyDaemonDiff(context.Background(), dir, cfg, nil, nil, changed)
}

func TestStopAndRemoveWithState(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".bake"), 0755)
	lifecycle.AddDaemon(dir, "test-svc", 99999999, "")
	stopAndRemove(dir, "test-svc")
	state, _ := lifecycle.Load(dir)
	if _, ok := state.Daemons["test-svc"]; ok {
		t.Error("expected daemon to be removed from state")
	}
}

func TestStartAndRecordHostDaemon(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".bake"), 0755)
	cfg := &config.File{RootDir: dir}
	d := &config.Daemon{Name: "sleeper", Steps: []config.Step{{Argv: []string{"sleep", "0.1"}}}}
	startAndRecord(context.Background(), dir, cfg, d)
	state, _ := lifecycle.Load(dir)
	if _, ok := state.Daemons["sleeper"]; !ok {
		t.Error("expected daemon to be recorded in state")
	}
}

func TestStartAndRecordNoSteps(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".bake"), 0755)
	cfg := &config.File{RootDir: dir}
	d := &config.Daemon{Name: "empty"}
	startAndRecord(context.Background(), dir, cfg, d)
}
