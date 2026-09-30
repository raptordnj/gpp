package parser

import (
	"go/token"

	"gpp/compiler/ast"
	"gpp/compiler/lexer"
)

func (p *Parser) parseDecl() ast.Decl {
	switch {
	case p.tok == token.CONST || p.tok == token.VAR:
		return p.parseGenDecl(p.tok, p.parseValueSpec)
	case p.tok == token.TYPE:
		return p.parseGenDecl(token.TYPE, p.parseTypeSpec)
	case p.tok == token.FUNC || p.isWord("function"):
		return p.parseFuncDecl()
	case p.isWord("class") || p.isWord("abstract"):
		return p.parseClassDecl()
	case p.tok == token.INTERFACE:
		return p.parseInterfaceDecl()
	case p.isWord("enum"):
		return p.parseEnumDecl()
	case p.tok == token.IMPORT:
		p.errorf(p.pos, "syntax error: imports must appear before other declarations")
	case p.tok == token.SEMICOLON:
		p.next()
		return p.parseDecl()
	}
	p.errorf(p.pos, "syntax error: non-declaration statement outside function body (found %s)", p.describe())
	return nil
}

func (p *Parser) parseGenDecl(keyword token.Token, spec func() ast.Spec) *ast.GenDecl {
	d := &ast.GenDecl{TokPos: p.expect(keyword), Tok: keyword}
	if p.tok == token.LPAREN {
		d.Lparen = true
		p.next()
		for p.tok != token.RPAREN && p.tok != token.EOF {
			d.Specs = append(d.Specs, spec())
		}
		p.expect(token.RPAREN)
		p.expectSemi()
	} else {
		d.Specs = append(d.Specs, spec())
	}
	return d
}

func (p *Parser) parseImportSpec() ast.Spec {
	s := &ast.ImportSpec{}
	switch p.tok {
	case token.PERIOD:
		s.Name = &ast.Ident{NamePos: p.pos, Name: "."}
		p.next()
	case token.IDENT:
		s.Name = p.parseIdent()
	}
	if p.tok != token.STRING {
		p.errorExpected("import path")
	}
	s.Path = &ast.BasicLit{ValuePos: p.pos, Kind: token.STRING, Value: p.lit}
	if unquote(p.lit) == "" {
		p.errorf(p.pos, "invalid import path: empty string")
	}
	p.next()
	p.expectSemi()
	return s
}

func (p *Parser) parseValueSpec() ast.Spec {
	s := &ast.ValueSpec{Names: p.parseIdentList()}
	if p.tok != token.EOF && p.tok != token.SEMICOLON && p.tok != token.RPAREN && p.tok != token.ASSIGN {
		s.Type = p.parseType()
	}
	if p.tok == token.ASSIGN {
		p.next()
		s.Values = p.parseExprList()
	}
	p.expectSemi()
	return s
}

// isTypeParamStart decides, after "type Name [", whether a type parameter
// list follows (as opposed to an array length).
func (p *Parser) isTypeParamStart() bool {
	if p.tok != token.IDENT {
		return false
	}
	switch p.peek(1).Kind {
	case token.IDENT, token.COMMA, token.INTERFACE, token.TILDE, token.LBRACK,
		token.MAP, token.CHAN, token.FUNC, token.STRUCT, token.LPAREN:
		return true
	case token.MUL, token.OR:
		// "[P *C]" is an array length expression (as in Go); a top-level
		// comma or a union makes it a type parameter list: "[P *C, Q any]".
		depth := 0
		for i := 1; ; i++ {
			switch p.peek(i).Kind {
			case token.LBRACK, token.LPAREN, token.LBRACE:
				depth++
			case token.RPAREN, token.RBRACE:
				depth--
			case token.RBRACK:
				if depth == 0 {
					return false
				}
				depth--
			case token.COMMA:
				if depth == 0 {
					return true
				}
			case token.OR:
				if depth == 0 && p.peek(1).Kind == token.MUL {
					return true
				}
			case token.EOF, token.SEMICOLON:
				return false
			}
		}
	}
	return false
}

