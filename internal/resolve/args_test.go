package resolve

import (
	"testing"

	"github.com/evmac/go-bake/internal/config"
)

func TestParseArgs(t *testing.T) {
	tgt := &config.Target{
		Args: []config.ArgDecl{
			{Name: "env", Type: "string", Short: "e", Default: "staging"},
			{Name: "dry_run", Type: "bool", Default: "false"},
		},
	}
	declared, live, pass, err := ParseArgs(tgt, []string{"--env", "prod", "--port", "8080", "--", "-v"})
	if err != nil {
		t.Fatal(err)
	}
	if declared["env"] != "prod" || declared["dry_run"] != "false" {
		t.Errorf("declared: %v", declared)
	}
	if live["port"] != "8080" {
		t.Errorf("live: %v", live)
	}
	if len(pass) != 1 || pass[0] != "-v" {
		t.Errorf("passthrough: %v", pass)
	}
}

func TestParseArgsShortFlag(t *testing.T) {
	tgt := &config.Target{
		Args: []config.ArgDecl{
			{Name: "env", Type: "string", Short: "e", Default: "staging"},
		},
	}
	declared, _, _, err := ParseArgs(tgt, []string{"-e", "prod"})
	if err != nil {
		t.Fatal(err)
	}
	if declared["env"] != "prod" {
		t.Errorf("short -e: declared=%v", declared)
	}
}

func TestTemplateData(t *testing.T) {
	declared := map[string]string{"env": "prod"}
	live := map[string]string{"port": "8080"}
	data := TemplateData(declared, live)
	if data["env"] != "prod" {
		t.Errorf("declared: %v", data)
	}
	if liveMap, ok := data["live"].(map[string]string); !ok || liveMap["port"] != "8080" {
		t.Errorf("live: %v", data["live"])
	}
}

func TestFormatBool(t *testing.T) {
	if got := FormatBool("true"); got != "true" {
		t.Errorf("FormatBool(true)=%q", got)
	}
	if got := FormatBool("false"); got != "false" {
		t.Errorf("FormatBool(false)=%q", got)
	}
	if got := FormatBool("1"); got != "true" {
		t.Errorf("FormatBool(1)=%q", got)
	}
}

func TestParseArgsKeyEqualsValue(t *testing.T) {
	tgt := &config.Target{Args: []config.ArgDecl{{Name: "env", Type: "string", Short: "e"}}}
	declared, _, _, err := ParseArgs(tgt, []string{"--env=prod"})
	if err != nil {
		t.Fatal(err)
	}
	if declared["env"] != "prod" {
		t.Errorf("declared=%v", declared)
	}
}

func TestParseArgsRequiredMissing(t *testing.T) {
	tgt := &config.Target{Args: []config.ArgDecl{{Name: "x", Type: "string", Required: true}}}
	_, _, _, err := ParseArgs(tgt, []string{})
	if err == nil {
		t.Fatal("expected error for required arg missing")
	}
}
