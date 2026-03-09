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

func TestLevelOrder(t *testing.T) {
	cfg := &config.File{
		Targets: []*config.Target{
			{Name: "a", Deps: []string{"b", "c"}},
			{Name: "b", Deps: []string{"c"}},
			{Name: "c", Deps: nil},
		},
	}
	levels, err := LevelOrder(cfg, "a")
	if err != nil {
		t.Fatal(err)
	}
	// Level 0: c (no deps in set). Level 1: b (dep c). Level 2: a (deps b,c).
	if len(levels) != 3 {
		t.Fatalf("expected 3 levels, got %d", len(levels))
	}
	if len(levels[0]) != 1 || levels[0][0] != "c" {
		t.Errorf("level 0: expected [c], got %v", levels[0])
	}
	if len(levels[1]) != 1 || levels[1][0] != "b" {
		t.Errorf("level 1: expected [b], got %v", levels[1])
	}
	if len(levels[2]) != 1 || levels[2][0] != "a" {
		t.Errorf("level 2: expected [a], got %v", levels[2])
	}
}

func TestLevelOrderParallel(t *testing.T) {
	// a depends on b and c (same level); b and c have no deps in set
	cfg := &config.File{
		Targets: []*config.Target{
			{Name: "a", Deps: []string{"b", "c"}},
			{Name: "b", Deps: nil},
			{Name: "c", Deps: nil},
		},
	}
	levels, err := LevelOrder(cfg, "a")
	if err != nil {
		t.Fatal(err)
	}
	if len(levels) != 2 {
		t.Fatalf("expected 2 levels, got %d", len(levels))
	}
	// Level 0: b and c (both no deps in {a,b,c})
	if len(levels[0]) != 2 {
		t.Errorf("level 0: expected 2 targets, got %v", levels[0])
	}
	// Level 1: a
	if len(levels[1]) != 1 || levels[1][0] != "a" {
		t.Errorf("level 1: expected [a], got %v", levels[1])
	}
}
