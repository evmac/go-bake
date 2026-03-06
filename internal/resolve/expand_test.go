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

func TestExpandEnv(t *testing.T) {
	data := map[string]interface{}{"env": "prod", "port": "8080"}
	env := map[string]string{"ENV": "{{.env}}", "PORT": "{{.port}}"}
	got, err := ExpandEnv(env, data)
	if err != nil {
		t.Fatal(err)
	}
	if got["ENV"] != "prod" || got["PORT"] != "8080" {
		t.Errorf("got %v", got)
	}
}

func TestExpandInvalidTemplate(t *testing.T) {
	_, err := Expand("{{.unclosed", map[string]interface{}{"x": "y"})
	if err == nil {
		t.Error("expected error for invalid template")
	}
}

func TestExpandArgvInvalidTemplate(t *testing.T) {
	_, err := ExpandArgv([]string{"ok", "{{.broken"}, map[string]interface{}{"x": "y"})
	if err == nil {
		t.Error("expected error from Expand in ExpandArgv")
	}
}

func TestExpandEnvInvalidTemplate(t *testing.T) {
	env := map[string]string{"K": "{{.unclosed"}
	_, err := ExpandEnv(env, map[string]interface{}{"x": "y"})
	if err == nil {
		t.Error("expected error from Expand in ExpandEnv")
	}
}
