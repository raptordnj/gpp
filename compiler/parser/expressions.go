package parser

import (
	"go/token"

	"github.com/raptordnj/gpp/compiler/ast"
)

// ---------------------------------------------------------------------------
// Types

func (p *Parser) parseType() ast.Expr {
	typ := p.tryIdentOrType()
	if typ == nil {
		p.errorExpected("type")
	}
	return typ
}

func (p *Parser) parseQualifiedIdent(ident *ast.Ident) ast.Expr {
	var x ast.Expr = ident
	if p.tok == token.PERIOD {
		p.next()
		x = &ast.SelectorExpr{X: ident, Sel: p.parseIdent()}
	}
	switch p.tok {
	case token.LBRACK:
		x = p.parseTypeInstance(x)
	case token.LSS:
		// G++ generic syntax in type position: Box<int>. A Go type is never
		// followed by '<', so this is unambiguous.
		args, lbrack := p.parseOptTypeArgs()
		x = &ast.IndexExpr{X: x, Lbrack: lbrack, Indices: args}
	}
	return x
}

func (p *Parser) parseTypeInstance(typ ast.Expr) ast.Expr {
	lbrack := p.expect(token.LBRACK)
	var list []ast.Expr
	for p.tok != token.RBRACK && p.tok != token.EOF {
		list = append(list, p.parseType())
		if !p.atComma("type argument list", token.RBRACK) {
			break
		}
		p.next()
	}
	p.expectClosing(token.RBRACK, "type argument list")
	if len(list) == 0 {
		p.errorf(lbrack, "syntax error: expected type argument list")
	}
	return &ast.IndexExpr{X: typ, Lbrack: lbrack, Indices: list}
}

// parseArrayTypeAfterOpen parses the rest of an array or slice type after '['.
func (p *Parser) parseArrayTypeAfterOpen(lbrack ast.Pos) ast.Expr {
	if p.tok == token.RBRACK {
		p.next()
		return &ast.ArrayType{Lbrack: lbrack, Elt: p.parseType()}
	}
	var length ast.Expr
	if p.tok == token.ELLIPSIS {
		length = &ast.Ellipsis{EllipsisPos: p.pos}
		p.next()
	} else {
		p.exprLev++
		length = p.parseExpr()
		p.exprLev--
	}
	p.expect(token.RBRACK)
	return &ast.ArrayType{Lbrack: lbrack, Len: length, Elt: p.parseType()}
}

func (p *Parser) parseStructType() *ast.StructType {
	pos := p.expect(token.STRUCT)
	lbrace := p.expect(token.LBRACE)
	st := &ast.StructType{Struct: pos, Fields: &ast.FieldList{Opening: lbrace}}
	for p.tok != token.RBRACE && p.tok != token.EOF {
		if p.tok == token.SEMICOLON {
			p.next()
			continue
		}
		st.Fields.List = append(st.Fields.List, p.parseFieldDecl())
	}
	p.expect(token.RBRACE)
	return st
}

func (p *Parser) parseFieldDecl() *ast.Field {
	f := &ast.Field{}
	switch p.tok {
	case token.IDENT:
		name := p.parseIdent()
		switch {
		case p.tok == token.PERIOD || p.tok == token.STRING || p.tok == token.SEMICOLON || p.tok == token.RBRACE:
			f.Type = p.parseQualifiedIdent(name) // embedded type
		case p.tok == token.LBRACK:
			names, typ := p.parseArrayFieldOrTypeInstance(name)
			if names != nil {
				f.Names = names
			}
			f.Type = typ
		default:
			f.Names = []*ast.Ident{name}
			for p.tok == token.COMMA {
				p.next()
				f.Names = append(f.Names, p.parseIdent())
			}
			f.Type = p.parseType()
		}
	case token.MUL:
		star := p.pos
		p.next()
		f.Type = &ast.StarExpr{Star: star, X: p.parseQualifiedIdent(p.parseIdent())}
	default:
		p.errorExpected("field name or embedded type")
	}
	if p.tok == token.STRING {
		f.Tag = &ast.BasicLit{ValuePos: p.pos, Kind: token.STRING, Value: p.lit}
		p.next()
	}
	p.expectSemi()
	return f
}

