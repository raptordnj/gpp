package semantic

import (
	"github.com/raptordnj/gpp/compiler/ast"
	"github.com/raptordnj/gpp/compiler/diag"
)

// Analyzer performs the declaration-level G++ semantic analysis.
type Analyzer struct {
	prog   *Program
	errors diag.ErrorList
	file   *diag.File // file currently being analyzed
}

// Analyze builds the symbol tables for the given files (one package) and
// validates the G++ object-oriented rules. Type checking of expressions is
// performed later by TypeCheck on the lowered program.
func Analyze(files []*ast.File) (*Program, diag.ErrorList) {
	a := &Analyzer{prog: &Program{
		Files:      files,
		Classes:    map[string]*Class{},
		Interfaces: map[string]*Interface{},
		Enums:      map[string]*Enum{},
		GoTypes:    map[string]ast.Expr{},
		StaticRefs: map[*ast.SelectorExpr]string{},
	}}
	a.collect()
	a.resolveParents()
	a.collectMembers()
	for _, c := range a.prog.ClassOrder {
		a.file = c.File
		a.checkClass(c)
	}
	for _, f := range files {
		a.file = f.Source
		a.checkBodies(f)
	}
	a.errors.Sort()
	return a.prog, a.errors
}

func (a *Analyzer) errorf(pos ast.Pos, format string, args ...any) {
	a.errors.Add(a.file.Position(pos), format, args...)
}

// ---------------------------------------------------------------------------
// Declarations

func (a *Analyzer) declared(name string) bool {
	p := a.prog
	return p.Classes[name] != nil || p.Interfaces[name] != nil || p.Enums[name] != nil
}

func (a *Analyzer) collect() {
	for _, f := range a.prog.Files {
		a.file = f.Source
		for _, d := range f.Decls {
			switch d := d.(type) {
			case *ast.ClassDecl:
				if a.declared(d.Name.Name) {
					a.errorf(d.Name.Pos(), "'%s' redeclared in this package", d.Name.Name)
					continue
				}
				c := &Class{Name: d.Name.Name, Decl: d, File: f.Source, Members: map[string]*Member{}}
				a.prog.Classes[c.Name] = c
				a.prog.ClassOrder = append(a.prog.ClassOrder, c)
			case *ast.InterfaceDecl:
				if a.declared(d.Name.Name) {
					a.errorf(d.Name.Pos(), "'%s' redeclared in this package", d.Name.Name)
					continue
				}
				a.prog.Interfaces[d.Name.Name] = &Interface{Name: d.Name.Name, Decl: d, File: f.Source}
			case *ast.EnumDecl:
				if a.declared(d.Name.Name) {
					a.errorf(d.Name.Pos(), "'%s' redeclared in this package", d.Name.Name)
					continue
				}
				e := &Enum{Name: d.Name.Name, Decl: d, File: f.Source, Members: map[string]bool{}}
				for _, m := range d.Members {
					e.Members[m.Name] = true
				}
				a.prog.Enums[e.Name] = e
			case *ast.GenDecl:
				for _, s := range d.Specs {
					if ts, ok := s.(*ast.TypeSpec); ok {
						a.prog.GoTypes[ts.Name.Name] = ts.Type
					}
				}
			}
		}
	}
}

// refName returns the base identifier of a class reference (A, pkg.A, A<T>).
func refName(x ast.Expr) (name string, qualified bool) {
	switch x := x.(type) {
	case *ast.Ident:
		return x.Name, false
	case *ast.SelectorExpr:
		return x.Sel.Name, true
	case *ast.IndexExpr:
		return refName(x.X)
	}
	return "", false
}

func (a *Analyzer) resolveParents() {
	for _, c := range a.prog.ClassOrder {
		a.file = c.File
		ext := c.Decl.Extends
		if ext == nil {
			continue
		}
		c.ParentExpr = ext
		name, qualified := refName(ext)
		c.ParentName = name
		if qualified {
			continue // class from another package: embedded as-is
		}
		switch {
		case a.prog.Classes[name] != nil:
			c.Parent = a.prog.Classes[name]
		case a.prog.Interfaces[name] != nil:
			a.errorf(ext.Pos(), "class '%s' cannot extend interface '%s'; use 'implements'", c.Name, name)
		case a.prog.GoTypes[name] != nil:
			// a plain Go type: embedded, but no OOP checks
		default:
			a.errorf(ext.Pos(), "unknown class '%s'", name)
		}
	}
	// detect inheritance cycles
	for _, c := range a.prog.ClassOrder {
		seen := map[*Class]bool{}
		for k := c; k != nil; k = k.Parent {
			if seen[k] {
				a.file = c.File
				a.errorf(c.Decl.Name.Pos(), "inheritance cycle involving class '%s'", c.Name)
				c.Parent = nil
				break
			}
			seen[k] = true
		}
	}
}

