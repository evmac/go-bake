package runner

import (
	"context"
	"testing"

	"github.com/evmac/go-bake/internal/config"
)

func BenchmarkTopoOrder(b *testing.B) {
	cfg := &config.File{
		Targets: []*config.Target{
			{Name: "a", Deps: []string{"b", "c"}},
			{Name: "b", Deps: []string{"c", "d"}},
			{Name: "c", Deps: []string{"d"}},
			{Name: "d", Deps: nil},
		},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = TopoOrder(cfg, "a")
	}
}

func BenchmarkRunNoOp(b *testing.B) {
	cfg := &config.File{
		RootDir: b.TempDir(),
		Targets: []*config.Target{
			{Name: "noop", Steps: []config.Step{{Argv: []string{"true"}}}},
		},
	}
	opts := RunOptions{RootDir: cfg.RootDir}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Run(context.Background(), cfg, "noop", opts)
	}
}
