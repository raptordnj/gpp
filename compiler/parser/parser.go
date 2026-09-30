// Package parser implements a recursive-descent parser for G++.
//
// The Go part of the grammar follows the structure of the standard library's
// go/parser (including its handling of composite-literal ambiguities in
// control clauses); the G++ extensions are parsed in declarations.go and in
// the operand rules for new/super.
package parser

import (
	"fmt"
	"go/token"
	"strings"

	"gpp/compiler/ast"
	"gpp/compiler/diag"
	"gpp/compiler/lexer"
)

// Parser holds the parsing state for one file.
type Parser struct {
	file *diag.File
	toks []lexer.Token
	i    int

	tok token.Token
	lit string
	pos diag.Pos

	// exprLev < 0: in control clause (composite literals of bare type names
	// are not allowed); >= 0: in expression.
	exprLev int

	errors diag.ErrorList
}

type bailout struct{}

// ParseFile parses a complete G++ source file.
func ParseFile(file *diag.File) (f *ast.File, err error) {
	toks, lexErrs := lexer.Tokenize(file, false)
	if len(lexErrs) > 0 {
		return nil, lexErrs
	}
	p := &Parser{file: file, toks: toks, i: -1}
	p.next()
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(bailout); !ok {
				panic(r)
			}
			f, err = nil, p.errors
		}
	}()
	f = p.parseFile()
	f.Source = file
	return f, nil
}

// ParseSource parses src as the file name.
func ParseSource(name string, src []byte) (*ast.File, error) {
	return ParseFile(diag.NewFile(name, src))
}

func (p *Parser) next() {
	if p.i < len(p.toks)-1 {
		p.i++
	}
	t := p.toks[p.i]
	p.tok, p.lit, p.pos = t.Kind, t.Lit, t.Pos
}

// peek returns the token n positions ahead of the current one.
func (p *Parser) peek(n int) lexer.Token {
	if p.i+n < len(p.toks) {
		return p.toks[p.i+n]
	}
	return p.toks[len(p.toks)-1]
}

func (p *Parser) isWord(w string) bool { return p.tok == token.IDENT && p.lit == w }

func (p *Parser) errorf(pos diag.Pos, format string, args ...any) {
	p.errors.Add(p.file.Position(pos), format, args...)
	panic(bailout{})
}

func (p *Parser) describe() string {
	switch {
	case p.tok == token.SEMICOLON && p.lit == "\n":
		return "newline"
	case p.tok == token.EOF:
		return "EOF"
	case p.tok == token.IDENT:
		return fmt.Sprintf("'%s'", p.lit)
	case p.tok.IsLiteral():
		return p.lit
	}
	return fmt.Sprintf("'%s'", p.tok)
}

func (p *Parser) errorExpected(what string) {
	p.errorf(p.pos, "syntax error: expected %s, found %s", what, p.describe())
}

func (p *Parser) expect(tok token.Token) diag.Pos {
	pos := p.pos
	if p.tok != tok {
		p.errorExpected("'" + tok.String() + "'")
	}
	p.next()
	return pos
}

// expectClosing is like expect but gives a better message for a missing
// comma before a newline.
func (p *Parser) expectClosing(tok token.Token, context string) diag.Pos {
	if p.tok != tok && p.tok == token.SEMICOLON && p.lit == "\n" {
		p.errorf(p.pos, "syntax error: unexpected newline in %s; possibly missing comma or %s", context, tok)
	}
	return p.expect(tok)
}

func (p *Parser) expectSemi() {
	switch p.tok {
	case token.RPAREN, token.RBRACE:
		// optional before a closing ) or }
	case token.SEMICOLON:
		p.next()
	default:
		p.errorf(p.pos, "syntax error: unexpected %s at end of statement", p.describe())
	}
}

// atComma reports whether a list continues. It errors on a missing comma.
func (p *Parser) atComma(context string, follow token.Token) bool {
	if p.tok == token.COMMA {
		return true
	}
	if p.tok != follow {
		msg := "missing ','"
		if p.tok == token.SEMICOLON && p.lit == "\n" {
			msg += " before newline"
		}
		p.errorf(p.pos, "syntax error: %s in %s", msg, context)
	}
	return false
}

func (p *Parser) parseIdent() *ast.Ident {
	pos := p.pos
	name := "_"
	if p.tok == token.IDENT {
		name = p.lit
		p.next()
	} else {
		p.errorExpected("identifier")
	}
	return &ast.Ident{NamePos: pos, Name: name}
}

func (p *Parser) parseIdentList() []*ast.Ident {
	list := []*ast.Ident{p.parseIdent()}
	for p.tok == token.COMMA {
		p.next()
		list = append(list, p.parseIdent())
	}
	return list
}

func (p *Parser) parseFile() *ast.File {
	f := &ast.File{}
	if p.tok != token.PACKAGE {
		p.errorf(p.pos, "syntax error: package statement must be first")
	}
	pos := p.expect(token.PACKAGE)
	name := p.parseIdent()
	if name.Name == "_" {
		p.errorf(name.Pos(), "invalid package name _")
	}
	f.Package = &ast.PackageDecl{PackagePos: pos, Name: name}
	p.expectSemi()

	for p.tok == token.IMPORT {
		d := p.parseGenDecl(token.IMPORT, p.parseImportSpec)
		f.Decls = append(f.Decls, d)
		for _, s := range d.Specs {
			f.Imports = append(f.Imports, s.(*ast.ImportSpec))
		}
	}
	for p.tok != token.EOF {
		f.Decls = append(f.Decls, p.parseDecl())
	}
	return f
}

// unquote returns the value of a string literal for messages.
func unquote(s string) string {
	return strings.Trim(s, "\"`")
}