func (a *Analyzer) addMember(c *Class, m *Member) {
	if prev := c.Members[m.Name]; prev != nil {
		a.errorf(m.Pos, "duplicate member '%s' in class '%s'", m.Name, c.Name)
		return
	}
	if m.Name == c.ParentName {
		a.errorf(m.Pos, "member '%s' conflicts with the parent class name", m.Name)
	}
	m.Class = c
	c.Members[m.Name] = m
	c.Order = append(c.Order, m)
}

func (a *Analyzer) collectMembers() {
	for _, c := range a.prog.ClassOrder {
		a.file = c.File
		for _, f := range c.Decl.Fields {
			kind := FieldMember
			if f.Mods.Static {
				kind = StaticFieldMember
			}
			for _, n := range f.Names {
				a.addMember(c, &Member{Name: n.Name, Kind: kind, Access: f.Mods.Access, Pos: n.Pos(), Field: f})
			}
		}
		for _, p := range c.Decl.Properties {
			a.addMember(c, &Member{Name: p.Name.Name, Kind: PropertyMember, Access: p.Mods.Access, Pos: p.Name.Pos(), Property: p})
		}
		for _, m := range c.Decl.Methods {
			kind := MethodMember
			if m.Mods.Static {
				kind = StaticMethodMember
			}
			a.addMember(c, &Member{Name: m.Name.Name, Kind: kind, Access: m.Mods.Access, Pos: m.Name.Pos(),
				Abstract: m.Mods.Abstract, Override: m.Mods.Override, Method: m})
		}
	}
}

// ---------------------------------------------------------------------------
// Class rules

func (a *Analyzer) checkClass(c *Class) {
	d := c.Decl
	for _, m := range c.Order {
		if m.Kind == PropertyMember {
			if m.Property.Mods.Static || m.Property.Mods.Abstract || m.Property.Mods.Override {
				a.errorf(m.Pos, "property '%s' cannot be static, abstract or override", m.Name)
			}
			if p := c.Parent; p != nil && p.Lookup(m.Name) != nil {
				a.errorf(m.Pos, "property '%s' in class '%s' hides a member of parent class '%s'", m.Name, c.Name, p.Name)
			}
			continue
		}
		if m.IsStatic() {
			if c.IsGeneric() {
				a.errorf(m.Pos, "generic class '%s' cannot declare static %s '%s'", c.Name, map[bool]string{true: "field", false: "method"}[m.Kind == StaticFieldMember], m.Name)
			}
			if m.Abstract || m.Override {
				a.errorf(m.Pos, "static method '%s' cannot be abstract or override", m.Name)
			}
			continue
		}
		if m.Kind != MethodMember {
			if p := c.Parent; p != nil {
				if pm := p.Lookup(m.Name); pm != nil && pm.Kind == FieldMember {
					a.errorf(m.Pos, "field '%s' in class '%s' hides field of parent class '%s'", m.Name, c.Name, pm.Class.Name)
				}
			}
			continue
		}
		if m.Abstract && !d.Abstract {
			a.errorf(m.Pos, "class '%s' must be declared abstract to declare abstract method '%s'", c.Name, m.Name)
		}
		if m.Abstract && m.Access == ast.Private {
			a.errorf(m.Pos, "abstract method '%s' cannot be private", m.Name)
		}
		a.checkOverride(c, m)
	}
	if !d.Abstract {
		a.checkAbstractImplemented(c)
	}
	for _, impl := range d.Implements {
		name, qualified := refName(impl)
		if qualified {
			continue
		}
		switch {
		case a.prog.Interfaces[name] != nil:
		case a.prog.Classes[name] != nil:
			a.errorf(impl.Pos(), "'%s' is a class, not an interface; use 'extends'", name)
		case a.prog.GoTypes[name] != nil:
			if _, ok := a.prog.GoTypes[name].(*ast.InterfaceType); !ok {
				a.errorf(impl.Pos(), "'%s' is not an interface", name)
			}
		default:
			if !isPredeclaredInterface(name) {
				a.errorf(impl.Pos(), "unknown interface '%s'", name)
			}
		}
	}
	a.checkConstructor(c)
}

func isPredeclaredInterface(name string) bool {
	return name == "error" || name == "any" || name == "comparable"
}

