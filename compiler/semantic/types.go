package semantic

import (
	"fmt"
	goast "go/ast"
	"go/importer"
	"go/token"
	"go/types"
	"strings"
	"sync"

	"github.com/raptordnj/gpp/compiler/diag"
)

// G++ reuses Go's type system: the lowered program is checked with go/types.
// Because generated nodes carry positions inside the original .gpp files,
// every diagnostic refers to .gpp lines and columns.

// TypeCheckResult carries the outcome of TypeCheck.
type TypeCheckResult struct {
	*TypeInfo
	Errors diag.ErrorList
	// Incomplete is set when imports could not be resolved.
	Incomplete bool
}

// The gc importer caches packages and is not safe for concurrent use, so
// type checking is serialized.
var (
	importerMu      sync.Mutex
	defaultImporter = importer.Default()
)

// fallbackImporter loads packages from compiled export data (fast, covers the
// standard library) and falls back to type checking package sources, which
// resolves packages of the enclosing Go module.
type fallbackImporter struct {
	gc  types.Importer
	src types.ImporterFrom
}

func (i *fallbackImporter) Import(path string) (*types.Package, error) {
	return i.ImportFrom(path, ".", 0)
}

func (i *fallbackImporter) ImportFrom(path, dir string, mode types.ImportMode) (*types.Package, error) {
	pkg, err := i.gc.Import(path)
	if err == nil {
		return pkg, nil
	}
	if p, srcErr := i.src.ImportFrom(path, dir, mode); srcErr == nil {
		return p, nil
	}
	return nil, err
}

// TypeCheck type-checks the lowered Go files and applies the G++ rules that
// need type information: member access control, override signatures and
// interface implementation.
func TypeCheck(fset *token.FileSet, files []*goast.File, prog *Program, low *LoweringInfo) *TypeCheckResult {
	importerMu.Lock()
	defer importerMu.Unlock()
	res := &TypeCheckResult{TypeInfo: &TypeInfo{}}
	implPos := map[token.Pos]bool{}
	for _, refs := range low.Implements {
		for _, r := range refs {
			implPos[r.Expr.Pos()] = true
		}
	}
	add := func(pos token.Position, msg string) {
		res.Errors = append(res.Errors, &diag.Error{Pos: diag.Position{Filename: pos.Filename, Line: pos.Line, Column: pos.Column}, Msg: msg})
	}
	conf := types.Config{
		Importer: &fallbackImporter{gc: defaultImporter, src: importer.ForCompiler(fset, "source", nil).(types.ImporterFrom)},
		Error: func(err error) {
			te, ok := err.(types.Error)
			if !ok {
				add(token.Position{}, err.Error())
				return
			}
			if strings.Contains(te.Msg, "could not import") {
				res.Incomplete = true
				return
			}
			if implPos[te.Pos] && strings.Contains(te.Msg, "does not implement") {
				return // reported with a G++ specific message below
			}
			add(fset.Position(te.Pos), translate(te.Msg))
		},
	}
	info := &types.Info{
		Types:      map[goast.Expr]types.TypeAndValue{},
		Defs:       map[*goast.Ident]types.Object{},
		Uses:       map[*goast.Ident]types.Object{},
		Selections: map[*goast.SelectorExpr]*types.Selection{},
	}
	pkgName := "main"
	if len(files) > 0 {
		pkgName = files[0].Name.Name
	}
	pkg, _ := conf.Check(pkgName, fset, files, info)
	res.Pkg, res.Info = pkg, info
	if res.Incomplete {
		// Without complete type information the Go toolchain reports the
		// remaining errors; keep only what we reported so far.
		return res
	}

	c := &typeChecker{prog: prog, low: low, pkg: pkg, info: info, fset: fset, add: add}
	c.checkAccess(files)
	c.checkOverrides()
	c.checkImplements()
	res.Errors.Sort()
	return res
}

// translate rewrites go/types messages into G++ terms.
func translate(msg string) string {
	if name, ok := strings.CutPrefix(msg, "undefined: "); ok {
		if name == "this" {
			return "'this' can only be used inside instance methods, constructors and property accessors"
		}
		if !strings.Contains(name, ".") {
			return fmt.Sprintf("unknown identifier '%s'", name)
		}
	}
	return msg
}

type typeChecker struct {
	prog *Program
	low  *LoweringInfo
	pkg  *types.Package
	info *types.Info
	fset *token.FileSet
	add  func(token.Position, string)
}

func (c *typeChecker) named(cl *Class) *types.Named {
	tn, _ := c.pkg.Scope().Lookup(cl.Name).(*types.TypeName)
	if tn == nil {
		return nil
	}
	n, _ := tn.Type().(*types.Named)
	return n
}

func origin(obj types.Object) types.Object {
	switch o := obj.(type) {
	case *types.Var:
		return o.Origin()
	case *types.Func:
		return o.Origin()
	}
	return obj
}

