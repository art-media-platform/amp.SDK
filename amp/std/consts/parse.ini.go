package consts

import (
	"github.com/alecthomas/participle/v2"
	"github.com/alecthomas/participle/v2/lexer"
)

// ~.ini
// func ParseINI(text []byte) (INI, error) {
// 	iniParser.ParseBytes(
// }

type MarshalOpts int32

const (
	AsText MarshalOpts = 1 << iota
	AsBytes
	AsValue
)

type INI interface {

	// By Convention, the first section is the head or global section and has a nil identifier.
	Sections() []*Section

	//MarshalIn(in []byte, format MarshalOpts) error

	MarshalOut(out []byte, format MarshalOpts) ([]byte, error)
}

// ini, err := parser.Parse("", os.Stdin)
// repr.Println(ini, repr.Indent("  "), repr.OmitEmpty(true))
// if err != nil {
// 	panic(err)
// }

// A custom lexer for INI files. This illustrates a relatively complex Regexp lexer, as well
// as use of the Unquote filter, which unquotes string tokens.
var (
	iniLexer = lexer.MustSimple([]lexer.SimpleRule{
		{`Ident`, `[a-zA-Z_][a-zA-Z_\d]*`},
		{`String`, `"(?:\\.|[^"])*"`},
		{`Float`, `\d+(?:\.\d+)?`},
		{`Punct`, `[][=]`},
		{"comment", `[#;][^\n]*`},
		{"whitespace", `\s+`},
	})
	iniParser = participle.MustBuild[INI](
		participle.Lexer(iniLexer),
		participle.Unquote("String"),
		participle.Union(Expr{}), // TODO should this be here?
	)
)

func Parser() *participle.Parser[INI] {
	return iniParser
}

// type Section struct {
// 	Identifier string      `"[" @Ident "]"`
// 	Properties []*Property `@@*`
// }

type LexerState struct {
	Location lexer.Position
}

type ini struct {
	LexerState

	Entries  []*Entry   `@@*`
	Sections []*Section `@@*`
}

type Section struct {
	LexerState

	Identifier string   `"[" @Ident "]"`
	Entries    []*Entry `@@*`
}

type Entry struct {
	LexerState

	Key   string `(@Ident "=") |`
	Value Expr   `@@`
}

type Expr struct {
	LexerState

	String *string  `  @String`
	Number *float64 `| @Float`
	Int    *int64   `| @Int`
	Bool   *bool    `| (@"true" | "false")`
}

// type Vector struct {
// 	lexer.Position

// 	Elements []*Expr `"[" ( @@ ( ","? @@ )* )? "]"`
// }

func (st *ini) MarshalOut(out []byte, format MarshalOpts) ([]byte, error) {
	// if format&AsText != 0 {
	// 	return st.MarshalText()
	// }
	// if format&AsBytes != 0 {
	// 	return st.MarshalBytes()
	// }
	// if format&AsValue != 0 {
	// 	return st.MarshalValue()
	// }
	return nil, nil
}

/*package parse

import (
	"github.com/alecthomas/participle/v2"
	"github.com/alecthomas/participle/v2/lexer"
)

// EXPERIMANTAL code to generate a parser for the const file format.
// this format is used to define constants in the amp SDK and is useful for
// defining constants, for .proto inspired compiliation to .cs .go .c etc

// A custom lexer for INI files. This illustrates a relatively complex Regexp lexer, as well
// as use of the Unquote filter, which unquotes string tokens.
var (
	iniLexer = lexer.MustSimple([]lexer.SimpleRule{
		{`Ident`, `[a-zA-Z_][a-zA-Z_\d]*`},
		{`String`, `"(?:\\.|[^"])*"`},
		{`Float`, `\d+(?:\.\d+)?`},
		{`Punct`, `[][=]`},
		{"comment", `[#;][^\n]*`},
		{"whitespace", `\s+`},
	})
	parser = participle.MustBuild[INI](
		participle.Lexer(iniLexer),
		participle.Unquote("String"),
		participle.Union[Value](Value{}), // TODO what is this?
	)
)

type LexerState struct {
	lexer.Position
}

type INI struct {
	LexerState

	Properties []*Property `@@*`
	Sections   []*Section  `@@*`
}

type Section struct {
	LexerState

	Identifier string      `"[" @Ident "]"`
	Properties []*Property `@@*`
}

type Property struct {
	LexerState

	Key   string `@Ident "="`
	Value Value  `@@`
}

type Value struct {
	LexerState

	String *string    `  @String`
	Number *float64   `| @Float`
	Int    *int64     `| @Int`
	Bool   *bool      `| (@"true" | "false")`
	Array  *ValueList `| @@`
}

type ValueList struct {
	LexerState

	Elements []*Value `"[" ( @@ ( ","? @@ )* )? "]"`
}

*/
