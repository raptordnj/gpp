package codegen

import (
	"fmt"
	goast "go/ast"
	"go/token"

	"gpp/compiler/ast"
	"gpp/compiler/semantic"
)

// Class lowering strategy
//
//	class C extends P { fields; constructor(...) {...}; methods }
//
// becomes
//
//	type C struct { P; fields }            // inheritance = embedding
//	func NewC(params) *C                   // allocation + init
//	func (this *C) initC(params)           // super(...), field initializers, constructor body
//	func (this *C) Method(...)             // pointer receivers named "this"
//	func C_Static(...), var C_Field        // static members at package level
//
// The init method exists only when construction needs code (a constructor,
// field initializers or a parent that needs init); otherwise NewC simply
// returns &C{}. Abstract classes get no NewC.

// InitName is the generated name of a class's init method.
func InitName(class string) string { return "init" + class }

func (g *Generator) classDecl(d *ast.ClassDecl) []goast.Decl {
	c := g.prog.Classes[d.Name.Name]
	if c == nil || c.Decl != d {
		return nil // duplicate declaration; already reported
	}
	g.class = c
	defer func() { g.class = nil }()

	var decls []goast.Decl
	decls = append(decls, g.classStruct(c))

	// static fields
	for _, f := range d.Fields {
		if !f.Mods.Static {
			continue
		}
		for _, n := range f.Names {
			spec := &goast.ValueSpec{
				Names: []*goast.Ident{newIdent(semantic.StaticName(c.Name, n.Name), g.pos(n.Pos()))},
				Type:  g.expr(f.Type),
			}
			if f.Value != nil {
				spec.Values = []goast.Expr{g.expr(f.Value)}
			}
			decls = append(decls, &goast.GenDecl{TokPos: g.pos(n.Pos()), Tok: token.VAR, Specs: []goast.Spec{spec}})
		}
	}

	if !d.Abstract {
		decls = append(decls, g.record(g.constructorFunc(c), c))
	}
	if c.NeedsInit() {
		decls = append(decls, g.record(g.initMethod(c), c))
	}
	for _, p := range d.Properties {
		for _, fd := range g.propertyAccessors(c, p) {
			decls = append(decls, g.record(fd, c))
		}
	}
	for _, m := range d.Methods {
		decls = append(decls, g.record(g.method(c, m), c))
	}
	if !c.IsGeneric() {
		for _, impl := range d.Implements {
			decls = append(decls, g.implementsAssertion(c, impl))
		}
	}
	return decls
}

func (g *Generator) record(fd *goast.FuncDecl, c *semantic.Class) *goast.FuncDecl {
	g.info.FuncClass[fd] = c
	return fd
}

// selfType returns C or C[T1, T2] for generic classes.
func (g *Generator) selfType(c *semantic.Class, pos token.Pos) goast.Expr {
	var t goast.Expr = newIdent(c.Name, pos)
	if tp := c.Decl.TypeParams; tp != nil {
		var args []goast.Expr
		for _, f := range tp.List {
			for _, n := range f.Names {
				args = append(args, newIdent(n.Name, pos))
			}
		}
		t = index(t, pos, args)
	}
	return t
}

func (g *Generator) receiver(c *semantic.Class, pos token.Pos) *goast.FieldList {
	return &goast.FieldList{List: []*goast.Field{{
		Names: []*goast.Ident{newIdent("this", pos)},
		Type:  &goast.StarExpr{Star: pos, X: g.selfType(c, pos)},
	}}}
}

func (g *Generator) classStruct(c *semantic.Class) *goast.GenDecl {
	d := c.Decl
	fields := &goast.FieldList{}
	if d.Extends != nil {
		fields.List = append(fields.List, &goast.Field{Type: g.expr(d.Extends)})
	}
	for _, f := range d.Fields {
		if f.Mods.Static {
			continue
		}
		fields.List = append(fields.List, &goast.Field{Names: g.idents(f.Names), Type: g.expr(f.Type)})
	}
	// Properties are represented by temporary fields while type checking so
	// that accesses can be resolved; RewriteProperties removes them.
	for _, p := range d.Properties {
		pf := &goast.Field{Names: []*goast.Ident{g.ident(p.Name)}, Type: g.expr(p.Type)}
		fields.List = append(fields.List, pf)
		if g.info.PropertyFields[c] == nil {
			g.info.PropertyFields[c] = map[string]*goast.Field{}
		}
		g.info.PropertyFields[c][p.Name.Name] = pf
	}
	spec := &goast.TypeSpec{
		Name:       g.ident(d.Name),
		TypeParams: g.fieldList(d.TypeParams),
		Type:       &goast.StructType{Struct: g.pos(d.ClassPos), Fields: fields},
	}
	return &goast.GenDecl{TokPos: g.pos(d.ClassPos), Tok: token.TYPE, Specs: []goast.Spec{spec}}
}