func (a *Analyzer) checkOverride(c *Class, m *Member) {
	var pm *Member
	if c.Parent != nil {
		pm = c.Parent.Lookup(m.Name)
	}
	if !m.Override {
		if pm != nil && pm.Kind == MethodMember && pm.Access != ast.Private {
			a.errorf(m.Pos, "method '%s' in class '%s' hides method of parent class '%s'; mark it 'override'", m.Name, c.Name, pm.Class.Name)
		}
		return
	}
	switch {
	case c.ParentExpr == nil:
		a.errorf(m.Pos, "method '%s' marked override but class '%s' has no parent class", m.Name, c.Name)
	case c.Parent == nil:
		// external parent: cannot be verified before type checking
	case pm == nil || pm.Kind != MethodMember || pm.Access == ast.Private:
		a.errorf(m.Pos, "method '%s' marked override but no overridable method exists in parent class '%s'", m.Name, c.Parent.Name)
	case m.Access != ast.AccessDefault && accessRank(m.Access) < accessRank(pm.Access):
		a.errorf(m.Pos, "method '%s' cannot reduce visibility of overridden method from '%s' to '%s'", m.Name, pm.Access, m.Access)
	}
}

func accessRank(a ast.Access) int {
	switch a {
	case ast.Private:
		return 0
	case ast.Protected:
		return 1
	case ast.Internal:
		return 2
	}
	return 3
}

// checkAbstractImplemented verifies that a concrete class implements all
// abstract methods inherited from its ancestors.
func (a *Analyzer) checkAbstractImplemented(c *Class) {
	var chain []*Class
	for k := c; k != nil; k = k.Parent {
		chain = append([]*Class{k}, chain...)
	}
	abstract := map[string]*Member{}
	var order []string
	for _, k := range chain {
		for _, m := range k.Order {
			if m.Kind != MethodMember {
				continue
			}
			if m.Abstract {
				if abstract[m.Name] == nil {
					order = append(order, m.Name)
				}
				abstract[m.Name] = m
			} else {
				delete(abstract, m.Name)
			}
		}
	}
	for _, name := range order {
		if m := abstract[name]; m != nil && m.Class != c {
			a.errorf(c.Decl.Name.Pos(), "class '%s' does not implement abstract method '%s' from class '%s'", c.Name, name, m.Class.Name)
		}
	}
}

// superCall returns the super(...) call if stmt is one.
func superCall(s ast.Stmt) *ast.CallExpr {
	es, ok := s.(*ast.ExprStmt)
	if !ok {
		return nil
	}
	call, ok := es.X.(*ast.CallExpr)
	if !ok {
		return nil
	}
	if _, ok := call.Fun.(*ast.SuperExpr); ok {
		return call
	}
	return nil
}

// SuperCall returns the leading super(...) call of a constructor body, if any.
func SuperCall(ctor *ast.ConstructorDecl) *ast.CallExpr {
	if ctor == nil || len(ctor.Body.List) == 0 {
		return nil
	}
	return superCall(ctor.Body.List[0])
}

func (a *Analyzer) checkConstructor(c *Class) {
	ctor := c.Decl.Ctor
	var sc *ast.CallExpr
	if ctor != nil {
		sc = SuperCall(ctor)
		if ctor.Mods.Override || ctor.Mods.Abstract || ctor.Mods.Static {
			a.errorf(ctor.Pos(), "invalid modifier on constructor")
		}
	}
	p := c.Parent
	if sc != nil {
		if c.ParentExpr == nil {
			a.errorf(sc.Pos(), "class '%s' has no parent class; 'super(...)' is not allowed", c.Name)
		} else if p != nil {
			a.checkArgCount(sc.Pos(), p, sc.Args, sc.Ellipsis, "super constructor of class")
		}
		return
	}
	if p != nil && p.CtorParams().NumFields() > 0 {
		pos := c.Decl.Name.Pos()
		if ctor != nil {
			pos = ctor.Pos()
			a.errorf(pos, "constructor of class '%s' must call super(...) because the constructor of '%s' requires arguments", c.Name, p.Name)
		} else {
			a.errorf(pos, "class '%s' must declare a constructor that calls super(...) because the constructor of '%s' requires arguments", c.Name, p.Name)
		}
	}
}

