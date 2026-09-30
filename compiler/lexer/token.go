// Package lexer turns G++ source text into tokens.
//
// G++ is a superset of Go, so the lexer reuses the token kinds from the Go
// standard library (go/token) and implements Go's automatic semicolon
// insertion. The additional G++ keywords (class, extends, this, ...) are
// contextual: they are lexed as identifiers and recognized by the parser only
// where they are meaningful, so existing Go code that uses these words as
// identifiers keeps working.
package lexer

import (
	"fmt"
	"go/token"

	"github.com/raptordnj/gpp/compiler/diag"
)

// Kind is the token kind. It is Go's token type.
type Kind = token.Token

// Token is a single lexical token.
type Token struct {
	Kind Kind
	Lit  string   // literal text for identifiers, literals and comments; "\n" for automatic semicolons
	Pos  diag.Pos // byte offset of the first character
}

func (t Token) String() string {
	switch {
	case t.Kind == token.IDENT || t.Kind.IsLiteral():
		return fmt.Sprintf("%s(%s)", t.Kind, t.Lit)
	case t.Kind == token.SEMICOLON && t.Lit == "\n":
		return "newline"
	}
	return t.Kind.String()
}

// Is reports whether t is the identifier/contextual keyword word.
func (t Token) Is(word string) bool {
	return t.Kind == token.IDENT && t.Lit == word
}

// GppKeywords are the contextual keywords G++ adds on top of Go.
var GppKeywords = map[string]bool{
	"function":    true,
	"class":       true,
	"abstract":    true,
	"extends":     true,
	"implements":  true,
	"constructor": true,
	"override":    true,
	"public":      true,
	"private":     true,
	"protected":   true,
	"internal":    true,
	"static":      true,
	"property":    true,
	"enum":        true,
	"this":        true,
	"super":       true,
	"new":         true,
}

// IsGppKeyword reports whether word is a G++ contextual keyword.
func IsGppKeyword(word string) bool { return GppKeywords[word] }

// IsKeyword reports whether word is a Go keyword or a G++ keyword.
func IsKeyword(word string) bool { return token.IsKeyword(word) || GppKeywords[word] }