// ctorParams returns the lowered constructor parameters (with every
// parameter named) and the argument expressions forwarding them.
func (g *Generator) ctorParams(c *semantic.Class) (*goast.FieldList, []goast.Expr, bool) {
	params := &goast.FieldList{}
	var args []goast.Expr
	variadic := false
	ctor := c.Decl.Ctor
	if ctor == nil {
		return params, nil, false
	}
	n := 0
	for _, f := range ctor.Type.Params.List {
		field := &goast.Field{Type: g.expr(f.Type)}
		names := f.Names
		if len(names) == 0 {
			names = []*ast.Ident{{NamePos: f.Type.Pos(), Name: "_"}}
		}
		for _, id := range names {
			name := id.Name
			if name == "_" {
				name = fmt.Sprintf("_p%d", n)
			}
			n++
			field.Names = append(field.Names, newIdent(name, g.pos(id.Pos())))
			args = append(args, newIdent(name, token.NoPos))
		}
		_, variadic = f.Type.(*ast.Ellipsis)
		params.List = append(params.List, field)
	}
	return params, args, variadic
}

// constructorFunc generates NewC.
func (g *Generator) constructorFunc(c *semantic.Class) *goast.FuncDecl {
	d := c.Decl
	pos := g.pos(d.Name.Pos())
	if d.Ctor != nil {
		pos = g.pos(d.Ctor.ConstructorPos)
	}
	params, args, variadic := g.ctorParams(c)
	alloc := &goast.UnaryExpr{Op: token.AND, X: &goast.CompositeLit{Type: g.selfType(c, pos)}}
	var body []goast.Stmt
	if c.NeedsInit() {
		call := &goast.CallExpr{Fun: &goast.SelectorExpr{X: newIdent("this", pos), Sel: newIdent(InitName(c.Name), pos)}, Args: args}
		if variadic {
			call.Ellipsis = pos
		}
		body = []goast.Stmt{
			&goast.AssignStmt{Lhs: []goast.Expr{newIdent("this", pos)}, Tok: token.DEFINE, Rhs: []goast.Expr{alloc}},
			&goast.ExprStmt{X: call},
			&goast.ReturnStmt{Results: []goast.Expr{newIdent("this", pos)}},
		}
	} else {
		body = []goast.Stmt{&goast.ReturnStmt{Results: []goast.Expr{alloc}}}
	}
	return &goast.FuncDecl{
		Name: newIdent(ConstructorName(c.Name), pos),
		Type: &goast.FuncType{
			Func:       pos,
			TypeParams: g.fieldList(d.TypeParams),
			Params:     params,
			Results:    &goast.FieldList{List: []*goast.Field{{Type: &goast.StarExpr{X: g.selfType(c, pos)}}}},
		},
		Body: &goast.BlockStmt{List: body},
	}
}

// initMethod generates initC: parent initialization, field initializers,
// then the constructor body.
func (g *Generator) initMethod(c *semantic.Class) *goast.FuncDecl {
	d := c.Decl
	pos := g.pos(d.Name.Pos())
	if d.Ctor != nil {
		pos = g.pos(d.Ctor.ConstructorPos)
	}
	params, _, _ := g.ctorParams(c)
	var body []goast.Stmt
	var rest []ast.Stmt
	if d.Ctor != nil {
		rest = d.Ctor.Body.List
	}
	sc := semantic.SuperCall(d.Ctor)
	if sc != nil {
		rest = rest[1:]
	}
	if s := g.superInit(c, sc); s != nil {
		body = append(body, s)
	}
	for _, f := range d.Fields {
		if f.Mods.Static || f.Value == nil {
			continue
		}
		fpos := g.pos(f.Names[0].Pos())
		body = append(body, &goast.AssignStmt{
			Lhs: []goast.Expr{&goast.SelectorExpr{X: newIdent("this", fpos), Sel: newIdent(f.Names[0].Name, fpos)}},
			Tok: token.ASSIGN,
			Rhs: []goast.Expr{g.expr(f.Value)},
		})
	}
	body = append(body, g.stmts(rest)...)
	return &goast.FuncDecl{
		Recv: g.receiver(c, pos),
		Name: newIdent(InitName(c.Name), pos),
		Type: &goast.FuncType{Func: pos, Params: params},
		Body: &goast.BlockStmt{Lbrace: pos, List: body},
	}
}

