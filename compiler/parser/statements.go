package parser

import (
	"go/token"

	"github.com/raptordnj/gpp/compiler/ast"
)

type stmtMode int

const (
	basic stmtMode = iota
	labelOk
	rangeOk
)

func (p *Parser) parseBody() *ast.BlockStmt {
	return p.parseBlockStmt()
}

func (p *Parser) parseBlockStmt() *ast.BlockStmt {
	lbrace := p.expect(token.LBRACE)
	list := p.parseStmtList()
	p.expectClosing(token.RBRACE, "block")
	return &ast.BlockStmt{Lbrace: lbrace, List: list}
}

func (p *Parser) parseStmtList() []ast.Stmt {
	var list []ast.Stmt
	for p.tok != token.CASE && p.tok != token.DEFAULT && p.tok != token.RBRACE && p.tok != token.EOF {
		list = append(list, p.parseStmt())
	}
	return list
}

func (p *Parser) parseStmt() ast.Stmt {
	switch p.tok {
	case token.CONST, token.VAR:
		return &ast.DeclStmt{Decl: p.parseGenDecl(p.tok, p.parseValueSpec)}
	case token.TYPE:
		return &ast.DeclStmt{Decl: p.parseGenDecl(token.TYPE, p.parseTypeSpec)}
	case token.IDENT, token.INT, token.FLOAT, token.IMAG, token.CHAR, token.STRING, token.FUNC, token.LPAREN,
		token.LBRACK, token.STRUCT, token.MAP, token.CHAN, token.INTERFACE,
		token.ADD, token.SUB, token.MUL, token.AND, token.XOR, token.ARROW, token.NOT:
		s, _ := p.parseSimpleStmt(labelOk)
		if _, isLabeled := s.(*ast.LabeledStmt); !isLabeled {
			p.expectSemi()
		}
		return s
	case token.GO:
		pos := p.pos
		p.next()
		call := p.parseCallStmtExpr("go")
		p.expectSemi()
		return &ast.GoStmt{Go: pos, Call: call}
	case token.DEFER:
		pos := p.pos
		p.next()
		call := p.parseCallStmtExpr("defer")
		p.expectSemi()
		return &ast.DeferStmt{Defer: pos, Call: call}
	case token.RETURN:
		s := &ast.ReturnStmt{Return: p.pos}
		p.next()
		if p.tok != token.SEMICOLON && p.tok != token.RBRACE {
			s.Results = p.parseExprList()
		}
		p.expectSemi()
		return s
	case token.BREAK, token.CONTINUE, token.GOTO, token.FALLTHROUGH:
		s := &ast.BranchStmt{TokPos: p.pos, Tok: p.tok}
		p.next()
		if s.Tok != token.FALLTHROUGH && p.tok == token.IDENT {
			s.Label = p.parseIdent()
		}
		p.expectSemi()
		return s
	case token.LBRACE:
		s := p.parseBlockStmt()
		p.expectSemi()
		return s
	case token.IF:
		return p.parseIfStmt()
	case token.SWITCH:
		return p.parseSwitchStmt()
	case token.SELECT:
		return p.parseSelectStmt()
	case token.FOR:
		return p.parseForStmt()
	case token.SEMICOLON:
		s := &ast.EmptyStmt{Semicolon: p.pos}
		p.next()
		return s
	case token.RBRACE:
		return &ast.EmptyStmt{Semicolon: p.pos}
	}
	p.errorExpected("statement")
	return nil
}

func (p *Parser) parseCallStmtExpr(what string) *ast.CallExpr {
	x := p.parseExpr()
	call, ok := unparen(x).(*ast.CallExpr)
	if !ok {
		p.errorf(x.Pos(), "expression in %s must be function call", what)
	}
	return call
}

