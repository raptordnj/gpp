package lexer

import (
	"go/token"
	"testing"

	"gpp/compiler/diag"
)

func kinds(t *testing.T, src string) []Token {
	t.Helper()
	toks, errs := Tokenize(diag.NewFile("t.gpp", []byte(src)), false)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	return toks
}

func TestTokens(t *testing.T) {
	src := "x := a.b(1, 2.5, 'c', \"s\", `raw`) <- ch &^= 3 ... 0x1F 1e9 2i"
	want := []token.Token{
		token.IDENT, token.DEFINE, token.IDENT, token.PERIOD, token.IDENT, token.LPAREN,
		token.INT, token.COMMA, token.FLOAT, token.COMMA, token.CHAR, token.COMMA,
		token.STRING, token.COMMA, token.STRING, token.RPAREN, token.ARROW, token.IDENT,
		token.AND_NOT_ASSIGN, token.INT, token.ELLIPSIS, token.INT, token.FLOAT, token.IMAG,
		token.SEMICOLON, token.EOF,
	}
	toks := kinds(t, src)
	if len(toks) != len(want) {
		t.Fatalf("got %d tokens %v, want %d", len(toks), toks, len(want))
	}
	for i, tok := range toks {
		if tok.Kind != want[i] {
			t.Errorf("token %d: got %v, want %v", i, tok, want[i])
		}
	}
}

func TestSemicolonInsertion(t *testing.T) {
	src := "a\nb++\nreturn\n}\nfoo( // c\n)\nx /* multi\nline */ y"
	var got []string
	for _, tok := range kinds(t, src) {
		got = append(got, tok.String())
	}
	want := []string{"IDENT(a)", "newline", "IDENT(b)", "++", "newline", "return", "newline",
		"}", "newline", "IDENT(foo)", "(", ")", "newline", "IDENT(x)", "newline", "IDENT(y)", "newline", "EOF"}
	if len(got) != len(want) {
		t.Fatalf("got %v\nwant %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("token %d: got %s, want %s\nall: %v", i, got[i], want[i], got)
		}
	}
}

func TestGppKeywordsAreContextual(t *testing.T) {
	toks := kinds(t, "class this super new function")
	for _, tok := range toks[:5] {
		if tok.Kind != token.IDENT || !IsGppKeyword(tok.Lit) {
			t.Errorf("%v: expected contextual keyword identifier", tok)
		}
	}
	if !IsKeyword("func") || !IsKeyword("class") || IsKeyword("foo") {
		t.Error("IsKeyword mismatch")
	}
}

func TestComments(t *testing.T) {
	toks, _ := Tokenize(diag.NewFile("t.gpp", []byte("a // c\n/* d */ b")), true)
	var comments int
	for _, tok := range toks {
		if tok.Kind == token.COMMENT {
			comments++
		}
	}
	if comments != 2 {
		t.Fatalf("expected 2 comments, got %d: %v", comments, toks)
	}
}

func TestPositions(t *testing.T) {
	f := diag.NewFile("t.gpp", []byte("package main\n\n  x"))
	toks, _ := Tokenize(f, false)
	p := f.Position(toks[3].Pos)
	if p.Line != 3 || p.Column != 3 {
		t.Fatalf("got %v, want t.gpp:3:3", p)
	}
}

func TestErrors(t *testing.T) {
	for _, src := range []string{"\"unterminated", "'ab'", "`raw", "/* open", "a @ b"} {
		_, errs := Tokenize(diag.NewFile("t.gpp", []byte(src)), false)
		if len(errs) == 0 {
			t.Errorf("%q: expected a lexical error", src)
		}
	}
}