func (p *Parser) parseTypeSpec() ast.Spec {
	s := &ast.TypeSpec{Name: p.parseIdent()}
	if p.tok == token.LBRACK {
		lbrack := p.pos
		p.next()
		if p.isTypeParamStart() {
			s.TypeParams = p.parseParameterListAfterOpen(lbrack, token.RBRACK, true)
			if p.tok == token.ASSIGN {
				s.Assign = true
				p.next()
			}
			s.Type = p.parseType()
		} else {
			s.Type = p.parseArrayTypeAfterOpen(lbrack)
		}
	} else {
		if p.tok == token.ASSIGN {
			s.Assign = true
			p.next()
		}
		s.Type = p.parseType()
	}
	p.expectSemi()
	return s
}

// parseFuncDecl parses "func" or "function" declarations.
func (p *Parser) parseFuncDecl() *ast.FuncDecl {
	pos := p.pos
	p.next() // func or function
	d := &ast.FuncDecl{}
	if p.tok == token.LPAREN {
		d.Recv = p.parseParameters(false)
	}
	d.Name = p.parseIdent()
	var tparams *ast.FieldList
	if p.tok == token.LBRACK {
		lbrack := p.pos
		p.next()
		tparams = p.parseParameterListAfterOpen(lbrack, token.RBRACK, true)
	}
	d.Type = p.parseSignature(pos)
	d.Type.TypeParams = tparams
	if p.tok == token.LBRACE {
		d.Body = p.parseBody()
	}
	p.expectSemi()
	return d
}

// parseSignature parses parameters and results.
func (p *Parser) parseSignature(pos ast.Pos) *ast.FuncType {
	t := &ast.FuncType{Func: pos}
	t.Params = p.parseParameters(false)
	t.Results = p.parseResult()
	return t
}

func (p *Parser) parseResult() *ast.FieldList {
	if p.tok == token.LPAREN {
		return p.parseParameters(false)
	}
	if typ := p.tryIdentOrType(); typ != nil {
		return &ast.FieldList{Opening: typ.Pos(), List: []*ast.Field{{Type: typ}}}
	}
	return nil
}

// ---------------------------------------------------------------------------
// G++ declarations

// parseClassDecl parses: [abstract] class Name[<T>] [extends P] [implements I, J] { members }
func (p *Parser) parseClassDecl() *ast.ClassDecl {
	c := &ast.ClassDecl{ClassPos: p.pos}
	if p.isWord("abstract") {
		c.Abstract = true
		p.next()
		if !p.isWord("class") {
			p.errorExpected("'class' after 'abstract'")
		}
	}
	p.next() // class
	c.Name = p.parseIdent()
	c.TypeParams = p.parseOptTypeParams()
	if p.isWord("extends") {
		p.next()
		c.Extends = p.parseClassRef()
	}
	if p.isWord("implements") {
		p.next()
		c.Implements = append(c.Implements, p.parseClassRef())
		for p.tok == token.COMMA {
			p.next()
			c.Implements = append(c.Implements, p.parseClassRef())
		}
	}
	p.expect(token.LBRACE)
	for p.tok != token.RBRACE && p.tok != token.EOF {
		if p.tok == token.SEMICOLON {
			p.next()
			continue
		}
		p.parseClassMember(c)
	}
	p.expect(token.RBRACE)
	p.expectSemi()
	return c
}

// parseOptTypeParams parses optional generic parameters in either G++ form
// <T, K comparable> or Go form [T any, K comparable].
func (p *Parser) parseOptTypeParams() *ast.FieldList {
	switch p.tok {
	case token.LBRACK:
		lbrack := p.pos
		p.next()
		return p.parseParameterListAfterOpen(lbrack, token.RBRACK, true)
	case token.LSS:
		l := &ast.FieldList{Opening: p.pos}
		p.next()
		for {
			name := p.parseIdent()
			var constraint ast.Expr
			if p.tok != token.COMMA && p.tok != token.GTR {
				constraint = p.parseConstraint()
			} else {
				constraint = &ast.Ident{NamePos: name.Pos(), Name: "any"}
			}
			l.List = append(l.List, &ast.Field{Names: []*ast.Ident{name}, Type: constraint})
			if p.tok != token.COMMA {
				break
			}
			p.next()
		}
		p.expectGtr()
		return l
	}
	return nil
}