func (a *Analyzer) checkArgCount(pos ast.Pos, c *Class, args []ast.Expr, ellipsis bool, what string) {
	params := c.CtorParams()
	want := params.NumFields()
	variadic := false
	if params != nil && len(params.List) > 0 {
		_, variadic = params.List[len(params.List)-1].Type.(*ast.Ellipsis)
	}
	got := len(args)
	if ellipsis {
		return // f(xs...) is checked by the type checker
	}
	if got == 1 && want > 1 {
		if _, isCall := args[0].(*ast.CallExpr); isCall {
			return // multi-value call; checked by the type checker
		}
	}
	plural := "s"
	if want == 1 {
		plural = ""
	}
	switch {
	case variadic && got < want-1:
		a.errorf(pos, "%s '%s' expects at least %d argument%s, got %d", what, c.Name, want-1, map[bool]string{true: "", false: "s"}[want-1 == 1], got)
	case !variadic && got != want:
		a.errorf(pos, "%s '%s' expects %d argument%s, got %d", what, c.Name, want, plural, got)
	}
}

// ---------------------------------------------------------------------------
// Bodies

type bodyContext struct {
	class    *Class
	static   bool // static method
	ctor     bool
	allowed  map[*ast.CallExpr]bool // permitted super(...) calls
	funcName string
}

func (a *Analyzer) checkBodies(f *ast.File) {
	for _, d := range f.Decls {
		switch d := d.(type) {
		case *ast.FuncDecl:
			a.walkBody(d, &bodyContext{funcName: d.Name.Name})
		case *ast.GenDecl:
			a.walkBody(d, &bodyContext{})
		case *ast.ClassDecl:
			c := a.prog.Classes[d.Name.Name]
			if c == nil || c.Decl != d {
				continue
			}
			for _, fd := range d.Fields {
				a.walkBody(fd.Type, &bodyContext{class: c, static: fd.Mods.Static})
				a.walkBody(fd.Value, &bodyContext{class: c, static: fd.Mods.Static})
			}
			if d.Ctor != nil {
				ctx := &bodyContext{class: c, ctor: true, allowed: map[*ast.CallExpr]bool{}}
				if sc := SuperCall(d.Ctor); sc != nil {
					ctx.allowed[sc] = true
				}
				a.walkBody(d.Ctor.Type, ctx)
				a.walkBody(d.Ctor.Body, ctx)
			}
			for _, m := range d.Methods {
				ctx := &bodyContext{class: c, static: m.Mods.Static, funcName: m.Name.Name}
				a.walkBody(m.Type, ctx)
				a.walkBody(m.Body, ctx)
			}
			for _, p := range d.Properties {
				ctx := &bodyContext{class: c, funcName: p.Name.Name}
				a.walkBody(p.Type, ctx)
				a.walkBody(p.Getter, ctx)
				a.walkBody(p.Setter, ctx)
			}
		case *ast.InterfaceDecl:
			a.walkBody(d, &bodyContext{})
		}
	}
}

func (a *Analyzer) walkBody(n ast.Node, ctx *bodyContext) {
	if n == nil {
		return
	}
	ast.Walk(n, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.NewExpr:
			a.checkNew(x, ctx)
		case *ast.CallExpr:
			if _, ok := x.Fun.(*ast.SuperExpr); ok {
				if !ctx.allowed[x] {
					a.errorf(x.Pos(), "super(...) can only be called as the first statement of a constructor")
				}
				for _, arg := range x.Args {
					a.walkBody(arg, ctx)
				}
				return false
			}
		case *ast.SelectorExpr:
			if _, ok := x.X.(*ast.SuperExpr); ok {
				a.checkSuperMember(x, ctx)
				return false
			}
			if id, ok := x.X.(*ast.Ident); ok {
				if a.checkStaticRef(x, id, ctx) {
					return false
				}
				if id.Name == "this" {
					a.checkThisMember(x, ctx)
				}
			}
		case *ast.SuperExpr:
			a.errorf(x.Pos(), "'super' must be followed by '(' or '.'")
		}
		return true
	})
}

func (a *Analyzer) checkNew(x *ast.NewExpr, ctx *bodyContext) {
	name, qualified := refName(x.Type)
	if qualified {
		return
	}
	c := a.prog.Classes[name]
	if c == nil {
		if a.prog.Interfaces[name] != nil {
			a.errorf(x.Type.Pos(), "cannot instantiate interface '%s'", name)
		} else {
			a.errorf(x.Type.Pos(), "unknown class '%s'", name)
		}
		return
	}
	if c.Decl.Abstract {
		a.errorf(x.Pos(), "cannot instantiate abstract class '%s'", name)
		return
	}
	if ctor := c.Decl.Ctor; ctor != nil && !a.canAccess(ctor.Mods.Access, c, ctx.class) {
		a.errorf(x.Pos(), "cannot access %s constructor of class '%s'", ctor.Mods.Access, name)
	}
	if len(x.TypeArgs) > 0 {
		want := c.Decl.TypeParams.NumFields()
		if want == 0 {
			a.errorf(x.Type.Pos(), "class '%s' is not generic", name)
		} else if len(x.TypeArgs) != want {
			a.errorf(x.Type.Pos(), "class '%s' expects %d type arguments, got %d", name, want, len(x.TypeArgs))
		}
	}
	a.checkArgCount(x.Pos(), c, x.Args, x.Ellipsis, "constructor for class")
}

