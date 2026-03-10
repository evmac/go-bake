package dsl

import (
	"fmt"
	"io"
	"sort"
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
	// Group file entries by type; output order: imports, private, suites (by name), profiles (by name), targets (by name). Daemons are inline inside suites (e.g. suite up).
	var imports []*ImportLine
	var hasPrivate bool
	var suites []*SuiteBlock
	var profiles []*ProfileBlock
	var targets []*TargetBlock
	for _, e := range ast.Entries {
		if e == nil {
			continue
		}
		if e.Import != nil {
			imports = append(imports, e.Import)
		}
		if e.Private != nil {
			hasPrivate = true
		}
		if e.Suite != nil {
			suites = append(suites, e.Suite)
		}
		if e.Profile != nil {
			profiles = append(profiles, e.Profile)
		}
		if e.Target != nil {
			targets = append(targets, e.Target)
		}
	}
	sort.Slice(suites, func(i, j int) bool { return suites[i].Name < suites[j].Name })
	sort.Slice(profiles, func(i, j int) bool { return profiles[i].Name < profiles[j].Name })
	sort.Slice(targets, func(i, j int) bool { return targets[i].Name < targets[j].Name })
	needNL := false
	for _, imp := range imports {
		if needNL {
			f.write("\n")
		}
		f.write("import ")
		f.writeQuoted(imp.Path.Value)
		f.write("\n")
		needNL = true
	}
	if hasPrivate {
		if needNL {
			f.write("\n")
		}
		f.write("private\n")
		needNL = true
	}
	for _, s := range suites {
		if needNL {
			f.write("\n")
		}
		f.writeSuite(s)
		needNL = true
	}
	for _, p := range profiles {
		if needNL {
			f.write("\n")
		}
		f.writeProfile(p)
		needNL = true
	}
	for _, t := range targets {
		if needNL {
			f.write("\n")
		}
		f.writeTarget(t)
		needNL = true
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
					f.writeQuoted(path.Value)
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

func (f *formatter) writeDaemon(d *DaemonBlock) {
	if d == nil || d.Body == nil {
		if d != nil {
			f.write("daemon " + d.Name + " {\n}\n")
		}
		return
	}
	f.write("daemon " + d.Name + " {\n")
	entries := orderBodyEntries(d.Body.Entries)
	for _, e := range entries {
		// Daemon body: only steps, env, cwd, image, unsafe (no deps, inputs, outputs, when, preset)
		if e.Steps != nil || e.Step != nil || e.Env != nil || e.Cwd != nil || e.Image != nil || e.Unsafe != nil {
			f.writeBodyEntry(e, "  ")
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
	var desc, deps, whenEnv, whenCmd, inputs, outputs, args, env, cwd, tags, private, passthrough, presets, pool, mutex, image, unsafe, nets, vols, steps, workflow, daemons []*BodyEntry
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
		case e.Preset != nil:
			presets = append(presets, e)
		case e.Pool != nil:
			pool = append(pool, e)
		case e.Mutex != nil:
			mutex = append(mutex, e)
		case e.Image != nil:
			image = append(image, e)
		case e.Unsafe != nil:
			unsafe = append(unsafe, e)
		case e.Net != nil:
			nets = append(nets, e)
		case e.Vol != nil:
			vols = append(vols, e)
		case e.Steps != nil, e.Step != nil:
			steps = append(steps, e)
		case e.Workflow != nil:
			workflow = append(workflow, e)
		case e.Daemon != nil:
			daemons = append(daemons, e)
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
	out = append(out, pool...)
	out = append(out, mutex...)
	out = append(out, image...)
	out = append(out, unsafe...)
	out = append(out, nets...)
	out = append(out, vols...)
	out = append(out, steps...)
	out = append(out, workflow...)
	out = append(out, daemons...)
	out = append(out, presets...)
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
		f.write(fmt.Sprintf("%spassthrough step = %d", indent, e.Passthrough.Step))
		if e.Passthrough.Name != nil && e.Passthrough.Name.Value != "" {
			f.write(" name ")
			f.writeQuoted(e.Passthrough.Name.Value)
		}
		f.write("\n")
	case e.Preset != nil:
		f.write(indent + "preset " + e.Preset.Name + " {\n")
		if e.Preset.Body != nil {
			for _, ve := range e.Preset.Body.Entries {
				if ve == nil {
					continue
				}
				if ve.Desc != nil {
					f.write(indent + "  desc ")
					f.writeQuoted(ve.Desc.Text.Value)
					f.write("\n")
				}
				if ve.Steps != nil && len(ve.Steps.Steps) > 0 {
					f.write(indent + "  steps {")
					if len(ve.Steps.Steps) == 1 {
						f.write(" ")
						f.writeStep(ve.Steps.Steps[0])
						f.write(" }\n")
					} else {
						for _, s := range ve.Steps.Steps {
							f.write("\n" + indent + "    ")
							f.writeStep(s)
						}
						f.write("\n" + indent + "  }\n")
					}
				}
				if ve.Argv != nil {
					f.write(indent + "  argv [")
					for i, elem := range ve.Argv.Argv {
						if i > 0 {
							f.write(", ")
						}
						f.writeExecElem(elem)
					}
					f.write("]\n")
				}
				if ve.Env != nil && len(ve.Env.Pairs) > 0 {
					f.write(indent + "  env {")
					for _, p := range ve.Env.Pairs {
						f.write(" " + p.Key + " ")
						f.writeEnvValue(p.Value)
					}
					f.write(" }\n")
				}
			}
		}
		f.write(indent + "}\n")
	case e.Pool != nil:
		f.write(indent + "pool " + e.Pool.Name + "\n")
	case e.Mutex != nil:
		f.write(indent + "mutex " + e.Mutex.Name + "\n")
	case e.Image != nil:
		if e.Image.Ident != nil {
			f.write(indent + "image " + *e.Image.Ident + "\n")
		} else {
			f.write(indent + "image ")
			f.writeQuoted(e.Image.String.Value)
			f.write("\n")
		}
	case e.Unsafe != nil:
		f.write(indent + "unsafe\n")
	case e.Net != nil:
		f.write(indent + "net " + e.Net.Name + "\n")
	case e.Vol != nil:
		f.write(indent + "vol " + e.Vol.Name)
		if e.Vol.HostPath != nil && e.Vol.HostPath.Value != "" {
			f.write(" ")
			f.writeQuoted(e.Vol.HostPath.Value)
		}
		f.write("\n")
	case e.Workflow != nil:
		f.write(indent + "workflow {")
		for _, entry := range e.Workflow.Entries {
			if entry == nil {
				continue
			}
			if entry.Ident != "" {
				f.write(" " + entry.Ident)
			}
			if entry.Schedule != nil {
				if entry.Schedule.Cron != nil {
					f.write(" schedule cron ")
					f.writeQuoted(entry.Schedule.Cron.Value)
				}
				if entry.Schedule.Interval != nil {
					f.write(" schedule interval ")
					f.writeQuoted(entry.Schedule.Interval.Value)
				}
			}
		}
		f.write(" }\n")
	case e.Daemon != nil:
		d := e.Daemon
		f.write(indent + "daemon " + d.Name + " {\n")
		if d.Body != nil {
			for _, be := range orderBodyEntries(d.Body.Entries) {
				if be == nil {
					continue
				}
				if be.Steps != nil || be.Step != nil || be.Env != nil || be.Cwd != nil || be.Image != nil || be.Unsafe != nil {
					f.writeBodyEntry(be, indent+"  ")
				}
			}
		}
		f.write(indent + "}\n")
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
	f.writeQuoted(e.Value)
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
	entries := make([]*SuiteEntry, 0, len(s.Entries))
	for _, e := range s.Entries {
		if e != nil {
			entries = append(entries, e)
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Raw() < entries[j].Raw() })
	for _, e := range entries {
		if e.Ident != nil {
			f.write("  " + e.Ident.V + "\n")
		} else if e.Quoted != nil {
			raw := e.Raw()
			f.write("  " + strings.Replace(raw, " ", ".", 1) + "\n")
		}
	}
	f.write("}\n")
}
