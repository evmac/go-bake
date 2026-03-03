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
