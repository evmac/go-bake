package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

const manifestDir = ".bake/cache"

// Manifest stores the result of a successful run for a given key.
type Manifest struct {
	TargetName   string            `json:"target_name,omitempty"` // for --why lookup
	InputHash    string            `json:"input_hash"`
	InputHashes  map[string]string `json:"input_hashes,omitempty"` // path -> hex hash for --why
	OutputPaths  []string          `json:"output_paths"`
	OutputMTimes map[string]int64  `json:"output_mtimes,omitempty"`
}

// Key computes a cache key from target name, input content hash, and step signature.
func Key(targetName string, inputHash []byte, stepSig []byte) string {
	h := sha256.New()
	h.Write([]byte(targetName))
	h.Write(inputHash)
	h.Write(stepSig)
	return hex.EncodeToString(h.Sum(nil))
}

// HashFiles reads each file and returns a combined content hash (SHA256 of sorted path+hash).
func HashFiles(rootDir string, files []string) ([]byte, error) {
	sort.Strings(files)
	hh := sha256.New()
	for _, f := range files {
		path := filepath.Join(rootDir, f)
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read input %s: %w", f, err)
		}
		sum := sha256.Sum256(data)
		hh.Write([]byte(f))
		hh.Write(sum[:])
	}
	out := sha256.Sum256(hh.Sum(nil))
	return out[:], nil
}

// HashFilesMap returns the combined hash and a per-file map path->hex for --why.
func HashFilesMap(rootDir string, files []string) (combined []byte, perFile map[string]string, err error) {
	sort.Strings(files)
	perFile = make(map[string]string)
	hh := sha256.New()
	for _, f := range files {
		path := filepath.Join(rootDir, f)
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, nil, fmt.Errorf("read input %s: %w", f, err)
		}
		sum := sha256.Sum256(data)
		hexSum := hex.EncodeToString(sum[:])
		perFile[f] = hexSum
		hh.Write([]byte(f))
		hh.Write(sum[:])
	}
	out := sha256.Sum256(hh.Sum(nil))
	return out[:], perFile, nil
}

// ResolveGlobs expands glob patterns under rootDir and returns sorted unique paths (relative to rootDir).
// Patterns without glob metacharacters are treated as literal paths (included if they exist).
func ResolveGlobs(rootDir string, patterns []string) ([]string, error) {
	seen := make(map[string]bool)
	var out []string
	for _, p := range patterns {
		pat := filepath.Join(rootDir, p)
		matches, err := filepath.Glob(pat)
		if err != nil {
			return nil, fmt.Errorf("glob %s: %w", p, err)
		}
		for _, m := range matches {
			rel, err := filepath.Rel(rootDir, m)
			if err != nil {
				return nil, err
			}
			info, err := os.Stat(m)
			if err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return nil, err
			}
			if info.IsDir() {
				err := filepath.Walk(m, func(path string, info os.FileInfo, err error) error {
					if err != nil || info.IsDir() {
						return err
					}
					r, _ := filepath.Rel(rootDir, path)
					if !seen[r] {
						seen[r] = true
						out = append(out, r)
					}
					return nil
				})
				if err != nil {
					return nil, err
				}
			} else {
				if !seen[rel] {
					seen[rel] = true
					out = append(out, rel)
				}
			}
		}
	}
	sort.Strings(out)
	return out, nil
}

// CacheDir returns the cache directory under rootDir.
func CacheDir(rootDir string) string {
	return filepath.Join(rootDir, manifestDir)
}

// LoadManifest reads the manifest for the given key from rootDir's cache.
func LoadManifest(rootDir, key string) (*Manifest, error) {
	path := filepath.Join(CacheDir(rootDir), key+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// LoadManifestForTarget finds a manifest for the given target name by scanning cache files.
// Used by --why when the current key has no entry (e.g. inputs changed).
func LoadManifestForTarget(rootDir, targetName string) (*Manifest, error) {
	dir := CacheDir(rootDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var m Manifest
		if json.Unmarshal(data, &m) != nil {
			continue
		}
		if m.TargetName == targetName {
			return &m, nil
		}
	}
	return nil, nil
}

// SaveManifest writes the manifest for the given key.
func SaveManifest(rootDir, key string, m *Manifest) error {
	dir := CacheDir(rootDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, key+".json"), data, 0644)
}

// OutputsExist returns true if all paths exist under rootDir.
func OutputsExist(rootDir string, paths []string) (bool, []string) {
	var missing []string
	for _, p := range paths {
		if _, err := os.Stat(filepath.Join(rootDir, p)); err != nil {
			if os.IsNotExist(err) {
				missing = append(missing, p)
			}
		}
	}
	return len(missing) == 0, missing
}

// RecordOutputMTimes stats each output path and returns a map path->mtime.
func RecordOutputMTimes(rootDir string, paths []string) (map[string]int64, error) {
	out := make(map[string]int64)
	for _, p := range paths {
		info, err := os.Stat(filepath.Join(rootDir, p))
		if err != nil {
			return nil, err
		}
		out[p] = info.ModTime().Unix()
	}
	return out, nil
}
