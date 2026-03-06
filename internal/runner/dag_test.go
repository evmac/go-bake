package runner

import (
	"testing"

	"github.com/evmac/go-bake/internal/config"
)

func TestTopoOrder(t *testing.T) {
	cfg := &config.File{
		Targets: []*config.Target{
			{Name: "a", Deps: []string{"b", "c"}},
			{Name: "b", Deps: []string{"c"}},
			{Name: "c", Deps: nil},
		},
	}
	order, err := TopoOrder(cfg, "a")
	if err != nil {
		t.Fatal(err)
	}
	// c before b before a
	cIdx, bIdx, aIdx := -1, -1, -1
	for i, n := range order {
		switch n {
		case "a":
			aIdx = i
		case "b":
			bIdx = i
		case "c":
			cIdx = i
		}
	}
	if cIdx >= bIdx || bIdx >= aIdx {
		t.Errorf("order should be c, b, a; got %v", order)
	}
}

func TestTopoOrderUnknownDep(t *testing.T) {
	cfg := &config.File{
		Targets: []*config.Target{
			{Name: "a", Deps: []string{"missing"}},
		},
	}
	_, err := TopoOrder(cfg, "a")
	if err == nil {
		t.Fatal("expected error for unknown dep")
	}
}

func TestAllEdges(t *testing.T) {
	cfg := &config.File{
		Targets: []*config.Target{
			{Name: "a", Deps: []string{"b", "c"}},
			{Name: "b", Deps: []string{"c"}},
			{Name: "c", Deps: nil},
		},
	}
	edges := AllEdges(cfg)
	if len(edges) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(edges))
	}
	if len(edges["a"]) != 2 || len(edges["b"]) != 1 || len(edges["c"]) != 0 {
		t.Errorf("edges: got %v", edges)
	}
}
