package forge

import (
	"strings"

	"github.com/alecthomas/participle/v2"
	"github.com/alecthomas/participle/v2/lexer"
)

type MarshalOpts int32

const (
	AsText MarshalOpts = 1 << iota
	AsBytes
	AsValue
)

// ParseINI parses INI content from a byte slice and returns an INI struct and/or error.
func ParseINI(filepath string, text []byte) (*INI, error) {
	ini, err := iniParser.ParseBytes(filepath, text)
	return ini, err
}

// A custom lexer for INI files. This illustrates a relatively complex Regexp lexer, as well
// as use of the Unquote filter, which unquotes string tokens.
var (
	iniLexer = lexer.MustSimple([]lexer.SimpleRule{
		{`Ident`, `[A-Za-z\._][A-Za-z0-9_-\.]*`},
		//{`Ident`, `[\p{L}\p{N}\d\.]*`},
		{`String`, `"(?:\\.|[^"])*"`},
		{`Number`, `\d+(?:\.\d+)?`},
		{`Punct`, `[][=]`},
		{"comment", `[#;][^\n]*`},
		{"whitespace", `\s+`},
	})
	iniParser = participle.MustBuild[INI](
		participle.Lexer(iniLexer),
		participle.Unquote("String"),
	)
)

func Parser() *participle.Parser[INI] {
	return iniParser
}

type LexerState struct {
	Location lexer.Position
}

type INI struct {
	LexerState

	Sections []*Section `@@*`
}

type Section struct {
	LexerState

	Header  *Entry   `( "[" @@ "]" )?`
	Entries []*Entry `@@*`
}

type List struct {
	lexer.Position

	Elements []*Value `"[" ( @@ ( ","? @@ )* )? "]"`
}

type Entry struct {
	LexerState

	LHS *Value `@@* "="`
	RHS *Value `@@*`
}

type Value struct {
	LexerState

	Args   []string `(@Ident)*`
	Number *float64 `| @Number`
	List   *List    `| @@`
}

func (val *Value) AsInt() int64 {
	if val.Number != nil {
		return int64(*val.Number)
	}

	// if args present, parse first as a various output types (hex, decimal, etc)
	for _, arg := range val.Args {
		if strings.HasPrefix(arg, "0x") {
			return 0 // TODO hex.ParseInt(arg[2:], 0, 64)
		}
		// TODO: ass other parsers here, parser plugin!?
	}

	if val.List != nil && len(val.List.Elements) > 0 {
		return val.List.Elements[0].AsInt()
	}

	return 0
}

func (v *Value) AsFloat() float64 {
	if v.Number != nil {
		return *v.Number
	}

	// Try parsing Args as float if available
	if len(v.Args) > 0 {
		// Could add float parsing logic here for strings
		// Currently just returning 0 as placeholder
	}

	// Try getting float from nested list if available
	if v.List != nil && len(v.List.Elements) > 0 {
		return v.List.Elements[0].AsFloat()
	}

	return 0.0
}

func (v *Value) AsString() {

}

func (ini *INI) MarshalOut(out []byte, format MarshalOpts) ([]byte, error) {
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
