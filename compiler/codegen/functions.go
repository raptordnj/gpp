package codegen

import (
	"fmt"
	goast "go/ast"
	"go/token"

	"gpp/compiler/ast"
)

// ---------------------------------------------------------------------------
// Expressions

func (g *Generator) exprOrNil(x ast.Expr) goast.Expr {
	if x == nil {
		return nil
	}
	return g.expr(x)
}

func (g *Generator) exprs(xs []ast.Expr) []goast.Expr {
	var out []goast.Expr
	for _, x := range xs {
		out = append(out, g.expr(x))
	}
	return out
}

// index builds x[i] or x[i, j] (generic instantiation).
func index(x goast.Expr, lbrack token.Pos, indices []goast.Expr) goast.Expr {
	if len(indices) == 1 {
		return &goast.IndexExpr{X: x, Lbrack: lbrack, Index: indices[0], Rbrack: lbrack}
	}
	return &goast.IndexListExpr{X: x, Lbrack: lbrack, Indices: indices, Rbrack: lbrack}
}

func (g *Generator) expr(x ast.Expr) goast.Expr {
	switch x := x.(type) {
	case *ast.Ident:
		return g.ident(x)
	case *ast.BasicLit:
		return g.basicLit(x)
	case *ast.CompositeLit:
		return &goast.CompositeLit{Type: g.exprOrNil(x.Type), Lbrace: g.pos(x.Lbrace), Elts: g.exprs(x.Elts), Rbrace: g.pos(x.Lbrace)}
	case *ast.FuncLit:
		return &goast.FuncLit{Type: g.funcType(x.Type), Body: g.block(x.Body)}
	case *ast.ParenExpr:
		return &goast.ParenExpr{Lparen: g.pos(x.Lparen), X: g.expr(x.X)}
	case *ast.SelectorExpr:
		if name, ok := g.prog.StaticRefs[x]; ok {
			return newIdent(name, g.pos(x.X.Pos()))
		}
		if sup, ok := x.X.(*ast.SuperExpr); ok {
			return &goast.SelectorExpr{X: g.superRef(sup), Sel: g.ident(x.Sel)}
		}
		return &goast.SelectorExpr{X: g.expr(x.X), Sel: g.ident(x.Sel)}
	case *ast.IndexExpr:
		return index(g.expr(x.X), g.pos(x.Lbrack), g.exprs(x.Indices))
	case *ast.SliceExpr:
		return &goast.SliceExpr{X: g.expr(x.X), Lbrack: g.pos(x.Lbrack), Low: g.exprOrNil(x.Low),
			High: g.exprOrNil(x.High), Max: g.exprOrNil(x.Max), Slice3: x.Slice3}
	case *ast.TypeAssertExpr:
		return &goast.TypeAssertExpr{X: g.expr(x.X), Type: g.exprOrNil(x.Type)}
	case *ast.CallExpr:
		call := &goast.CallExpr{Fun: g.expr(x.Fun), Lparen: g.pos(x.Lparen), Args: g.exprs(x.Args)}
		if x.Ellipsis {
			call.Ellipsis = g.pos(x.Lparen)
			if call.Ellipsis == token.NoPos {
				call.Ellipsis = 1
			}
		}
		return call
	case *ast.StarExpr:
		return &goast.StarExpr{Star: g.pos(x.Star), X: g.expr(x.X)}
	case *ast.UnaryExpr:
		return &goast.UnaryExpr{OpPos: g.pos(x.OpPos), Op: x.Op, X: g.expr(x.X)}
	case *ast.BinaryExpr:
		return &goast.BinaryExpr{X: g.expr(x.X), OpPos: g.pos(x.OpPos), Op: x.Op, Y: g.expr(x.Y)}
	case *ast.KeyValueExpr:
		return &goast.KeyValueExpr{Key: g.expr(x.Key), Colon: g.pos(x.Colon), Value: g.expr(x.Value)}
	case *ast.NewExpr:
		return g.newExpr(x)
	case *ast.SuperExpr:
		return g.superRef(x)
	case *ast.ArrayType:
		return &goast.ArrayType{Lbrack: g.pos(x.Lbrack), Len: g.exprOrNil(x.Len), Elt: g.expr(x.Elt)}
	case *ast.StructType:
		fl := g.fieldList(x.Fields)
		if fl == nil {
			fl = &goast.FieldList{}
		}
		return &goast.StructType{Struct: g.pos(x.Struct), Fields: fl}
	case *ast.FuncType:
		return g.funcType(x)
	case *ast.InterfaceType:
		fl := g.fieldList(x.Methods)
		if fl == nil {
			fl = &goast.FieldList{}
		}
		return &goast.InterfaceType{Interface: g.pos(x.Interface), Methods: fl}
	case *ast.MapType:
		return &goast.MapType{Map: g.pos(x.Map), Key: g.expr(x.Key), Value: g.expr(x.Value)}
	case *ast.ChanType:
		return &goast.ChanType{Begin: g.pos(x.Begin), Dir: goast.ChanDir(x.Dir), Value: g.expr(x.Value)}
	case *ast.Ellipsis:
		return &goast.Ellipsis{Ellipsis: g.pos(x.EllipsisPos), Elt: g.exprOrNil(x.Elt)}
	case *ast.BadExpr:
		return &goast.BadExpr{From: g.pos(x.From)}
	}
	panic(fmt.Sprintf("codegen: unexpected expression %T", x))
}

