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
File       = (DotenvLine)? (SuiteBlock | TargetBlock)+
DotenvLine = "dotenv" Ident*
SuiteBlock = "suite" Ident "{" Ident* "}"
TargetBlock = "target" Ident ("{" TargetBody "}" | "cmd" Ident*)
TargetBody = (DepsClause | WhenClause | StepsBlock | EnvBlock | ArgsClause | CwdClause | DescClause | TagsClause | PassthroughClause)*
DepsClause = "deps" Ident ("," Ident)*
WhenClause = "when" ("env" Ident | "cmd" "[" (Ident|String)* "]")   # planned v1.2
StepsBlock = "steps" "{" Step+ "}"
Step       = "exec" "[" (Ident|String)* "]" | "cmd" Ident* | "shell" Ident String
EnvBlock   = "env" "{" (Ident (Ident|String))* "}"
ArgsClause = "args" Ident Ident Ident? Ident?
CwdClause  = "cwd" Ident
DescClause = "desc" String
TagsClause = "tags" Ident*
PassthroughClause = "passthrough" "step" "=" Int
```

Single-line target: only allowed when the file has exactly one target and that target has no other clauses (one `cmd` only).