// MemberObjects maps go/types objects of class fields, properties and methods
// to their G++ members.
func MemberObjects(pkg *types.Package, prog *Program) map[types.Object]*Member {
	out := map[types.Object]*Member{}
	for _, cl := range prog.ClassOrder {
		tn, _ := pkg.Scope().Lookup(cl.Name).(*types.TypeName)
		if tn == nil {
			continue
		}
		named, _ := tn.Type().(*types.Named)
		if named == nil {
			continue
		}
		if st, ok := named.Underlying().(*types.Struct); ok {
			for i := 0; i < st.NumFields(); i++ {
				f := st.Field(i)
				if m := cl.Members[f.Name()]; m != nil && !f.Embedded() && (m.Kind == FieldMember || m.Kind == PropertyMember) {
					out[f] = m
				}
			}
		}
		for i := 0; i < named.NumMethods(); i++ {
			fn := named.Method(i)
			if m := cl.Members[fn.Name()]; m != nil && m.Kind == MethodMember {
				out[fn] = m
			}
		}
	}
	return out
}

func (c *typeChecker) checkAccess(files []*goast.File) {
	members := MemberObjects(c.pkg, c.prog)
	check := func(n goast.Node, ctx *Class) {
		goast.Inspect(n, func(n goast.Node) bool {
			id, ok := n.(*goast.Ident)
			if !ok {
				return true
			}
			obj := c.info.Uses[id]
			if obj == nil {
				return true
			}
			m := members[origin(obj)]
			if m == nil || CanAccess(m.Access, m.Class, ctx) {
				return true
			}
			c.add(c.fset.Position(id.Pos()), fmt.Sprintf("cannot access %s %s '%s' of class '%s'", m.Access, m.Kind, m.Name, m.Class.Name))
			return true
		})
	}
	for _, f := range files {
		for _, d := range f.Decls {
			if fd, ok := d.(*goast.FuncDecl); ok {
				check(fd, c.low.FuncClass[fd])
			} else {
				check(d, nil)
			}
		}
	}
}

func (c *typeChecker) checkOverrides() {
	for _, cl := range c.prog.ClassOrder {
		if cl.ParentExpr == nil {
			continue
		}
		named := c.named(cl)
		if named == nil {
			continue
		}
		st, ok := named.Underlying().(*types.Struct)
		if !ok || st.NumFields() == 0 || !st.Field(0).Embedded() {
			continue
		}
		parentType := st.Field(0).Type()
		for _, m := range cl.Order {
			if m.Kind != MethodMember || !m.Override {
				continue
			}
			var child *types.Func
			for i := 0; i < named.NumMethods(); i++ {
				if named.Method(i).Name() == m.Name {
					child = named.Method(i)
				}
			}
			if child == nil {
				continue
			}
			obj, _, _ := types.LookupFieldOrMethod(types.NewPointer(parentType), true, c.pkg, m.Name)
			parent, ok := obj.(*types.Func)
			pos := cl.File.Position(m.Pos)
			tp := token.Position{Filename: pos.Filename, Line: pos.Line, Column: pos.Column}
			if !ok {
				if cl.Parent == nil { // external parent, not verified before
					c.add(tp, fmt.Sprintf("method '%s' marked override but no overridable method exists in parent class '%s'", m.Name, cl.ParentName))
				}
				continue
			}
			if !types.Identical(child.Type(), parent.Type()) {
				q := types.RelativeTo(c.pkg)
				c.add(tp, fmt.Sprintf("method '%s' overrides '%s.%s' with a different signature: have %s, want %s",
					m.Name, cl.ParentName, m.Name, sigString(child, q), sigString(parent, q)))
			}
		}
	}
}

func sigString(fn *types.Func, q types.Qualifier) string {
	sig := fn.Type().(*types.Signature)
	s := types.TypeString(types.NewSignatureType(nil, nil, nil, sig.Params(), sig.Results(), sig.Variadic()), q)
	return strings.TrimPrefix(s, "func")
}

func (c *typeChecker) checkImplements() {
	for _, cl := range c.prog.ClassOrder {
		named := c.named(cl)
		if named == nil {
			continue
		}
		for _, ref := range c.low.Implements[cl] {
			pos := cl.File.Position(ref.Source.Pos())
			tp := token.Position{Filename: pos.Filename, Line: pos.Line, Column: pos.Column}
			tv, ok := c.info.Types[ref.Expr]
			if !ok || tv.Type == nil {
				continue // unresolved; already reported
			}
			iface, ok := tv.Type.Underlying().(*types.Interface)
			if !ok {
				c.add(tp, fmt.Sprintf("'%s' is not an interface", ref.Name))
				continue
			}
			m, wrong := types.MissingMethod(types.NewPointer(named), iface, true)
			switch {
			case m == nil:
			case wrong:
				c.add(tp, fmt.Sprintf("class '%s' does not implement interface '%s': method '%s' has the wrong signature", cl.Name, ref.Name, m.Name()))
			default:
				c.add(tp, fmt.Sprintf("class '%s' does not implement interface '%s': missing method '%s'", cl.Name, ref.Name, m.Name()))
			}
		}
	}
}
