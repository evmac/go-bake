package resolve

import (
	"testing"
)

func TestExpand(t *testing.T) {
	data := map[string]interface{}{
		"env":     "prod",
		"dry_run": "true",
		"live":    map[string]string{"port": "8080"},
	}
	got, err := Expand("env={{.env}} dry_run={{.dry_run}}", data)
	if err != nil {
		t.Fatal(err)
	}
	if got != "env=prod dry_run=true" {
		t.Errorf("got %q", got)
	}
	got2, err := Expand("port={{.live.port}}", data)
	if err != nil {
		t.Fatal(err)
	}
	if got2 != "port=8080" {
		t.Errorf("got %q", got2)
	}
}

func TestExpandArgv(t *testing.T) {
	data := map[string]interface{}{"x": "hello"}
	argv := []string{"echo", "{{.x}}"}
	got, err := ExpandArgv(argv, data)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[1] != "hello" {
		t.Errorf("got %v", got)
	}
}
