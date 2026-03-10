# Grammar spec (sketch)

## Lexer

- **Comment** — `#` to EOL
- **String** — `"(\\"|[^"])*"`
- **Int** — `\d+`
- **Ident** — `[a-zA-Z_./][a-zA-Z0-9_./-]*`
- **Punct** — `[ ] { } , =`
- **Whitespace** — `[ \t\n\r]+` (elided)

## Grammar

```text
File       = (DotenvLine)? (ImportLine | PrivateLine | SuiteBlock | ProfileBlock | TargetBlock)*
DotenvLine = "dotenv" Ident*
ImportLine = "import" String
PrivateLine = "private"   # file-level: all targets in this file are private
SuiteBlock = "suite" Ident "{" SuiteEntry* "}"
SuiteEntry = Ident | String   # target (e.g. build), target.preset (e.g. test.cover), or quoted "target preset". Suite "precommit" is used by install hooks.
ProfileBlock = "profile" Ident "{" (EnvBlock | DotenvLine)* "}"
TargetBlock = "target" Ident ("{" TargetBody "}" | "cmd" Ident*)
TargetBody = (DepsClause | WhenClause | StepsBlock | EnvBlock | ArgsClause | CwdClause | DescClause | TagsClause | PassthroughClause | InputsClause | OutputsClause | PresetBlock | PrivateLine | PoolClause | MutexClause | ImageClause | UnsafeClause | NetClause | VolClause)*
ImageClause = "image" ( Ident | String )
UnsafeClause = "unsafe"
NetClause = "net" Ident
VolClause = "vol" Ident ( String )?   # name; optional host path for bind mount
PresetBlock = "preset" Ident "{" (DescClause | StepsBlock | PresetArgvClause | EnvBlock)+ "}"
PresetArgvClause = "argv" "[" (Ident|String)* "]"   # appends args to passthrough step; ignored when StepsBlock is present
DepsClause = "deps" Ident ("," Ident)*
InputsClause = "inputs" "[" (Ident|String)* "]"
OutputsClause = "outputs" "[" (Ident|String)* "]"
WhenClause = "when" ("env" Ident | "cmd" "[" (Ident|String)* "]")
StepsBlock = "steps" "{" Step+ "}"
Step       = "exec" "[" (Ident|String)* "]" | "cmd" Ident* | "shell" Ident String
EnvBlock   = "env" "{" (Ident (Ident|String))* "}"
ArgsClause = "args" Ident Ident Ident? Ident?
CwdClause  = "cwd" Ident
DescClause = "desc" String
TagsClause = "tags" Ident*
PassthroughClause = "passthrough" "step" "=" Int ( "name" String )?
PoolClause = "pool" Ident
MutexClause = "mutex" Ident
```

Single-line target: only allowed when the file has exactly one target and that target has no other clauses (one `cmd` only).