// expectGtr consumes a '>' closing a generic argument list, splitting '>>'.
// Like Go's ']', a closing '>' at the end of a line ends the statement, so an
// automatic semicolon is inserted when the next token is on a later line.
func (p *Parser) expectGtr() {
	switch p.tok {
	case token.SHR:
		p.toks[p.i].Kind = token.GTR
		p.toks[p.i].Pos++
		p.tok = token.GTR
		p.pos++
		return
	case token.GEQ, token.SHR_ASSIGN:
		p.errorExpected("'>'")
	}
	gtr := p.pos
	p.expect(token.GTR)
	if p.tok != token.SEMICOLON && p.tok != token.EOF && p.lineOf(p.pos) > p.lineOf(gtr) {
		semi := lexer.Token{Kind: token.SEMICOLON, Lit: "\n", Pos: gtr + 1}
		p.toks = append(p.toks[:p.i], append([]lexer.Token{semi}, p.toks[p.i:]...)...)
		p.tok, p.lit, p.pos = semi.Kind, semi.Lit, semi.Pos
	}
}

func (p *Parser) lineOf(pos ast.Pos) int { return p.file.Position(pos).Line }

// parseClassRef parses a (possibly qualified, possibly generic) class or
// interface reference used after extends/implements/new.
func (p *Parser) parseClassRef() ast.Expr {
	var x ast.Expr = p.parseIdent()
	if p.tok == token.PERIOD {
		p.next()
		x = &ast.SelectorExpr{X: x, Sel: p.parseIdent()}
	}
	if args, lbrack := p.parseOptTypeArgs(); args != nil {
		x = &ast.IndexExpr{X: x, Lbrack: lbrack, Indices: args}
	}
	return x
}

// parseOptTypeArgs parses <T1, T2> or [T1, T2] if present.
func (p *Parser) parseOptTypeArgs() ([]ast.Expr, ast.Pos) {
	if p.tok != token.LSS && p.tok != token.LBRACK {
		return nil, ast.Pos(0)
	}
	open, pos := p.tok, p.pos
	p.next()
	args := []ast.Expr{p.parseType()}
	for p.tok == token.COMMA {
		p.next()
		args = append(args, p.parseType())
	}
	if open == token.LSS {
		p.expectGtr()
	} else {
		p.expect(token.RBRACK)
	}
	return args, pos
}

func (p *Parser) parseModifiers() ast.Modifiers {
	m := ast.Modifiers{Pos: p.pos}
	for p.tok == token.IDENT {
		var acc ast.Access
		switch p.lit {
		case "public":
			acc = ast.Public
		case "private":
			acc = ast.Private
		case "protected":
			acc = ast.Protected
		case "internal":
			acc = ast.Internal
		case "static":
			if m.Static {
				p.errorf(p.pos, "duplicate modifier 'static'")
			}
			m.Static = true
			p.next()
			continue
		case "abstract":
			if m.Abstract {
				p.errorf(p.pos, "duplicate modifier 'abstract'")
			}
			m.Abstract = true
			p.next()
			continue
		case "override":
			if m.Override {
				p.errorf(p.pos, "duplicate modifier 'override'")
			}
			m.Override = true
			p.next()
			continue
		default:
			return m
		}
		// Only treat the access word as a modifier when a member follows;
		// otherwise it is a field named e.g. "internal".
		next := p.peek(1)
		if next.Kind != token.IDENT && next.Kind != token.FUNC {
			return m
		}
		if m.Access != ast.AccessDefault {
			p.errorf(p.pos, "multiple access modifiers ('%s' and '%s')", m.Access, p.lit)
		}
		m.Access = acc
		p.next()
	}
	return m
}