// canStartType reports whether the current token can begin a type.
func (p *Parser) canStartType() bool {
	switch p.tok {
	case token.IDENT, token.MUL, token.LBRACK, token.LPAREN, token.FUNC, token.MAP,
		token.CHAN, token.STRUCT, token.INTERFACE, token.ARROW:
		return true
	}
	return false
}

// parseArrayFieldOrTypeInstance disambiguates "name [N]T" (a named field or
// parameter of array type) from "T[A]" (a generic instantiation).
func (p *Parser) parseArrayFieldOrTypeInstance(name *ast.Ident) ([]*ast.Ident, ast.Expr) {
	lbrack := p.expect(token.LBRACK)
	if p.tok == token.RBRACK {
		p.next()
		return []*ast.Ident{name}, &ast.ArrayType{Lbrack: lbrack, Elt: p.parseType()}
	}
	if p.tok == token.ELLIPSIS {
		p.errorf(p.pos, "syntax error: invalid use of [...] array")
	}
	var args []ast.Expr
	p.exprLev++
	for p.tok != token.RBRACK && p.tok != token.EOF {
		args = append(args, p.parseExpr())
		if !p.atComma("type argument list", token.RBRACK) {
			break
		}
		p.next()
	}
	p.exprLev--
	p.expect(token.RBRACK)
	if len(args) == 1 && p.canStartType() {
		return []*ast.Ident{name}, &ast.ArrayType{Lbrack: lbrack, Len: args[0], Elt: p.parseType()}
	}
	return nil, &ast.IndexExpr{X: name, Lbrack: lbrack, Indices: args}
}

func (p *Parser) parseFuncType() *ast.FuncType {
	pos := p.expect(token.FUNC)
	return p.parseSignature(pos)
}

// parseInterfaceBody parses "{ elements }" for both Go interface types and
// G++ interface declarations (where methods may be prefixed by function/func).
func (p *Parser) parseInterfaceBody() *ast.FieldList {
	l := &ast.FieldList{Opening: p.expect(token.LBRACE)}
	for p.tok != token.RBRACE && p.tok != token.EOF {
		if p.tok == token.SEMICOLON {
			p.next()
			continue
		}
		if (p.isWord("function") || p.tok == token.FUNC) && p.peek(1).Kind == token.IDENT {
			p.next()
		}
		if p.tok == token.IDENT && p.peek(1).Kind == token.LPAREN {
			name := p.parseIdent()
			ft := p.parseSignature(name.Pos())
			l.List = append(l.List, &ast.Field{Names: []*ast.Ident{name}, Type: ft})
		} else {
			l.List = append(l.List, &ast.Field{Type: p.parseConstraint()})
		}
		p.expectSemi()
	}
	p.expect(token.RBRACE)
	return l
}

func (p *Parser) parseInterfaceType() *ast.InterfaceType {
	pos := p.expect(token.INTERFACE)
	return &ast.InterfaceType{Interface: pos, Methods: p.parseInterfaceBody()}
}

// parseConstraint parses a type constraint term list: ~int | string | T.
func (p *Parser) parseConstraint() ast.Expr {
	x := p.parseConstraintTerm()
	for p.tok == token.OR {
		pos := p.pos
		p.next()
		x = &ast.BinaryExpr{X: x, OpPos: pos, Op: token.OR, Y: p.parseConstraintTerm()}
	}
	return x
}

func (p *Parser) parseConstraintTerm() ast.Expr {
	if p.tok == token.TILDE {
		pos := p.pos
		p.next()
		return &ast.UnaryExpr{OpPos: pos, Op: token.TILDE, X: p.parseType()}
	}
	return p.parseType()
}

func (p *Parser) parseMapType() *ast.MapType {
	pos := p.expect(token.MAP)
	p.expect(token.LBRACK)
	key := p.parseType()
	p.expect(token.RBRACK)
	return &ast.MapType{Map: pos, Key: key, Value: p.parseType()}
}

func (p *Parser) parseChanType() *ast.ChanType {
	pos := p.pos
	dir := ast.SEND | ast.RECV
	if p.tok == token.CHAN {
		p.next()
		if p.tok == token.ARROW {
			p.next()
			dir = ast.SEND
		}
	} else {
		p.expect(token.ARROW)
		p.expect(token.CHAN)
		dir = ast.RECV
	}
	return &ast.ChanType{Begin: pos, Dir: dir, Value: p.parseType()}
}