// superInit lowers super(args) (or the implicit parent initialization).
func (g *Generator) superInit(c *semantic.Class, sc *ast.CallExpr) goast.Stmt {
	if c.ParentExpr == nil {
		return nil
	}
	pos := g.pos(c.Decl.Name.Pos())
	var args []goast.Expr
	ellipsis := false
	if sc != nil {
		pos = g.pos(sc.Pos())
		args = g.exprs(sc.Args)
		ellipsis = sc.Ellipsis
	}
	parentField := &goast.SelectorExpr{X: newIdent("this", pos), Sel: newIdent(c.ParentName, pos)}
	var call *goast.CallExpr
	switch {
	case c.Parent != nil:
		if !c.Parent.NeedsInit() {
			return nil
		}
		call = &goast.CallExpr{Fun: &goast.SelectorExpr{X: parentField, Sel: newIdent(InitName(c.Parent.Name), pos)}, Args: args}
	case sc != nil:
		// parent from another package or plain Go type: use its New function
		var fun goast.Expr
		base := c.ParentExpr
		if ix, ok := base.(*ast.IndexExpr); ok {
			base = ix.X
		}
		switch t := base.(type) {
		case *ast.SelectorExpr:
			fun = &goast.SelectorExpr{X: g.expr(t.X), Sel: newIdent(ConstructorName(t.Sel.Name), pos)}
		default:
			fun = newIdent(ConstructorName(c.ParentName), pos)
		}
		call = &goast.CallExpr{Fun: fun, Args: args}
		if ellipsis {
			call.Ellipsis = pos
		}
		return &goast.AssignStmt{Lhs: []goast.Expr{parentField}, Tok: token.ASSIGN, Rhs: []goast.Expr{&goast.StarExpr{Star: pos, X: call}}}
	default:
		return nil
	}
	if ellipsis {
		call.Ellipsis = pos
	}
	return &goast.ExprStmt{X: call}
}

func (g *Generator) method(c *semantic.Class, m *ast.MethodDecl) *goast.FuncDecl {
	pos := g.pos(m.Name.Pos())
	fd := &goast.FuncDecl{Name: g.ident(m.Name), Type: g.funcType(m.Type)}
	if m.Mods.Static {
		fd.Name = newIdent(semantic.StaticName(c.Name, m.Name.Name), pos)
	} else {
		fd.Recv = g.receiver(c, pos)
	}
	if m.Body != nil {
		fd.Body = g.block(m.Body)
	} else {
		// abstract method: a stub so the embedding class satisfies interfaces
		msg := fmt.Sprintf("abstract method %s.%s called", c.Name, m.Name.Name)
		fd.Body = &goast.BlockStmt{List: []goast.Stmt{&goast.ExprStmt{X: &goast.CallExpr{
			Fun: newIdent("panic", pos), Args: []goast.Expr{stringLit(msg)},
		}}}}
	}
	return fd
}

// GetterName and SetterName are the generated property accessor names.
func GetterName(prop string) string { return "Get" + prop }

// SetterName is the generated property setter name.
func SetterName(prop string) string { return "Set" + prop }

func (g *Generator) propertyAccessors(c *semantic.Class, p *ast.PropertyDecl) []*goast.FuncDecl {
	pos := g.pos(p.Name.Pos())
	var out []*goast.FuncDecl
	if p.Getter != nil {
		out = append(out, &goast.FuncDecl{
			Recv: g.receiver(c, pos),
			Name: newIdent(GetterName(p.Name.Name), pos),
			Type: &goast.FuncType{Func: pos, Params: &goast.FieldList{},
				Results: &goast.FieldList{List: []*goast.Field{{Type: g.expr(p.Type)}}}},
			Body: g.block(p.Getter),
		})
	}
	if p.Setter != nil {
		out = append(out, &goast.FuncDecl{
			Recv: g.receiver(c, pos),
			Name: newIdent(SetterName(p.Name.Name), pos),
			Type: &goast.FuncType{Func: pos, Params: &goast.FieldList{List: []*goast.Field{{
				Names: []*goast.Ident{newIdent("value", pos)}, Type: g.expr(p.Type),
			}}}},
			Body: g.block(p.Setter),
		})
	}
	return out
}

// implementsAssertion generates: var _ I = (*C)(nil)
func (g *Generator) implementsAssertion(c *semantic.Class, impl ast.Expr) goast.Decl {
	pos := g.pos(impl.Pos())
	typ := g.expr(impl)
	name, _ := refNameOf(impl)
	g.info.Implements[c] = append(g.info.Implements[c], semantic.ImplementsRef{Expr: typ, Source: impl, Name: name})
	value := &goast.CallExpr{
		Fun:  &goast.ParenExpr{Lparen: pos, X: &goast.StarExpr{Star: pos, X: newIdent(c.Name, pos)}},
		Args: []goast.Expr{newIdent("nil", pos)},
	}
	return &goast.GenDecl{TokPos: pos, Tok: token.VAR, Specs: []goast.Spec{&goast.ValueSpec{
		Names:  []*goast.Ident{newIdent("_", pos)},
		Type:   typ,
		Values: []goast.Expr{value},
	}}}
}

func refNameOf(x ast.Expr) (string, bool) {
	switch x := x.(type) {
	case *ast.Ident:
		return x.Name, false
	case *ast.SelectorExpr:
		n, _ := refNameOf(x.X)
		return n + "." + x.Sel.Name, true
	case *ast.IndexExpr:
		return refNameOf(x.X)
	}
	return "?", false
}
