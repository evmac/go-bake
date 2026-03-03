package env

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LoadDotenv reads the given files from rootDir (each name is relative to rootDir)
// and returns a single env map. Later files override earlier; empty/missing files are skipped.
func LoadDotenv(rootDir string, filenames []string) (map[string]string, error) {
	out := make(map[string]string)
	for _, name := range filenames {
		path := filepath.Join(rootDir, name)
		m, err := readEnvFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		for k, v := range m {
			out[k] = v
		}
	}
	return out, nil
}

// readEnvFile parses a single KEY=VALUE file (blank lines and # comments ignored).
func readEnvFile(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := make(map[string]string)
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.Index(line, "=")
		if idx <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		value := strings.TrimSpace(line[idx+1:])
		// Remove optional surrounding quotes
		if len(value) >= 2 && (value[0] == '"' && value[len(value)-1] == '"' || value[0] == '\'' && value[len(value)-1] == '\'') {
			value = value[1 : len(value)-1]
		}
		out[key] = value
	}
	return out, sc.Err()
}

// Merge overlays envs in order; later maps override earlier. Process env is the base.
func Merge(processEnv []string, dotenv, targetEnv map[string]string) map[string]string {
	// Parse process env into map
	base := make(map[string]string)
	for _, s := range processEnv {
		idx := strings.Index(s, "=")
		if idx > 0 {
			base[strings.TrimSpace(s[:idx])] = strings.TrimSpace(s[idx+1:])
		}
	}
	for k, v := range dotenv {
		base[k] = v
	}
	for k, v := range targetEnv {
		base[k] = v
	}
	return base
}