func (a *Analyzer) checkSuperMember(x *ast.SelectorExpr, ctx *bodyContext) {
	c := ctx.class
	switch {
	case c == nil || ctx.static:
		a.errorf(x.X.Pos(), "'super' can only be used inside instance methods and constructors")
		return
	case c.ParentExpr == nil:
		a.errorf(x.X.Pos(), "'super' used in class '%s' which has no parent class", c.Name)
		return
	case c.Parent == nil:
		return // external parent
	}
	m := c.Parent.Lookup(x.Sel.Name)
	if m == nil {
		a.errorf(x.Sel.Pos(), "parent class '%s' has no member '%s'", c.Parent.Name, x.Sel.Name)
		return
	}
	if m.IsStatic() {
		a.errorf(x.Sel.Pos(), "cannot access static %s '%s' through 'super'; use '%s.%s'", m.Kind, m.Name, m.Class.Name, m.Name)
		return
	}
	if m.Abstract {
		a.errorf(x.Sel.Pos(), "cannot call abstract method '%s' through 'super'", m.Name)
		return
	}
	if !a.canAccess(m.Access, m.Class, c) {
		a.errorf(x.Sel.Pos(), "cannot access %s %s '%s' of class '%s'", m.Access, m.Kind, m.Name, m.Class.Name)
	}
}

// checkThisMember rejects calls of abstract methods through this: G++ v0.1
// lowers inheritance to Go embedding, which dispatches statically, so such a
// call would always reach the abstract stub.
func (a *Analyzer) checkThisMember(x *ast.SelectorExpr, ctx *bodyContext) {
	if ctx.class == nil || ctx.static {
		return
	}
	if m := ctx.class.Lookup(x.Sel.Name); m != nil && m.Abstract {
		a.errorf(x.Sel.Pos(), "cannot call abstract method '%s' through 'this' in class '%s': G++ v0.1 uses static dispatch, so the call would not reach subclass overrides", m.Name, ctx.class.Name)
	}
}

// checkStaticRef handles Class.Member and Enum.Member selectors. It reports
// whether x was such a reference.
func (a *Analyzer) checkStaticRef(x *ast.SelectorExpr, id *ast.Ident, ctx *bodyContext) bool {
	if e := a.prog.Enums[id.Name]; e != nil {
		if !e.Members[x.Sel.Name] {
			a.errorf(x.Sel.Pos(), "enum '%s' has no member '%s'", e.Name, x.Sel.Name)
		}
		a.prog.StaticRefs[x] = e.Name + x.Sel.Name
		return true
	}
	c := a.prog.Classes[id.Name]
	if c == nil {
		return false
	}
	var m *Member
	for k := c; k != nil && m == nil; k = k.Parent {
		if mm := k.Members[x.Sel.Name]; mm != nil && mm.IsStatic() {
			m = mm
		}
	}
	if m == nil {
		if im := c.Lookup(x.Sel.Name); im != nil {
			a.errorf(x.Sel.Pos(), "cannot access instance %s '%s' of class '%s' without an object", im.Kind, im.Name, im.Class.Name)
		} else {
			a.errorf(x.Sel.Pos(), "class '%s' has no static member '%s'", c.Name, x.Sel.Name)
		}
		return true
	}
	if !a.canAccess(m.Access, m.Class, ctx.class) {
		a.errorf(x.Sel.Pos(), "cannot access %s %s '%s' of class '%s'", m.Access, m.Kind, m.Name, m.Class.Name)
	}
	a.prog.StaticRefs[x] = StaticName(m.Class.Name, m.Name)
	return true
}

// StaticName is the generated Go name of a static member.
func StaticName(class, member string) string { return class + "_" + member }

func (a *Analyzer) canAccess(acc ast.Access, owner, from *Class) bool {
	return CanAccess(acc, owner, from)
}

// CanAccess implements the G++ member access rules: private members are
// visible only inside the declaring class, protected members inside the
// declaring class and its subclasses, and public/internal members anywhere in
// the package.
func CanAccess(acc ast.Access, owner, from *Class) bool {
	switch acc {
	case ast.Private:
		return from == owner
	case ast.Protected:
		return from != nil && from.IsSubclassOf(owner)
	}
	return true
}
