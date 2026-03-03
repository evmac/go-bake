package config

import (
	"strings"
	"testing"
)

func TestValidateUnknownDep(t *testing.T) {
	cfg := &File{
		Targets: []*Target{
			{Name: "build", Deps: []string{"generate"}, Line: 2, Column: 1},
			{Name: "test", Deps: []string{"build"}, Line: 5, Column: 1},
		},
	}
	errs := Validate(cfg)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	if !strings.Contains(errs[0].Message, "unknown dependency") || !strings.Contains(errs[0].Message, "generate") {
		t.Errorf("unexpected message: %s", errs[0].Message)
	}
	if errs[0].Line != 2 {
		t.Errorf("expected line 2, got %d", errs[0].Line)
	}
}

func TestValidateDuplicateTarget(t *testing.T) {
	cfg := &File{
		Targets: []*Target{
			{Name: "build", Line: 1, Column: 1},
			{Name: "build", Line: 4, Column: 1},
		},
	}
	errs := Validate(cfg)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	if !strings.Contains(errs[0].Message, "duplicate target") {
		t.Errorf("unexpected message: %s", errs[0].Message)
	}
}

func TestValidateSuiteUnknownTarget(t *testing.T) {
	cfg := &File{
		Targets: []*Target{{Name: "build"}},
		Suites:  []*Suite{{Name: "local", Targets: []string{"build", "deploy"}, Line: 2, Column: 1}},
	}
	errs := Validate(cfg)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	if !strings.Contains(errs[0].Message, "unknown target") || !strings.Contains(errs[0].Message, "deploy") {
		t.Errorf("unexpected message: %s", errs[0].Message)
	}
}

