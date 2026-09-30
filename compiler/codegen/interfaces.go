package codegen

import (
	goast "go/ast"
	"go/token"

	"gpp/compiler/ast"
)

// interfaceDecl lowers "interface I { function M() }" to
// "type I interface { M() }".
func (g *Generator) interfaceDecl(d *ast.InterfaceDecl) goast.Decl {
	methods := g.fieldList(d.Methods)
	if methods == nil {
		methods = &goast.FieldList{}
	}
	spec := &goast.TypeSpec{
		Name:       g.ident(d.Name),
		TypeParams: g.fieldList(d.TypeParams),
		Type:       &goast.InterfaceType{Interface: g.pos(d.InterfacePos), Methods: methods},
	}
	return &goast.GenDecl{TokPos: g.pos(d.InterfacePos), Tok: token.TYPE, Specs: []goast.Spec{spec}}
}

// EnumMemberName is the generated constant name of an enum member.
func EnumMemberName(enum, member string) string { return enum + member }

// enumDecl lowers an enum to a named int type, an iota constant block and a
// String method.
func (g *Generator) enumDecl(d *ast.EnumDecl) []goast.Decl {
	pos := g.pos(d.EnumPos)
	name := d.Name.Name
	typeDecl := &goast.GenDecl{TokPos: pos, Tok: token.TYPE, Specs: []goast.Spec{&goast.TypeSpec{
		Name: g.ident(d.Name),
		Type: newIdent("int", pos),
	}}}

	consts := &goast.GenDecl{TokPos: pos, Tok: token.CONST, Lparen: pos, Rparen: pos}
	var cases []goast.Stmt
	for i, m := range d.Members {
		spec := &goast.ValueSpec{Names: []*goast.Ident{newIdent(EnumMemberName(name, m.Name), g.pos(m.Pos()))}}
		if i == 0 {
			spec.Type = newIdent(name, pos)
			spec.Values = []goast.Expr{newIdent("iota", pos)}
		}
		consts.Specs = append(consts.Specs, spec)
		cases = append(cases, &goast.CaseClause{
			List: []goast.Expr{newIdent(EnumMemberName(name, m.Name), pos)},
			Body: []goast.Stmt{&goast.ReturnStmt{Results: []goast.Expr{stringLit(m.Name)}}},
		})
	}

	str := &goast.FuncDecl{
		Recv: &goast.FieldList{List: []*goast.Field{{Names: []*goast.Ident{newIdent("e", pos)}, Type: newIdent(name, pos)}}},
		Name: newIdent("String", pos),
		Type: &goast.FuncType{Params: &goast.FieldList{}, Results: &goast.FieldList{List: []*goast.Field{{Type: newIdent("string", pos)}}}},
		Body: &goast.BlockStmt{List: []goast.Stmt{
			&goast.SwitchStmt{Tag: newIdent("e", pos), Body: &goast.BlockStmt{List: cases}},
			&goast.ReturnStmt{Results: []goast.Expr{stringLit(name + "(invalid)")}},
		}},
	}
	return []goast.Decl{typeDecl, consts, str}
}
