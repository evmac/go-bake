package config

import "fmt"

// ValidationError is a config error with optional source position.
type ValidationError struct {
	Filename string
	Line     int
	Column   int
	Message  string
}

func (e ValidationError) Error() string {
	if e.Line > 0 && e.Column > 0 {
		if e.Filename != "" {
			return fmt.Sprintf("%s:%d:%d: %s", e.Filename, e.Line, e.Column, e.Message)
		}
		return fmt.Sprintf("%d:%d: %s", e.Line, e.Column, e.Message)
	}
	if e.Filename != "" {
		return fmt.Sprintf("%s: %s", e.Filename, e.Message)
	}
	return e.Message
}

// Validate runs schema and semantic checks on the compiled config.
// Returns a slice of validation errors with file:line:col when available.
func Validate(cfg *File) []ValidationError {
	var errs []ValidationError
	filename := cfg.BakePath
	if filename == "" {
		filename = "Bakefile"
	}

	// Duplicate target names
	seenTarget := make(map[string]int) // name -> first occurrence line
	for _, t := range cfg.Targets {
		if first, ok := seenTarget[t.Name]; ok {
			errs = append(errs, ValidationError{
				Filename: filename,
				Line:     t.Line,
				Column:   t.Column,
				Message:  fmt.Sprintf("duplicate target %q (first at line %d)", t.Name, first),
			})
		} else {
			seenTarget[t.Name] = t.Line
		}
	}

	// Unknown dependencies
	for _, t := range cfg.Targets {
		for _, dep := range t.Deps {
			if cfg.TargetByName(dep) == nil {
				errs = append(errs, ValidationError{
					Filename: filename,
					Line:     t.Line,
					Column:   t.Column,
					Message:  fmt.Sprintf("unknown dependency %q", dep),
				})
			}
		}
	}

	// Duplicate profile names
	seenProfile := make(map[string]int)
	for _, p := range cfg.Profiles {
		if first, ok := seenProfile[p.Name]; ok {
			errs = append(errs, ValidationError{
				Filename: filename,
				Line:     0,
				Column:   0,
				Message:  fmt.Sprintf("duplicate profile %q (first at line %d)", p.Name, first),
			})
		} else {
			seenProfile[p.Name] = 0
		}
	}

	// Duplicate suite names
	seenSuite := make(map[string]int)
	for _, s := range cfg.Suites {
		if first, ok := seenSuite[s.Name]; ok {
			errs = append(errs, ValidationError{
				Filename: filename,
				Line:     s.Line,
				Column:   s.Column,
				Message:  fmt.Sprintf("duplicate suite %q (first at line %d)", s.Name, first),
			})
		} else {
			seenSuite[s.Name] = s.Line
		}
	}

	// Suite references to missing targets
	for _, s := range cfg.Suites {
		for _, name := range s.Targets {
			if cfg.TargetByName(name) == nil {
				errs = append(errs, ValidationError{
					Filename: filename,
					Line:     s.Line,
					Column:   s.Column,
					Message:  fmt.Sprintf("suite %q references unknown target %q", s.Name, name),
				})
			}
		}
	}

	return errs
}