// tryIdentOrType parses a type if one starts at the current token.
func (p *Parser) tryIdentOrType() ast.Expr {
	switch p.tok {
	case token.IDENT:
		return p.parseQualifiedIdent(p.parseIdent())
	case token.LBRACK:
		lbrack := p.pos
		p.next()
		return p.parseArrayTypeAfterOpen(lbrack)
	case token.STRUCT:
		return p.parseStructType()
	case token.MUL:
		star := p.pos
		p.next()
		return &ast.StarExpr{Star: star, X: p.parseType()}
	case token.FUNC:
		return p.parseFuncType()
	case token.INTERFACE:
		return p.parseInterfaceType()
	case token.MAP:
		return p.parseMapType()
	case token.CHAN, token.ARROW:
		return p.parseChanType()
	case token.LPAREN:
		lparen := p.pos
		p.next()
		typ := p.parseType()
		p.expect(token.RPAREN)
		return &ast.ParenExpr{Lparen: lparen, X: typ}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Parameters

type paramEntry struct {
	name *ast.Ident
	typ  ast.Expr
}

// parseParameters parses a parenthesized parameter list.
func (p *Parser) parseParameters(typeParams bool) *ast.FieldList {
	lparen := p.expect(token.LPAREN)
	return p.parseParameterListAfterOpen(lparen, token.RPAREN, typeParams)
}

func (p *Parser) parseParamDecl(typeParam bool) paramEntry {
	parseTyp := p.parseType
	if typeParam {
		parseTyp = p.parseConstraint
	}
	switch p.tok {
	case token.IDENT:
		name := p.parseIdent()
		if typeParam {
			// type parameter lists always start with a name: [T any, K, V C]
			if p.tok == token.COMMA || p.tok == token.RBRACK {
				return paramEntry{nil, name}
			}
			return paramEntry{name, p.parseConstraint()}
		}
		switch p.tok {
		case token.IDENT, token.MUL, token.ARROW, token.FUNC, token.CHAN, token.MAP,
			token.STRUCT, token.INTERFACE, token.LPAREN, token.TILDE:
			return paramEntry{name, parseTyp()}
		case token.LBRACK:
			names, typ := p.parseArrayFieldOrTypeInstance(name)
			if names != nil {
				return paramEntry{names[0], typ}
			}
			return paramEntry{nil, typ}
		case token.ELLIPSIS:
			pos := p.pos
			p.next()
			return paramEntry{name, &ast.Ellipsis{EllipsisPos: pos, Elt: p.parseType()}}
		case token.PERIOD:
			return paramEntry{nil, p.parseQualifiedIdent(name)}
		}
		return paramEntry{nil, name}
	case token.ELLIPSIS:
		pos := p.pos
		p.next()
		return paramEntry{nil, &ast.Ellipsis{EllipsisPos: pos, Elt: p.parseType()}}
	}
	return paramEntry{nil, parseTyp()}
}

func (p *Parser) parseParameterListAfterOpen(open ast.Pos, close token.Token, typeParams bool) *ast.FieldList {
	var list []paramEntry
	named := false
	for p.tok != close && p.tok != token.EOF {
		e := p.parseParamDecl(typeParams)
		if e.name != nil {
			named = true
		}
		list = append(list, e)
		if !p.atComma("parameter list", close) {
			break
		}
		p.next()
	}
	p.expectClosing(close, "parameter list")
	fl := &ast.FieldList{Opening: open}
	if typeParams && len(list) == 0 {
		p.errorf(open, "syntax error: empty type parameter list")
	}
	if !named {
		if typeParams {
			p.errorf(open, "syntax error: type parameters must be named")
		}
		for _, e := range list {
			fl.List = append(fl.List, &ast.Field{Type: e.typ})
		}
		return fl
	}
	// named parameters: a, b int, c string
	var pending []*ast.Ident
	for _, e := range list {
		if e.name == nil {
			id, ok := e.typ.(*ast.Ident)
			if !ok {
				p.errorf(e.typ.Pos(), "syntax error: mixed named and unnamed parameters")
			}
			pending = append(pending, id)
			continue
		}
		fl.List = append(fl.List, &ast.Field{Names: append(pending, e.name), Type: e.typ})
		pending = nil
	}
	if len(pending) > 0 {
		p.errorf(pending[0].Pos(), "syntax error: mixed named and unnamed parameters")
	}
	return fl
}

// ---------------------------------------------------------------------------
// Expressions

func (p *Parser) parseExpr() ast.Expr {
	return p.parseBinaryExpr(nil, token.LowestPrec+1)
}

func (p *Parser) parseExprList() []ast.Expr {
	list := []ast.Expr{p.parseExpr()}
	for p.tok == token.COMMA {
		p.next()
		list = append(list, p.parseExpr())
	}
	return list
}

func (p *Parser) parseBinaryExpr(x ast.Expr, prec1 int) ast.Expr {
	if x == nil {
		x = p.parseUnaryExpr()
	}
	for {
		op, oprec := p.tok, p.tok.Precedence()
		if oprec < prec1 {
			return x
		}
		pos := p.pos
		p.next()
		y := p.parseBinaryExpr(nil, oprec+1)
		x = &ast.BinaryExpr{X: x, OpPos: pos, Op: op, Y: y}
	}
}

func (p *Parser) parseUnaryExpr() ast.Expr {
	switch p.tok {
	case token.ADD, token.SUB, token.NOT, token.XOR, token.AND, token.TILDE:
		pos, op := p.pos, p.tok
		p.next()
		return &ast.UnaryExpr{OpPos: pos, Op: op, X: p.parseUnaryExpr()}
	case token.ARROW:
		if p.peek(1).Kind == token.CHAN { // <-chan T
			return p.parsePrimaryExpr(p.parseChanType())
		}
		pos := p.pos
		p.next()
		return &ast.UnaryExpr{OpPos: pos, Op: token.ARROW, X: p.parseUnaryExpr()}
	case token.MUL:
		pos := p.pos
		p.next()
		return &ast.StarExpr{Star: pos, X: p.parseUnaryExpr()}
	}
	return p.parsePrimaryExpr(nil)
}

func (p *Parser) parseOperand() ast.Expr {
	switch p.tok {
	case token.IDENT:
		if p.lit == "new" && p.peek(1).Kind == token.IDENT {
			return p.parseNewExpr()
		}
		if p.lit == "super" {
			pos := p.pos
			p.next()
			return &ast.SuperExpr{SuperPos: pos}
		}
		return p.parseIdent()
	case token.INT, token.FLOAT, token.IMAG, token.CHAR, token.STRING:
		x := &ast.BasicLit{ValuePos: p.pos, Kind: p.tok, Value: p.lit}
		p.next()
		return x
	case token.LPAREN:
		lparen := p.pos
		p.next()
		p.exprLev++
		x := p.parseExpr()
		p.exprLev--
		p.expectClosing(token.RPAREN, "parenthesized expression")
		return &ast.ParenExpr{Lparen: lparen, X: x}
	case token.FUNC:
		pos := p.pos
		typ := p.parseFuncType()
		if p.tok != token.LBRACE {
			return typ
		}
		typ.Func = pos
		p.exprLev++
		body := p.parseBody()
		p.exprLev--
		return &ast.FuncLit{Type: typ, Body: body}
	}
	if typ := p.tryIdentOrType(); typ != nil {
		return typ
	}
	p.errorExpected("expression")
	return nil
}

// parseNewExpr parses the G++ constructor call: new Type[<T>](args).
func (p *Parser) parseNewExpr() ast.Expr {
	x := &ast.NewExpr{NewPos: p.pos}
	p.next() // new
	var typ ast.Expr = p.parseIdent()
	if p.tok == token.PERIOD {
		p.next()
		typ = &ast.SelectorExpr{X: typ, Sel: p.parseIdent()}
	}
	x.Type = typ
	x.TypeArgs, _ = p.parseOptTypeArgs()
	if p.tok != token.LPAREN {
		p.errorExpected("'(' after new " + exprName(typ))
	}
	x.Args, x.Ellipsis = p.parseCallArgs()
	return p.parsePrimaryExpr(x)
}

func exprName(x ast.Expr) string {
	switch x := x.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.SelectorExpr:
		return exprName(x.X) + "." + x.Sel.Name
	}
	return "type"
}

func (p *Parser) parseCallArgs() ([]ast.Expr, bool) {
	p.expect(token.LPAREN)
	p.exprLev++
	var args []ast.Expr
	ellipsis := false
	for p.tok != token.RPAREN && p.tok != token.EOF && !ellipsis {
		args = append(args, p.parseExpr())
		if p.tok == token.ELLIPSIS {
			ellipsis = true
			p.next()
		}
		if !p.atComma("argument list", token.RPAREN) {
			break
		}
		p.next()
	}
	p.exprLev--
	p.expectClosing(token.RPAREN, "argument list")
	return args, ellipsis
}

func (p *Parser) parsePrimaryExpr(x ast.Expr) ast.Expr {
	if x == nil {
		x = p.parseOperand()
	}
	for {
		switch p.tok {
		case token.PERIOD:
			p.next()
			switch p.tok {
			case token.IDENT:
				x = &ast.SelectorExpr{X: x, Sel: p.parseIdent()}
			case token.LPAREN:
				p.next()
				ta := &ast.TypeAssertExpr{X: x}
				if p.tok == token.TYPE {
					p.next()
				} else {
					ta.Type = p.parseType()
				}
				p.expect(token.RPAREN)
				x = ta
			default:
				p.errorExpected("selector or type assertion")
			}
		case token.LBRACK:
			x = p.parseIndexOrSlice(x)
		case token.LPAREN:
			lparen := p.pos
			args, ellipsis := p.parseCallArgs()
			x = &ast.CallExpr{Fun: x, Lparen: lparen, Args: args, Ellipsis: ellipsis}
		case token.LBRACE:
			switch t := unparen(x).(type) {
			case *ast.Ident, *ast.SelectorExpr, *ast.IndexExpr:
				if p.exprLev < 0 {
					return x
				}
			case *ast.ArrayType, *ast.StructType, *ast.MapType:
			default:
				_ = t
				return x
			}
			x = p.parseLiteralValue(x)
		default:
			return x
		}
	}
}

func unparen(x ast.Expr) ast.Expr {
	for {
		p, ok := x.(*ast.ParenExpr)
		if !ok {
			return x
		}
		x = p.X
	}
}

func (p *Parser) parseIndexOrSlice(x ast.Expr) ast.Expr {
	lbrack := p.expect(token.LBRACK)
	if p.tok == token.RBRACK {
		p.errorExpected("operand")
	}
	p.exprLev++
	var index [3]ast.Expr
	if p.tok != token.COLON {
		index[0] = p.parseExpr()
	}
	ncolons := 0
	switch p.tok {
	case token.COLON:
		for p.tok == token.COLON && ncolons < 2 {
			p.next()
			ncolons++
			if p.tok != token.COLON && p.tok != token.RBRACK && p.tok != token.EOF {
				index[ncolons] = p.parseExpr()
			}
		}
	case token.COMMA:
		args := []ast.Expr{index[0]}
		for p.tok == token.COMMA {
			p.next()
			if p.tok != token.RBRACK {
				args = append(args, p.parseType())
			}
		}
		p.exprLev--
		p.expectClosing(token.RBRACK, "type argument list")
		return &ast.IndexExpr{X: x, Lbrack: lbrack, Indices: args}
	}
	p.exprLev--
	p.expect(token.RBRACK)
	if ncolons > 0 {
		s := &ast.SliceExpr{X: x, Lbrack: lbrack, Low: index[0], High: index[1], Max: index[2], Slice3: ncolons == 2}
		if s.Slice3 && (s.High == nil || s.Max == nil) {
			p.errorf(lbrack, "syntax error: middle and final index required in 3-index slice")
		}
		return s
	}
	return &ast.IndexExpr{X: x, Lbrack: lbrack, Indices: []ast.Expr{index[0]}}
}

func (p *Parser) parseLiteralValue(typ ast.Expr) ast.Expr {
	lbrace := p.expect(token.LBRACE)
	lit := &ast.CompositeLit{Type: typ, Lbrace: lbrace}
	p.exprLev++
	for p.tok != token.RBRACE && p.tok != token.EOF {
		lit.Elts = append(lit.Elts, p.parseElement())
		if !p.atComma("composite literal", token.RBRACE) {
			break
		}
		p.next()
	}
	p.exprLev--
	p.expectClosing(token.RBRACE, "composite literal")
	return lit
}

func (p *Parser) parseElementValue() ast.Expr {
	if p.tok == token.LBRACE {
		return p.parseLiteralValue(nil)
	}
	return p.parseExpr()
}

func (p *Parser) parseElement() ast.Expr {
	x := p.parseElementValue()
	if p.tok == token.COLON {
		colon := p.pos
		p.next()
		x = &ast.KeyValueExpr{Key: x, Colon: colon, Value: p.parseElementValue()}
	}
	return x
}