func (p *Parser) parseSimpleStmt(mode stmtMode) (ast.Stmt, bool) {
	x := p.parseExprList()
	switch p.tok {
	case token.DEFINE, token.ASSIGN, token.ADD_ASSIGN, token.SUB_ASSIGN, token.MUL_ASSIGN,
		token.QUO_ASSIGN, token.REM_ASSIGN, token.AND_ASSIGN, token.OR_ASSIGN, token.XOR_ASSIGN,
		token.SHL_ASSIGN, token.SHR_ASSIGN, token.AND_NOT_ASSIGN:
		pos, tok := p.pos, p.tok
		p.next()
		var y []ast.Expr
		isRange := false
		if mode == rangeOk && p.tok == token.RANGE && (tok == token.DEFINE || tok == token.ASSIGN) {
			rpos := p.pos
			p.next()
			y = []ast.Expr{&ast.UnaryExpr{OpPos: rpos, Op: token.RANGE, X: p.parseExpr()}}
			isRange = true
		} else {
			y = p.parseExprList()
		}
		return &ast.AssignStmt{Lhs: x, TokPos: pos, Tok: tok, Rhs: y}, isRange
	}
	if len(x) > 1 {
		p.errorf(x[0].Pos(), "syntax error: expected 1 expression")
	}
	switch p.tok {
	case token.COLON:
		if label, ok := x[0].(*ast.Ident); mode == labelOk && ok {
			p.next()
			if p.tok == token.RBRACE {
				return &ast.LabeledStmt{Label: label, Stmt: &ast.EmptyStmt{Semicolon: p.pos}}, false
			}
			return &ast.LabeledStmt{Label: label, Stmt: p.parseStmt()}, false
		}
	case token.ARROW:
		arrow := p.pos
		p.next()
		return &ast.SendStmt{Chan: x[0], Arrow: arrow, Value: p.parseExpr()}, false
	case token.INC, token.DEC:
		s := &ast.IncDecStmt{X: x[0], Tok: p.tok}
		p.next()
		return s, false
	}
	return &ast.ExprStmt{X: x[0]}, false
}

func (p *Parser) makeExpr(s ast.Stmt, want string) ast.Expr {
	if s == nil {
		return nil
	}
	if es, ok := s.(*ast.ExprStmt); ok {
		return es.X
	}
	found := "simple statement"
	if _, ok := s.(*ast.AssignStmt); ok {
		found = "assignment"
	}
	p.errorf(s.Pos(), "syntax error: expected %s, found %s", want, found)
	return nil
}

func (p *Parser) parseIfStmt() *ast.IfStmt {
	s := &ast.IfStmt{If: p.expect(token.IF)}
	if p.tok == token.LBRACE {
		p.errorf(p.pos, "syntax error: missing condition in if statement")
	}
	prevLev := p.exprLev
	p.exprLev = -1
	var init ast.Stmt
	if p.tok != token.SEMICOLON {
		init, _ = p.parseSimpleStmt(basic)
	}
	var condStmt ast.Stmt
	if p.tok == token.SEMICOLON {
		p.next()
		if p.tok == token.LBRACE {
			p.errorf(p.pos, "syntax error: missing condition in if statement")
		}
		condStmt, _ = p.parseSimpleStmt(basic)
	} else {
		condStmt, init = init, nil
	}
	s.Init = init
	s.Cond = p.makeExpr(condStmt, "boolean expression")
	p.exprLev = prevLev

	s.Body = p.parseBlockStmt()
	if p.tok == token.ELSE {
		p.next()
		switch p.tok {
		case token.IF:
			s.Else = p.parseIfStmt()
		case token.LBRACE:
			s.Else = p.parseBlockStmt()
			p.expectSemi()
		default:
			p.errorExpected("if statement or block")
		}
	} else {
		p.expectSemi()
	}
	return s
}

func isTypeSwitchGuard(s ast.Stmt) bool {
	switch t := s.(type) {
	case *ast.ExprStmt:
		ta, ok := t.X.(*ast.TypeAssertExpr)
		return ok && ta.Type == nil
	case *ast.AssignStmt:
		if t.Tok == token.DEFINE && len(t.Lhs) == 1 && len(t.Rhs) == 1 {
			ta, ok := t.Rhs[0].(*ast.TypeAssertExpr)
			return ok && ta.Type == nil
		}
	}
	return false
}

