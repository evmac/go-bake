package dsl

import (
	"fmt"
	"io"
	"strings"
)

// Format writes a canonical, pretty-printed Bakefile to w.
// Clause order is deterministic; comments are not preserved (parser elides them).
func Format(ast *Bakefile, w io.Writer) error {
	f := &formatter{w: w}
	return f.writeBakefile(ast)
}

type formatter struct {
	w   io.Writer
	err error
}

func (f *formatter) write(s string) {
	if f.err != nil {
		return
	}
	_, f.err = io.WriteString(f.w, s)
}

func (f *formatter) writeBakefile(ast *Bakefile) error {
	if ast.Dotenv != nil && len(ast.Dotenv.Files) > 0 {
		f.write("dotenv")
		for _, file := range ast.Dotenv.Files {
			f.write(" " + file)
		}
		f.write("\n\n")
	}
	for i, e := range ast.Entries {
		if e == nil {
			continue
		}
		if e.Import != nil {
			f.write("import ")
			f.writeQuoted(e.Import.Path.Value)
			f.write("\n")
		}
		if e.Private != nil {
			f.write("private\n")
		}
		if e.Profile != nil {
			f.writeProfile(e.Profile)
		}
		if e.Target != nil {
			f.writeTarget(e.Target)
		}
		if e.Suite != nil {
			f.writeSuite(e.Suite)
		}
		if i < len(ast.Entries)-1 {
			f.write("\n")
		}
	}
	return f.err
}

func (f *formatter) writeProfile(p *ProfileBlock) {
	if p == nil {
		return
	}
	f.write("profile " + p.Name + " {\n")
	if p.Body != nil {
		for _, e := range p.Body.Entries {
			if e == nil {
				continue
			}
			if e.Dotenv != nil && len(e.Dotenv.Paths) > 0 {
				f.write("  dotenv")
				for _, path := range e.Dotenv.Paths {
					f.write(" ")
					f.writeQuoted(string(path))
				}
				f.write("\n")
			}
			if e.Env != nil && len(e.Env.Pairs) > 0 {
				f.write("  env {")
				for _, pair := range e.Env.Pairs {
					f.write(" " + pair.Key + " ")
					f.writeEnvValue(pair.Value)
				}
				f.write(" }\n")
			}
		}
	}
	f.write("}\n")
}

func (f *formatter) writeTarget(t *TargetBlock) {
	if t == nil {
		return
	}
	if len(t.CmdTok) > 0 {
		f.write("target " + t.Name + " cmd")
		for _, tok := range t.CmdTok {
			f.write(" " + tok)
		}
		f.write("\n")
		return
	}
	f.write("target " + t.Name + " {\n")
	if t.Body != nil {
		entries := orderBodyEntries(t.Body.Entries)
		for _, e := range entries {
			f.writeBodyEntry(e, "  ")
		}
	}
	f.write("}\n")
}

// orderBodyEntries returns body entries in canonical order.
func orderBodyEntries(entries []*BodyEntry) []*BodyEntry {
	var desc, deps, whenEnv, whenCmd, inputs, outputs, args, env, cwd, tags, private, passthrough, steps []*BodyEntry
	for _, e := range entries {
		if e == nil {
			continue
		}
		switch {
		case e.Desc != nil:
			desc = append(desc, e)
		case e.Deps != nil:
			deps = append(deps, e)
		case e.WhenEnv != nil:
			whenEnv = append(whenEnv, e)
		case e.WhenCmd != nil:
			whenCmd = append(whenCmd, e)
		case e.Inputs != nil:
			inputs = append(inputs, e)
		case e.Outputs != nil:
			outputs = append(outputs, e)
		case e.Args != nil:
			args = append(args, e)
		case e.Env != nil:
			env = append(env, e)
		case e.Cwd != nil:
			cwd = append(cwd, e)
		case e.Tags != nil:
			tags = append(tags, e)
		case e.Private != nil:
			private = append(private, e)
		case e.Passthrough != nil:
			passthrough = append(passthrough, e)
		case e.Steps != nil, e.Step != nil:
			steps = append(steps, e)
		}
	}
	var out []*BodyEntry
	out = append(out, desc...)
	out = append(out, deps...)
	out = append(out, whenEnv...)
	out = append(out, whenCmd...)
	out = append(out, inputs...)
	out = append(out, outputs...)
	out = append(out, args...)
	out = append(out, env...)
	out = append(out, cwd...)
	out = append(out, tags...)
	out = append(out, private...)
	out = append(out, passthrough...)
	out = append(out, steps...)
	return out
}