// newExpr lowers "new C(args)" to "NewC(args)" and "new pkg.C(args)" to
// "pkg.NewC(args)"; generic type arguments are kept as Go instantiations.
func (g *Generator) newExpr(x *ast.NewExpr) goast.Expr {
	pos := g.pos(x.NewPos)
	var fun goast.Expr
	switch t := x.Type.(type) {
	case *ast.Ident:
		fun = newIdent(ConstructorName(t.Name), pos)
	case *ast.SelectorExpr:
		fun = &goast.SelectorExpr{X: g.expr(t.X), Sel: newIdent(ConstructorName(t.Sel.Name), g.pos(t.Sel.Pos()))}
	}
	if len(x.TypeArgs) > 0 {
		fun = index(fun, pos, g.exprs(x.TypeArgs))
	}
	call := &goast.CallExpr{Fun: fun, Lparen: pos, Args: g.exprs(x.Args)}
	if x.Ellipsis {
		call.Ellipsis = pos
	}
	return call
}

// ConstructorName is the generated Go constructor function for a class.
func ConstructorName(class string) string { return "New" + class }

// superRef lowers the object designated by super: this.<Parent>.
func (g *Generator) superRef(s *ast.SuperExpr) goast.Expr {
	pos := g.pos(s.SuperPos)
	parent := "super"
	if g.class != nil && g.class.ParentName != "" {
		parent = g.class.ParentName
	}
	return &goast.SelectorExpr{X: newIdent("this", pos), Sel: newIdent(parent, pos)}
}

// ---------------------------------------------------------------------------
// Statements

func (g *Generator) blockOrNil(b *ast.BlockStmt) *goast.BlockStmt {
	if b == nil {
		return nil
	}
	return g.block(b)
}

func (g *Generator) block(b *ast.BlockStmt) *goast.BlockStmt {
	return &goast.BlockStmt{Lbrace: g.pos(b.Lbrace), List: g.stmts(b.List)}
}

func (g *Generator) stmts(list []ast.Stmt) []goast.Stmt {
	var out []goast.Stmt
	for _, s := range list {
		out = append(out, g.stmt(s))
	}
	return out
}

func (g *Generator) stmtOrNil(s ast.Stmt) goast.Stmt {
	if s == nil {
		return nil
	}
	return g.stmt(s)
}

