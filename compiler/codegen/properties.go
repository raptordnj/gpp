package codegen

import (
	goast "go/ast"
	"go/token"
	"go/types"
	"reflect"

	"gpp/compiler/diag"
	"gpp/compiler/semantic"
)

// Properties are lowered in two steps. During the first type check each
// property is represented by a temporary struct field, so that obj.Name
// resolves with full type information. RewriteProperties then turns reads
// into obj.GetName() calls and writes into obj.SetName(v) calls, and removes
// the temporary fields.

type propRef struct {
	class  *semantic.Class
	name   string
	getter bool
	setter bool
}

// HasProperties reports whether any class declares a property.
func (o *Output) HasProperties() bool { return len(o.Info.PropertyFields) > 0 }

// RewriteProperties replaces property accesses with accessor calls.
func RewriteProperties(out *Output, prog *semantic.Program, ti *semantic.TypeInfo) diag.ErrorList {
	props := map[types.Object]*propRef{}
	for obj, m := range semantic.MemberObjects(ti.Pkg, prog) {
		if m.Kind == semantic.PropertyMember {
			props[obj] = &propRef{class: m.Class, name: m.Name, getter: m.Property.Getter != nil, setter: m.Property.Setter != nil}
		}
	}
	r := &propRewriter{fset: out.Fset, info: ti.Info, props: props}
	for _, f := range out.Files {
		r.rewrite(reflect.ValueOf(f))
	}
	// drop the temporary fields
	for _, f := range out.Files {
		goast.Inspect(f, func(n goast.Node) bool {
			st, ok := n.(*goast.StructType)
			if !ok {
				return true
			}
			list := st.Fields.List[:0]
			for _, fld := range st.Fields.List {
				if !isPropertyField(out.Info, fld) {
					list = append(list, fld)
				}
			}
			st.Fields.List = list
			return true
		})
	}
	r.errs.Sort()
	return r.errs
}

func isPropertyField(info *semantic.LoweringInfo, f *goast.Field) bool {
	for _, m := range info.PropertyFields {
		for _, pf := range m {
			if pf == f {
				return true
			}
		}
	}
	return false
}

type propRewriter struct {
	fset  *token.FileSet
	info  *types.Info
	props map[types.Object]*propRef
	errs  diag.ErrorList
}

func (r *propRewriter) errorf(pos token.Pos, format string, args ...any) {
	p := r.fset.Position(pos)
	r.errs.Add(diag.Position{Filename: p.Filename, Line: p.Line, Column: p.Column}, format, args...)
}

// property returns the property referenced by x, if any.
func (r *propRewriter) property(x goast.Expr) (*goast.SelectorExpr, *propRef) {
	sel, ok := x.(*goast.SelectorExpr)
	if !ok {
		return nil, nil
	}
	s := r.info.Selections[sel]
	if s == nil || s.Kind() != types.FieldVal {
		return nil, nil
	}
	obj := s.Obj()
	if v, ok := obj.(*types.Var); ok {
		obj = v.Origin()
	}
	p := r.props[obj]
	if p == nil {
		return nil, nil
	}
	return sel, p
}

func (r *propRewriter) getter(sel *goast.SelectorExpr, p *propRef) goast.Expr {
	if !p.getter {
		r.errorf(sel.Sel.Pos(), "property '%s' of class '%s' is write-only", p.name, p.class.Name)
	}
	return &goast.CallExpr{Fun: &goast.SelectorExpr{X: sel.X, Sel: newIdent(GetterName(p.name), sel.Sel.Pos())}, Lparen: sel.Sel.Pos()}
}

func (r *propRewriter) setter(sel *goast.SelectorExpr, p *propRef, value goast.Expr) goast.Stmt {
	if !p.setter {
		r.errorf(sel.Sel.Pos(), "property '%s' of class '%s' is read-only", p.name, p.class.Name)
	}
	return &goast.ExprStmt{X: &goast.CallExpr{
		Fun:    &goast.SelectorExpr{X: sel.X, Sel: newIdent(SetterName(p.name), sel.Sel.Pos())},
		Lparen: sel.Sel.Pos(),
		Args:   []goast.Expr{value},
	}}
}

var compoundOps = map[token.Token]token.Token{
	token.ADD_ASSIGN: token.ADD, token.SUB_ASSIGN: token.SUB, token.MUL_ASSIGN: token.MUL,
	token.QUO_ASSIGN: token.QUO, token.REM_ASSIGN: token.REM, token.AND_ASSIGN: token.AND,
	token.OR_ASSIGN: token.OR, token.XOR_ASSIGN: token.XOR, token.SHL_ASSIGN: token.SHL,
	token.SHR_ASSIGN: token.SHR, token.AND_NOT_ASSIGN: token.AND_NOT,
}

