package lint

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// RuleConfig is one rule's config (id + severity). Severity "disabled" turns the rule off.
type RuleConfig struct {
	ID       string
	Severity string // "warn", "error", or "disabled"
}

// Config is the linter config. All registered rules default to "warn"; the config can override per rule.
type Config struct {
	Rules []RuleConfig
}

// LoadConfig reads config from path. If the file does not exist, returns default config (all rules enabled).
func LoadConfig(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultConfig(), nil
		}
		return nil, err
	}
	defer f.Close()
	return parseConfig(f)
}

// DefaultConfig returns config with all registered rules enabled at warn.
func DefaultConfig() *Config {
	cfg := &Config{}
	for _, r := range Registry {
		cfg.Rules = append(cfg.Rules, RuleConfig{ID: r.ID(), Severity: "warn"})
	}
	return cfg
}

// parseConfig reads a simple text format: one rule per line, "#" comment, "rule_id [warn|error|disabled]".
// Rules not listed in the file keep their default severity (warn).
func parseConfig(f *os.File) (*Config, error) {
	overrides := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) == 0 {
			continue
		}
		id := parts[0]
		severity := "warn"
		if len(parts) >= 2 {
			s := strings.ToLower(parts[1])
			if s == "warn" || s == "error" || s == "disabled" {
				severity = s
			}
		}
		overrides[id] = severity
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	cfg := DefaultConfig()
	for i := range cfg.Rules {
		if sev, ok := overrides[cfg.Rules[i].ID]; ok {
			cfg.Rules[i].Severity = sev
		}
	}
	return cfg, nil
}

// RuleEnabled returns true if the rule is not disabled.
func (c *Config) RuleEnabled(id string) bool {
	for _, r := range c.Rules {
		if r.ID == id {
			return r.Severity != "disabled"
		}
	}
	return true
}

// Severity returns the severity for the rule (warn, error, or disabled).
func (c *Config) Severity(id string) string {
	for _, r := range c.Rules {
		if r.ID == id {
			if r.Severity != "" {
				return r.Severity
			}
			return "warn"
		}
	}
	return "warn"
}

// DisableRule sets a rule to "disabled" (adds it if not present).
func (c *Config) DisableRule(id string) {
	for i := range c.Rules {
		if c.Rules[i].ID == id {
			c.Rules[i].Severity = "disabled"
			return
		}
	}
	c.Rules = append(c.Rules, RuleConfig{ID: id, Severity: "disabled"})
}

// WriteConfig writes the config to path in the simple text format.
func (c *Config) WriteConfig(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	var sb strings.Builder
	sb.WriteString("# bake lint config\n")
	sb.WriteString("# rule_id [warn|error|disabled]\n")
	for _, r := range c.Rules {
		if r.Severity == "" {
			r.Severity = "warn"
		}
		sb.WriteString(r.ID + " " + r.Severity + "\n")
	}
	return os.WriteFile(path, []byte(sb.String()), 0644)
}

// ConfigPath returns the config path. Checks .bake/config, .bake-lint.yaml, .bake-lint (in order).
func ConfigPath(bakefileDir string) string {
	for _, name := range []string{".bake/config", ".bake-lint.yaml", ".bake-lint"} {
		p := filepath.Join(bakefileDir, name)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return filepath.Join(bakefileDir, ".bake", "config")
}
