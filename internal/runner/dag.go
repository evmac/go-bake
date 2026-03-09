package runner

import (
	"fmt"

	"github.com/evmac/go-bake/internal/config"
)

// TopoOrder returns target names in dependency order (deps first, then the target).
func TopoOrder(cfg *config.File, targetName string) ([]string, error) {
	visited := make(map[string]bool)
	var order []string
	var visit func(name string) error
	visit = func(name string) error {
		if visited[name] {
			return nil
		}
		visited[name] = true
		tgt := cfg.TargetByName(name)
		if tgt == nil {
			return fmt.Errorf("unknown dependency %q", name)
		}
		for _, d := range tgt.Deps {
			if err := visit(d); err != nil {
				return err
			}
		}
		order = append(order, name)
		return nil
	}
	if err := visit(targetName); err != nil {
		return nil, err
	}
	return order, nil
}

// AllEdges returns the full dependency graph: map from target name to its direct deps.
func AllEdges(cfg *config.File) map[string][]string {
	out := make(map[string][]string)
	for _, t := range cfg.Targets {
		out[t.Name] = append([]string(nil), t.Deps...)
	}
	return out
}

// LevelOrder returns the same targets as TopoOrder but grouped by level: level 0 = no deps in set, level k = deps only in levels < k.
// Used for parallel execution: targets in the same level can run concurrently.
func LevelOrder(cfg *config.File, targetName string) ([][]string, error) {
	order, err := TopoOrder(cfg, targetName)
	if err != nil {
		return nil, err
	}
	set := make(map[string]bool)
	for _, n := range order {
		set[n] = true
	}
	edges := AllEdges(cfg)
	levelOf := make(map[string]int)
	for _, n := range order {
		maxDepLevel := -1
		for _, d := range edges[n] {
			if set[d] && levelOf[d] > maxDepLevel {
				maxDepLevel = levelOf[d]
			}
		}
		levelOf[n] = maxDepLevel + 1
	}
	maxLevel := 0
	for _, l := range levelOf {
		if l > maxLevel {
			maxLevel = l
		}
	}
	levels := make([][]string, maxLevel+1)
	for _, n := range order {
		l := levelOf[n]
		levels[l] = append(levels[l], n)
	}
	return levels, nil
}