func (g *Generator) stmt(s ast.Stmt) goast.Stmt {
	switch s := s.(type) {
	case *ast.BadStmt:
		return &goast.BadStmt{From: g.pos(s.From)}
	case *ast.DeclStmt:
		return &goast.DeclStmt{Decl: g.genDecl(s.Decl)}
	case *ast.EmptyStmt:
		return &goast.EmptyStmt{Semicolon: g.pos(s.Semicolon), Implicit: true}
	case *ast.LabeledStmt:
		return &goast.LabeledStmt{Label: g.ident(s.Label), Stmt: g.stmt(s.Stmt)}
	case *ast.ExprStmt:
		return &goast.ExprStmt{X: g.expr(s.X)}
	case *ast.SendStmt:
		return &goast.SendStmt{Chan: g.expr(s.Chan), Arrow: g.pos(s.Arrow), Value: g.expr(s.Value)}
	case *ast.IncDecStmt:
		return &goast.IncDecStmt{X: g.expr(s.X), Tok: s.Tok}
	case *ast.AssignStmt:
		return &goast.AssignStmt{Lhs: g.exprs(s.Lhs), TokPos: g.pos(s.TokPos), Tok: s.Tok, Rhs: g.exprs(s.Rhs)}
	case *ast.GoStmt:
		return &goast.GoStmt{Go: g.pos(s.Go), Call: g.expr(s.Call).(*goast.CallExpr)}
	case *ast.DeferStmt:
		return &goast.DeferStmt{Defer: g.pos(s.Defer), Call: g.expr(s.Call).(*goast.CallExpr)}
	case *ast.ReturnStmt:
		return &goast.ReturnStmt{Return: g.pos(s.Return), Results: g.exprs(s.Results)}
	case *ast.BranchStmt:
		out := &goast.BranchStmt{TokPos: g.pos(s.TokPos), Tok: s.Tok}
		if s.Label != nil {
			out.Label = g.ident(s.Label)
		}
		return out
	case *ast.BlockStmt:
		return g.block(s)
	case *ast.IfStmt:
		return &goast.IfStmt{If: g.pos(s.If), Init: g.stmtOrNil(s.Init), Cond: g.expr(s.Cond), Body: g.block(s.Body), Else: g.stmtOrNil(s.Else)}
	case *ast.CaseClause:
		return &goast.CaseClause{Case: g.pos(s.Case), List: g.exprs(s.List), Body: g.stmts(s.Body)}
	case *ast.SwitchStmt:
		return &goast.SwitchStmt{Switch: g.pos(s.Switch), Init: g.stmtOrNil(s.Init), Tag: g.exprOrNil(s.Tag), Body: g.block(s.Body)}
	case *ast.TypeSwitchStmt:
		return &goast.TypeSwitchStmt{Switch: g.pos(s.Switch), Init: g.stmtOrNil(s.Init), Assign: g.stmt(s.Assign), Body: g.block(s.Body)}
	case *ast.CommClause:
		return &goast.CommClause{Case: g.pos(s.Case), Comm: g.stmtOrNil(s.Comm), Body: g.stmts(s.Body)}
	case *ast.SelectStmt:
		return &goast.SelectStmt{Select: g.pos(s.Select), Body: g.block(s.Body)}
	case *ast.ForStmt:
		return &goast.ForStmt{For: g.pos(s.For), Init: g.stmtOrNil(s.Init), Cond: g.exprOrNil(s.Cond), Post: g.stmtOrNil(s.Post), Body: g.block(s.Body)}
	case *ast.RangeStmt:
		return &goast.RangeStmt{For: g.pos(s.For), Key: g.exprOrNil(s.Key), Value: g.exprOrNil(s.Value),
			TokPos: g.pos(s.TokPos), Tok: s.Tok, Range: g.pos(s.TokPos), X: g.expr(s.X), Body: g.block(s.Body)}
	}
	panic(fmt.Sprintf("codegen: unexpected statement %T", s))
}