func (p *Parser) parseSwitchStmt() ast.Stmt {
	pos := p.expect(token.SWITCH)
	var s1, s2 ast.Stmt
	if p.tok != token.LBRACE {
		prevLev := p.exprLev
		p.exprLev = -1
		if p.tok != token.SEMICOLON {
			s2, _ = p.parseSimpleStmt(basic)
		}
		if p.tok == token.SEMICOLON {
			p.next()
			s1, s2 = s2, nil
			if p.tok != token.LBRACE {
				s2, _ = p.parseSimpleStmt(basic)
			}
		}
		p.exprLev = prevLev
	}
	typeSwitch := isTypeSwitchGuard(s2)
	lbrace := p.expect(token.LBRACE)
	body := &ast.BlockStmt{Lbrace: lbrace}
	for p.tok == token.CASE || p.tok == token.DEFAULT {
		cc := &ast.CaseClause{Case: p.pos}
		if p.tok == token.CASE {
			p.next()
			cc.List = p.parseExprList()
		} else {
			p.next()
		}
		p.expect(token.COLON)
		cc.Body = p.parseStmtList()
		body.List = append(body.List, cc)
	}
	p.expect(token.RBRACE)
	p.expectSemi()
	if typeSwitch {
		return &ast.TypeSwitchStmt{Switch: pos, Init: s1, Assign: s2, Body: body}
	}
	return &ast.SwitchStmt{Switch: pos, Init: s1, Tag: p.makeExpr(s2, "switch expression"), Body: body}
}

func (p *Parser) parseSelectStmt() *ast.SelectStmt {
	pos := p.expect(token.SELECT)
	lbrace := p.expect(token.LBRACE)
	body := &ast.BlockStmt{Lbrace: lbrace}
	for p.tok == token.CASE || p.tok == token.DEFAULT {
		cc := &ast.CommClause{Case: p.pos}
		if p.tok == token.CASE {
			p.next()
			lhs := p.parseExprList()
			switch p.tok {
			case token.ARROW:
				if len(lhs) > 1 {
					p.errorf(lhs[0].Pos(), "syntax error: expected 1 expression")
				}
				arrow := p.pos
				p.next()
				cc.Comm = &ast.SendStmt{Chan: lhs[0], Arrow: arrow, Value: p.parseExpr()}
			case token.ASSIGN, token.DEFINE:
				tpos, tok := p.pos, p.tok
				p.next()
				cc.Comm = &ast.AssignStmt{Lhs: lhs, TokPos: tpos, Tok: tok, Rhs: []ast.Expr{p.parseExpr()}}
			default:
				if len(lhs) > 1 {
					p.errorf(lhs[0].Pos(), "syntax error: expected 1 expression")
				}
				cc.Comm = &ast.ExprStmt{X: lhs[0]}
			}
		} else {
			p.next()
		}
		p.expect(token.COLON)
		cc.Body = p.parseStmtList()
		body.List = append(body.List, cc)
	}
	p.expect(token.RBRACE)
	p.expectSemi()
	return &ast.SelectStmt{Select: pos, Body: body}
}

func (p *Parser) parseForStmt() ast.Stmt {
	pos := p.expect(token.FOR)
	var s1, s2, s3 ast.Stmt
	isRange := false
	if p.tok != token.LBRACE {
		prevLev := p.exprLev
		p.exprLev = -1
		if p.tok != token.SEMICOLON {
			if p.tok == token.RANGE {
				rpos := p.pos
				p.next()
				y := []ast.Expr{&ast.UnaryExpr{OpPos: rpos, Op: token.RANGE, X: p.parseExpr()}}
				s2 = &ast.AssignStmt{Rhs: y, Tok: token.ILLEGAL, TokPos: rpos}
				isRange = true
			} else {
				s2, isRange = p.parseSimpleStmt(rangeOk)
			}
		}
		if !isRange && p.tok == token.SEMICOLON {
			p.next()
			s1, s2 = s2, nil
			if p.tok != token.SEMICOLON {
				s2, _ = p.parseSimpleStmt(basic)
			}
			if p.tok != token.SEMICOLON {
				p.errorExpected("';' in for clause")
			}
			p.next()
			if p.tok != token.LBRACE {
				s3, _ = p.parseSimpleStmt(basic)
			}
		}
		p.exprLev = prevLev
	}
	body := p.parseBlockStmt()
	p.expectSemi()

	if isRange {
		as := s2.(*ast.AssignStmt)
		rs := &ast.RangeStmt{For: pos, TokPos: as.TokPos, Tok: as.Tok, X: as.Rhs[0].(*ast.UnaryExpr).X, Body: body}
		switch len(as.Lhs) {
		case 0:
		case 1:
			rs.Key = as.Lhs[0]
		case 2:
			rs.Key, rs.Value = as.Lhs[0], as.Lhs[1]
		default:
			p.errorf(as.Lhs[len(as.Lhs)-1].Pos(), "syntax error: range clause permits at most two iteration variables")
		}
		return rs
	}
	return &ast.ForStmt{For: pos, Init: s1, Cond: p.makeExpr(s2, "for loop condition"), Post: s3, Body: body}
}