// rewriteStmt handles statements that write to a property.
func (r *propRewriter) rewriteStmt(s goast.Stmt) goast.Stmt {
	switch s := s.(type) {
	case *goast.AssignStmt:
		for _, lhs := range s.Lhs {
			sel, p := r.property(lhs)
			if p == nil {
				continue
			}
			if len(s.Lhs) != 1 || len(s.Rhs) != 1 {
				r.errorf(sel.Sel.Pos(), "property '%s' cannot be assigned in a multi-value assignment", p.name)
				return s
			}
			value := r.rewriteExpr(s.Rhs[0])
			if op, ok := compoundOps[s.Tok]; ok {
				if _, isBinary := value.(*goast.BinaryExpr); isBinary {
					value = &goast.ParenExpr{X: value}
				}
				value = &goast.BinaryExpr{X: r.getter(sel, p), Op: op, Y: value}
			}
			r.rewriteSelectorBase(sel)
			return r.setter(sel, p, value)
		}
	case *goast.IncDecStmt:
		if sel, p := r.property(s.X); p != nil {
			op := token.ADD
			if s.Tok == token.DEC {
				op = token.SUB
			}
			r.rewriteSelectorBase(sel)
			one := &goast.BasicLit{Kind: token.INT, Value: "1"}
			return r.setter(sel, p, &goast.BinaryExpr{X: r.getter(sel, p), Op: op, Y: one})
		}
	}
	return nil
}

func (r *propRewriter) rewriteSelectorBase(sel *goast.SelectorExpr) {
	sel.X = r.rewriteExpr(sel.X)
}

// rewriteExpr rewrites an expression tree, replacing property reads.
func (r *propRewriter) rewriteExpr(x goast.Expr) goast.Expr {
	if x == nil {
		return nil
	}
	if u, ok := x.(*goast.UnaryExpr); ok && u.Op == token.AND {
		if sel, p := r.property(u.X); p != nil {
			r.errorf(sel.Sel.Pos(), "cannot take the address of property '%s'", p.name)
			return x
		}
	}
	if sel, p := r.property(x); p != nil {
		r.rewriteSelectorBase(sel)
		return r.getter(sel, p)
	}
	r.rewrite(reflect.ValueOf(x))
	return x
}

var (
	exprType     = reflect.TypeOf((*goast.Expr)(nil)).Elem()
	stmtType     = reflect.TypeOf((*goast.Stmt)(nil)).Elem()
	exprListType = reflect.TypeOf([]goast.Expr(nil))
	stmtListType = reflect.TypeOf([]goast.Stmt(nil))
)

// rewrite visits the fields of a node (a pointer to a go/ast struct) and
// replaces property accesses in place.
func (r *propRewriter) rewrite(v reflect.Value) {
	if v.Kind() == reflect.Interface {
		v = v.Elem()
	}
	if v.Kind() != reflect.Pointer || v.IsNil() {
		return
	}
	e := v.Elem()
	if e.Kind() != reflect.Struct {
		return
	}
	for i := 0; i < e.NumField(); i++ {
		f := e.Field(i)
		if !f.CanSet() {
			continue
		}
		switch {
		case f.Type() == exprType:
			if !f.IsNil() {
				f.Set(reflect.ValueOf(r.rewriteExpr(f.Interface().(goast.Expr))))
			}
		case f.Type() == exprListType:
			for j := 0; j < f.Len(); j++ {
				el := f.Index(j)
				el.Set(reflect.ValueOf(r.rewriteExpr(el.Interface().(goast.Expr))))
			}
		case f.Type() == stmtType:
			if !f.IsNil() {
				if ns := r.rewriteStmt(f.Interface().(goast.Stmt)); ns != nil {
					f.Set(reflect.ValueOf(ns))
				} else {
					r.rewrite(f)
				}
			}
		case f.Type() == stmtListType:
			for j := 0; j < f.Len(); j++ {
				el := f.Index(j)
				if ns := r.rewriteStmt(el.Interface().(goast.Stmt)); ns != nil {
					el.Set(reflect.ValueOf(ns))
				} else {
					r.rewrite(el)
				}
			}
		case f.Kind() == reflect.Pointer || f.Kind() == reflect.Interface:
			if f.Type().Implements(reflect.TypeOf((*goast.Node)(nil)).Elem()) {
				r.rewrite(f)
			}
		case f.Kind() == reflect.Slice:
			for j := 0; j < f.Len(); j++ {
				el := f.Index(j)
				if el.Kind() == reflect.Pointer || el.Kind() == reflect.Interface {
					r.rewrite(el)
				}
			}
		}
	}
}
