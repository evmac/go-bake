package resolve

import (
	"bytes"
	"fmt"
	"text/template"
)

// Expand replaces {{.key}} and {{.live.key}} in s with values from data.
// data should contain declared args and a "live" map for live args.
func Expand(s string, data map[string]interface{}) (string, error) {
	t, err := template.New("").Parse(s)
	if err != nil {
		return "", err
	}
	var b bytes.Buffer
	if err := t.Execute(&b, data); err != nil {
		return "", err
	}
	return b.String(), nil
}

// ExpandArgv expands each element of argv with data; returns new slice.
func ExpandArgv(argv []string, data map[string]interface{}) ([]string, error) {
	out := make([]string, len(argv))
	for i, s := range argv {
		exp, err := Expand(s, data)
		if err != nil {
			return nil, fmt.Errorf("argv[%d]: %w", i, err)
		}
		out[i] = exp
	}
	return out, nil
}

// ExpandEnv expands values in env map; keys are unchanged.
func ExpandEnv(env map[string]string, data map[string]interface{}) (map[string]string, error) {
	out := make(map[string]string, len(env))
	for k, v := range env {
		exp, err := Expand(v, data)
		if err != nil {
			return nil, fmt.Errorf("env %s: %w", k, err)
		}
		out[k] = exp
	}
	return out, nil
}
