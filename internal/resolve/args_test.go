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