func (p *Parser) parseClassMember(c *ast.ClassDecl) {
	mods := p.parseModifiers()
	switch {
	case p.isWord("constructor"):
		pos := p.pos
		p.next()
		if c.Ctor != nil {
			p.errorf(pos, "class '%s' already has a constructor (constructor overloading is not supported)", c.Name.Name)
		}
		if mods.Static || mods.Abstract || mods.Override {
			p.errorf(mods.Pos, "invalid modifier on constructor")
		}
		ft := &ast.FuncType{Func: pos, Params: p.parseParameters(false)}
		if p.tok != token.LBRACE {
			p.errorExpected("constructor body")
		}
		c.Ctor = &ast.ConstructorDecl{Mods: mods, ConstructorPos: pos, Type: ft, Body: p.parseBody()}
		p.expectSemi()

	case p.isWord("function") || p.tok == token.FUNC:
		pos := p.pos
		p.next()
		m := &ast.MethodDecl{Mods: mods, Name: p.parseIdent()}
		m.Type = p.parseSignature(pos)
		if p.tok == token.LBRACE {
			if mods.Abstract {
				p.errorf(m.Name.Pos(), "abstract method '%s' cannot have a body", m.Name.Name)
			}
			m.Body = p.parseBody()
		} else if !mods.Abstract {
			p.errorf(p.pos, "missing body for method '%s'", m.Name.Name)
		}
		c.Methods = append(c.Methods, m)
		p.expectSemi()

	case p.isWord("property") && p.peek(1).Kind == token.IDENT:
		p.next()
		prop := &ast.PropertyDecl{Mods: mods, Name: p.parseIdent()}
		prop.Type = p.parseType()
		p.expect(token.LBRACE)
		for p.tok != token.RBRACE && p.tok != token.EOF {
			switch {
			case p.tok == token.SEMICOLON:
				p.next()
			case p.isWord("get") && prop.Getter == nil:
				p.next()
				prop.Getter = p.parseBody()
			case p.isWord("set") && prop.Setter == nil:
				p.next()
				prop.Setter = p.parseBody()
			default:
				p.errorExpected("'get' or 'set' accessor")
			}
		}
		p.expect(token.RBRACE)
		p.expectSemi()
		if prop.Getter == nil && prop.Setter == nil {
			p.errorf(prop.Name.Pos(), "property '%s' must have a get or set accessor", prop.Name.Name)
		}
		c.Properties = append(c.Properties, prop)

	case p.tok == token.IDENT:
		if mods.Abstract || mods.Override {
			p.errorf(mods.Pos, "fields cannot be abstract or override")
		}
		f := &ast.FieldDecl{Mods: mods, Names: p.parseIdentList()}
		f.Type = p.parseType()
		if p.tok == token.ASSIGN {
			p.next()
			f.Value = p.parseExpr()
			if len(f.Names) > 1 {
				p.errorf(f.Names[0].Pos(), "field initializer requires a single field name")
			}
		}
		c.Fields = append(c.Fields, f)
		p.expectSemi()

	default:
		p.errorExpected("class member (field, method, constructor or property)")
	}
}

// parseInterfaceDecl parses a G++ interface: interface Name[<T>] { function M() }.
func (p *Parser) parseInterfaceDecl() *ast.InterfaceDecl {
	d := &ast.InterfaceDecl{InterfacePos: p.expect(token.INTERFACE)}
	d.Name = p.parseIdent()
	d.TypeParams = p.parseOptTypeParams()
	d.Methods = p.parseInterfaceBody()
	p.expectSemi()
	return d
}

// parseEnumDecl parses: enum Name { A B C } (newline or comma separated).
func (p *Parser) parseEnumDecl() *ast.EnumDecl {
	d := &ast.EnumDecl{EnumPos: p.pos}
	p.next()
	d.Name = p.parseIdent()
	p.expect(token.LBRACE)
	seen := map[string]bool{}
	for p.tok != token.RBRACE && p.tok != token.EOF {
		if p.tok == token.SEMICOLON || p.tok == token.COMMA {
			p.next()
			continue
		}
		id := p.parseIdent()
		if seen[id.Name] {
			p.errorf(id.Pos(), "duplicate enum member '%s' in enum '%s'", id.Name, d.Name.Name)
		}
		seen[id.Name] = true
		d.Members = append(d.Members, id)
		if p.tok == token.ASSIGN {
			p.errorf(p.pos, "explicit enum values are not supported yet")
		}
	}
	p.expect(token.RBRACE)
	p.expectSemi()
	if len(d.Members) == 0 {
		p.errorf(d.Name.Pos(), "enum '%s' must have at least one member", d.Name.Name)
	}
	return d
}
