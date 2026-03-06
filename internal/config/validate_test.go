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

func TestProfileByName(t *testing.T) {
	cfg := &File{
		Targets: []*Target{{Name: "build"}},
		Profiles: []*Profile{
			{Name: "prod", Env: map[string]string{"ENV": "prod"}},
			{Name: "staging"},
		},
	}
	if p := cfg.ProfileByName("prod"); p == nil || p.Env["ENV"] != "prod" {
		t.Errorf("ProfileByName(prod): got %v", p)
	}
	if p := cfg.ProfileByName("staging"); p == nil || p.Name != "staging" {
		t.Errorf("ProfileByName(staging): got %v", p)
	}
	if cfg.ProfileByName("missing") != nil {
		t.Error("ProfileByName(missing) should be nil")
	}
}

func TestValidateDuplicateProfile(t *testing.T) {
	cfg := &File{
		Targets: []*Target{{Name: "build"}},
		Profiles: []*Profile{
			{Name: "prod"},
			{Name: "prod"},
		},
	}
	errs := Validate(cfg)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	if !strings.Contains(errs[0].Message, "duplicate profile") || !strings.Contains(errs[0].Message, "prod") {
		t.Errorf("unexpected message: %s", errs[0].Message)
	}
}

func TestValidateSuiteUnknownTarget(t *testing.T) {
	cfg := &File{
		Targets: []*Target{{Name: "build"}},
		Suites:  []*Suite{{Name: "dev", Targets: []string{"build", "deploy"}, Line: 2, Column: 1}},
	}
	errs := Validate(cfg)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	if !strings.Contains(errs[0].Message, "unknown target") || !strings.Contains(errs[0].Message, "deploy") {
		t.Errorf("unexpected message: %s", errs[0].Message)
	}
}

func TestValidationErrorError(t *testing.T) {
	e := ValidationError{Filename: "Bakefile", Line: 2, Column: 3, Message: "test error"}
	s := e.Error()
	if s == "" || !strings.Contains(s, "test error") {
		t.Errorf("Error(): %q", s)
	}
	e2 := ValidationError{Message: "no pos"}
	if e2.Error() != "no pos" {
		t.Errorf("Error() without pos: %q", e2.Error())
	}
	e3 := ValidationError{Filename: "Bakefile", Message: "file only"}
	if !strings.Contains(e3.Error(), "Bakefile") || !strings.Contains(e3.Error(), "file only") {
		t.Errorf("Error() filename+msg: %q", e3.Error())
	}
	e4 := ValidationError{Line: 1, Column: 2, Message: "line col no file"}
	if !strings.Contains(e4.Error(), "1") || !strings.Contains(e4.Error(), "2") {
		t.Errorf("Error() line+col: %q", e4.Error())
	}
}
