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
		PassthroughStep: 1,
	}
	declared, live, passByStep, err := ParseArgs(tgt, []string{"--env", "prod", "--port", "8080", "--", "-v"})
	if err != nil {
		t.Fatal(err)
	}
	if declared["env"] != "prod" || declared["dry_run"] != "false" {
		t.Errorf("declared: %v", declared)
	}
	if live["port"] != "8080" {
		t.Errorf("live: %v", live)
	}
	pass := passByStep[1]
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

func TestParseArgsShortUndeclared(t *testing.T) {
	// -x value when no arg has short "x" -> live["x"] = value (findDeclaredByShort returns "")
	tgt := &config.Target{Args: []config.ArgDecl{{Name: "env", Type: "string", Short: "e"}}}
	_, live, _, err := ParseArgs(tgt, []string{"-x", "val"})
	if err != nil {
		t.Fatal(err)
	}
	if live["x"] != "val" {
		t.Errorf("short -x should go to live: %v", live)
	}
}

func TestParseArgsBareDoubleDashFlag(t *testing.T) {
	tgt := &config.Target{Args: []config.ArgDecl{{Name: "v", Type: "bool"}}}
	declared, _, _, err := ParseArgs(tgt, []string{"--v"})
	if err != nil {
		t.Fatal(err)
	}
	if declared["v"] != "true" {
		t.Errorf("bare --v should set true: %v", declared)
	}
}

func TestParseArgsMultiSlotPassthrough(t *testing.T) {
	tgt := &config.Target{
		Passthrough: []config.PassthroughSlot{{Step: 1}, {Step: 2}},
	}
	_, _, passByStep, err := ParseArgs(tgt, []string{"--", "a", "b", "--", "c"})
	if err != nil {
		t.Fatal(err)
	}
	if len(passByStep[1]) != 2 || passByStep[1][0] != "a" || passByStep[1][1] != "b" {
		t.Errorf("slot 1: got %v", passByStep[1])
	}
	if len(passByStep[2]) != 1 || passByStep[2][0] != "c" {
		t.Errorf("slot 2: got %v", passByStep[2])
	}
}
