package dsl

import (
	"github.com/alecthomas/participle/v2/lexer"
)

// BakeLexer tokenizes Bakefiles: idents, strings, punctuation, comments.
var BakeLexer = lexer.MustSimple([]lexer.SimpleRule{
	{"Comment", `#[^\n]*`},
	{"String", `"(\\"|[^"])*"`},
	{"Int", `\d+`},
	{"Ident", `[a-zA-Z_./][a-zA-Z0-9_./-]*`},
	{"Punct", `\[|\]|\{|\}|,|=`},
	{"Whitespace", `[ \t\n\r]+`},
})