func (f *formatter) writeBodyEntry(e *BodyEntry, indent string) {
	if e == nil {
		return
	}
	switch {
	case e.Desc != nil:
		f.write(indent + "desc ")
		f.writeQuoted(e.Desc.Text.Value)
		f.write("\n")
	case e.Deps != nil:
		f.write(indent + "deps " + strings.Join(e.Deps.Names, ", ") + "\n")
	case e.WhenEnv != nil:
		f.write(indent + "when env " + e.WhenEnv.Var + "\n")
	case e.WhenCmd != nil:
		f.write(indent + "when cmd [")
		for i, elem := range e.WhenCmd.Argv {
			if i > 0 {
				f.write(", ")
			}
			f.writeExecElem(elem)
		}
		f.write("]\n")
	case e.Inputs != nil:
		f.write(indent + "inputs [")
		for i, p := range e.Inputs.Paths {
			if i > 0 {
				f.write(", ")
			}
			f.writeExecElem(p)
		}
		f.write("]\n")
	case e.Outputs != nil:
		f.write(indent + "outputs [")
		for i, p := range e.Outputs.Paths {
			if i > 0 {
				f.write(", ")
			}
			f.writeExecElem(p)
		}
		f.write("]\n")
	case e.Args != nil:
		f.write(indent + "args " + e.Args.Name + " " + e.Args.Type)
		if e.Args.Short != "" {
			f.write(" " + e.Args.Short)
		}
		if e.Args.Default != "" {
			f.write(" " + e.Args.Default)
		}
		f.write("\n")
	case e.Env != nil:
		f.write(indent + "env {")
		for _, p := range e.Env.Pairs {
			f.write(" " + p.Key + " ")
			f.writeEnvValue(p.Value)
		}
		f.write(" }\n")
	case e.Cwd != nil:
		f.write(indent + "cwd " + e.Cwd.Path + "\n")
	case e.Tags != nil:
		f.write(indent + "tags")
		for _, t := range e.Tags.Tags {
			f.write(" " + t)
		}
		f.write("\n")
	case e.Private != nil:
		f.write(indent + "private\n")
	case e.Passthrough != nil:
		f.write(fmt.Sprintf("%spassthrough step = %d\n", indent, e.Passthrough.Step))
	case e.Steps != nil:
		f.write(indent + "steps {")
		if len(e.Steps.Steps) == 0 {
			f.write(" }\n")
			return
		}
		if len(e.Steps.Steps) == 1 {
			f.write(" ")
			f.writeStep(e.Steps.Steps[0])
			f.write(" }\n")
			return
		}
		for _, s := range e.Steps.Steps {
			f.write("\n" + indent + "  ")
			f.writeStep(s)
		}
		f.write("\n" + indent + "}\n")
	case e.Step != nil:
		f.write(indent)
		f.writeStep(e.Step)
		f.write("\n")
	}
}

func (f *formatter) writeStep(s *Step) {
	if s == nil {
		return
	}
	if s.Exec != nil {
		f.write("exec [")
		for i, elem := range s.Exec.Argv {
			if i > 0 {
				f.write(", ")
			}
			f.writeExecElem(elem)
		}
		f.write("]")
		return
	}
	if len(s.Cmd) > 0 {
		f.write("cmd")
		for _, tok := range s.Cmd {
			f.write(" " + tok)
		}
		return
	}
	if s.Shell != nil {
		f.write("shell " + s.Shell.Runner + " ")
		f.writeQuoted(s.Shell.Script.Value)
	}
}

func (f *formatter) writeExecElem(e ExecElem) {
	s := string(e)
	if needsQuoting(s) {
		f.writeQuoted(s)
	} else {
		f.write(s)
	}
}

func (f *formatter) writeEnvValue(v EnvValue) {
	s := string(v)
	if needsQuoting(s) {
		f.writeQuoted(s)
	} else {
		f.write(s)
	}
}

func (f *formatter) writeQuoted(s string) {
	f.write(`"`)
	for _, r := range s {
		if r == '"' {
			f.write(`\"`)
		} else {
			f.write(string(r))
		}
	}
	f.write(`"`)
}

func needsQuoting(s string) bool {
	if s == "" {
		return true
	}
	// Flag-like args must be quoted (idents cannot start with -).
	if len(s) > 0 && s[0] == '-' {
		return true
	}
	for _, r := range s {
		switch r {
		case ' ', '\t', '"', ',', '=', '[', ']', '{', '}', '*', '?':
			return true
		}
	}
	return false
}

func (f *formatter) writeSuite(s *SuiteBlock) {
	if s == nil {
		return
	}
	f.write("suite " + s.Name + " {\n")
	// One target per line so we can add multi-token lines (e.g. "test cover") later.
	for _, t := range s.Targets {
		f.write("  " + t + "\n")
	}
	f.write("}\n")
}
