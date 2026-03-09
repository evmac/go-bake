package lint

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if len(cfg.Rules) != len(Registry) {
		t.Errorf("default config should have %d rules, got %d", len(Registry), len(cfg.Rules))
	}
	if !cfg.RuleEnabled("prefer-exec") {
		t.Error("prefer-exec should be enabled")
	}
	if !cfg.RuleEnabled("prefer-quoted-exec") {
		t.Error("prefer-quoted-exec should be enabled")
	}
	if cfg.Severity("require-desc") != "warn" {
		t.Errorf("severity: got %q", cfg.Severity("require-desc"))
	}
}

func TestLoadConfigMissing(t *testing.T) {
	dir := t.TempDir()
	cfg, err := LoadConfig(filepath.Join(dir, ".bake", "config"))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Rules) == 0 {
		t.Error("expected default rules")
	}
}

func TestLoadConfigWithDisabled(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".bake", "config")
	os.MkdirAll(filepath.Dir(path), 0755)
	content := `# lint config
prefer-exec error
require-desc disabled
brackets-only warn
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Severity("prefer-exec") != "error" {
		t.Errorf("prefer-exec severity: got %q", cfg.Severity("prefer-exec"))
	}
	if cfg.RuleEnabled("require-desc") {
		t.Error("require-desc should be disabled")
	}
	if !cfg.RuleEnabled("brackets-only") {
		t.Error("brackets-only should be enabled")
	}
	if !cfg.RuleEnabled("prefer-quoted-exec") {
		t.Error("prefer-quoted-exec should still be enabled (not in config = default warn)")
	}
}

func TestSeverityDefault(t *testing.T) {
	cfg := &Config{Rules: []RuleConfig{{ID: "x"}}}
	if cfg.Severity("x") != "warn" {
		t.Errorf("Severity(x): got %q", cfg.Severity("x"))
	}
	if cfg.Severity("missing") != "warn" {
		t.Errorf("Severity(missing): got %q", cfg.Severity("missing"))
	}
}

func TestDisableRule(t *testing.T) {
	cfg := DefaultConfig()
	if !cfg.RuleEnabled("require-desc") {
		t.Fatal("require-desc should start enabled")
	}
	cfg.DisableRule("require-desc")
	if cfg.RuleEnabled("require-desc") {
		t.Error("require-desc should be disabled after DisableRule")
	}
	if cfg.Severity("require-desc") != "disabled" {
		t.Errorf("severity after disable: got %q", cfg.Severity("require-desc"))
	}
}

func TestWriteConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".bake", "config")
	cfg := DefaultConfig()
	cfg.DisableRule("require-desc")
	if err := cfg.WriteConfig(path); err != nil {
		t.Fatal(err)
	}
	cfg2, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg2.RuleEnabled("require-desc") {
		t.Error("require-desc should still be disabled after round-trip")
	}
	if !cfg2.RuleEnabled("prefer-exec") {
		t.Error("prefer-exec should still be enabled")
	}
}

func TestConfigPathPrecedence(t *testing.T) {
	dir := t.TempDir()
	// No config files → default to .bake/config
	p := ConfigPath(dir)
	if p != filepath.Join(dir, ".bake", "config") {
		t.Errorf("expected .bake/config, got %s", p)
	}
	// .bake-lint takes precedence over default
	lintFile := filepath.Join(dir, ".bake-lint")
	os.WriteFile(lintFile, []byte("prefer-exec\n"), 0644)
	p = ConfigPath(dir)
	if p != lintFile {
		t.Errorf("expected .bake-lint, got %s", p)
	}
	// .bake/config takes highest precedence
	bakeDir := filepath.Join(dir, ".bake")
	os.MkdirAll(bakeDir, 0755)
	configFile := filepath.Join(bakeDir, "config")
	os.WriteFile(configFile, []byte("prefer-exec warn\n"), 0644)
	p = ConfigPath(dir)
	if p != configFile {
		t.Errorf("expected .bake/config, got %s", p)
	}
}
