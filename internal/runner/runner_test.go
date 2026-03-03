package runner

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/em/go-bake/internal/config"
)

func TestRunEcho(t *testing.T) {
	cfg := &config.File{
		RootDir: t.TempDir(),
		Targets: []*config.Target{
			{
				Name: "echo",
				Steps: []config.Step{
					{Argv: []string{"echo", "hello"}},
				},
			},
		},
	}
	err := Run(context.Background(), cfg, "echo", RunOptions{RootDir: cfg.RootDir})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRunWithDeps(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.File{
		RootDir: dir,
		Targets: []*config.Target{
			{Name: "first", Steps: []config.Step{{Argv: []string{"touch", filepath.Join(dir, "first.done")}}}},
			{Name: "second", Deps: []string{"first"}, Steps: []config.Step{{Argv: []string{"test", "-f", filepath.Join(dir, "first.done")}}}},
		},
	}
	err := Run(context.Background(), cfg, "second", RunOptions{RootDir: dir})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRunPassthrough(t *testing.T) {
	cfg := &config.File{
		RootDir: t.TempDir(),
		Targets: []*config.Target{
			{
				Name: "pt",
				Steps: []config.Step{
					{Argv: []string{"sh", "-c", "echo $1"}},
				},
				PassthroughStep: 1,
			},
		},
	}
	// Passthrough args go to step 1 (first step)
	err := Run(context.Background(), cfg, "pt", RunOptions{
		RootDir:    cfg.RootDir,
		Passthrough: []string{"hi"},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRunWithEnv(t *testing.T) {
	cfg := &config.File{
		RootDir: t.TempDir(),
		Targets: []*config.Target{
			{
				Name: "env",
				Env:  map[string]string{"BAKE_TEST_RUN": "1"},
				Steps: []config.Step{
					{Argv: []string{"sh", "-c", "test \"$BAKE_TEST_RUN\" = 1"}},
				},
			},
		},
	}
	err := Run(context.Background(), cfg, "env", RunOptions{RootDir: cfg.RootDir})
	if err != nil {
		t.Fatal(err)
	}
}
