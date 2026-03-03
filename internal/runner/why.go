package runner

import (
	"fmt"
	"sort"

	"github.com/evmac/go-bake/internal/cache"
	"github.com/evmac/go-bake/internal/config"
	"github.com/evmac/go-bake/internal/resolve"
)

// WhyReasons returns human-readable reasons why the target would run or be skipped (incremental build).
// If the target has no inputs/outputs, returns a single line. Otherwise reports cache state and changed inputs.
func WhyReasons(rootDir string, tgt *config.Target, opts RunOptions, dotenvMap map[string]string) ([]string, error) {
	if len(tgt.Inputs) == 0 || len(tgt.Outputs) == 0 {
		return []string{"target has no inputs/outputs (always runs)"}, nil
	}
	data := resolve.TemplateData(opts.DeclaredArgs, opts.LiveArgs)
	expandedInputs, _ := resolve.ExpandArgv(append([]string{}, tgt.Inputs...), data)
	inputFiles, err := cache.ResolveGlobs(rootDir, expandedInputs)
	if err != nil {
		return nil, err
	}
	inputHash, inputHashesMap, err := cache.HashFilesMap(rootDir, inputFiles)
	if err != nil {
		return nil, err
	}
	stepSig := stepSignature(tgt, rootDir, dotenvMap, opts)
	key := cache.Key(tgt.Name, inputHash, stepSig)
	manifest, err := cache.LoadManifest(rootDir, key)
	if err != nil {
		return nil, err
	}
	var reasons []string
	inputHashHex := fmt.Sprintf("%x", inputHash)
	if manifest == nil {
		prev, _ := cache.LoadManifestForTarget(rootDir, tgt.Name)
		if prev != nil && prev.InputHashes != nil {
			changed := changedInputs(prev.InputHashes, inputHashesMap)
			if len(changed) > 0 {
				reasons := []string{"inputs changed:"}
				for _, p := range changed {
					reasons = append(reasons, "  "+p)
				}
				reasons = append(reasons, "-> would run")
				return reasons, nil
			}
		}
		return []string{"no cache entry (would run)"}, nil
	}
	if manifest.InputHash != inputHashHex {
		changed := changedInputs(manifest.InputHashes, inputHashesMap)
		if len(changed) > 0 {
			reasons = append(reasons, "inputs changed:")
			for _, p := range changed {
				reasons = append(reasons, "  "+p)
			}
		} else {
			reasons = append(reasons, "inputs changed (hash mismatch)")
		}
		reasons = append(reasons, "-> would run")
		return reasons, nil
	}
	ok, missing := cache.OutputsExist(rootDir, manifest.OutputPaths)
	if !ok {
		reasons = append(reasons, "outputs missing:")
		for _, p := range missing {
			reasons = append(reasons, "  "+p)
		}
		reasons = append(reasons, "-> would run")
		return reasons, nil
	}
	return []string{"up to date (cache hit); would skip"}, nil
}

func changedInputs(prev, cur map[string]string) []string {
	var out []string
	seen := make(map[string]bool)
	for p, h := range cur {
		seen[p] = true
		if prev[p] != h {
			out = append(out, p)
		}
	}
	for p := range prev {
		if !seen[p] {
			out = append(out, p+" (removed)")
		}
	}
	sort.Strings(out)
	return out
}
