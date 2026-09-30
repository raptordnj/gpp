// Package semantic implements G++ semantic analysis: symbol tables for
// classes, interfaces and enums, OOP rule validation (inheritance, override,
// abstract, access modifiers, constructors) and type checking of the lowered
// program with go/types.
package semantic

import (
	goast "go/ast"
	"go/types"

	"gpp/compiler/ast"
	"gpp/compiler/diag"
)

// MemberKind classifies class members.
type MemberKind int

// Member kinds.
const (
	FieldMember MemberKind = iota
	MethodMember
	StaticFieldMember
	StaticMethodMember
	PropertyMember
)

func (k MemberKind) String() string {
	switch k {
	case FieldMember:
		return "field"
	case StaticFieldMember:
		return "static field"
	case StaticMethodMember:
		return "static method"
	case PropertyMember:
		return "property"
	}
	return "method"
}

// Member describes a class member.
type Member struct {
	Name     string
	Kind     MemberKind
	Access   ast.Access
	Abstract bool
	Override bool
	Class    *Class
	Pos      ast.Pos
	Method   *ast.MethodDecl   // for methods
	Field    *ast.FieldDecl    // for fields
	Property *ast.PropertyDecl // for properties
}

// IsStatic reports whether the member belongs to the class rather than instances.
func (m *Member) IsStatic() bool {
	return m.Kind == StaticFieldMember || m.Kind == StaticMethodMember
}

// Class is the symbol for a G++ class.
type Class struct {
	Name       string
	Decl       *ast.ClassDecl
	File       *diag.File
	Parent     *Class   // G++ parent class, nil if none or external
	ParentExpr ast.Expr // extends expression, nil if none
	ParentName string   // name of the embedded parent field
	Members    map[string]*Member
	Order      []*Member

	needsInit *bool
}

// IsGeneric reports whether the class has type parameters.
func (c *Class) IsGeneric() bool { return c.Decl.TypeParams != nil }

// Lookup finds a member in the class or its ancestors.
func (c *Class) Lookup(name string) *Member {
	for k := c; k != nil; k = k.Parent {
		if m := k.Members[name]; m != nil {
			return m
		}
	}
	return nil
}

// IsSubclassOf reports whether c is other or inherits from it.
func (c *Class) IsSubclassOf(other *Class) bool {
	for k := c; k != nil; k = k.Parent {
		if k == other {
			return true
		}
	}
	return false
}

// CtorParams returns the declared constructor parameters (nil if none).
func (c *Class) CtorParams() *ast.FieldList {
	if c.Decl.Ctor == nil {
		return nil
	}
	return c.Decl.Ctor.Type.Params
}

// NeedsInit reports whether construction requires running an init method:
// the class has a constructor, instance field initializers, or a G++ parent
// that needs init.
func (c *Class) NeedsInit() bool {
	if c.needsInit != nil {
		return *c.needsInit
	}
	v := false
	c.needsInit = &v // cycle guard
	if c.Decl.Ctor != nil {
		v = true
	}
	for _, f := range c.Decl.Fields {
		if f.Value != nil && !f.Mods.Static {
			v = true
		}
	}
	if c.Parent != nil && c.Parent.NeedsInit() {
		v = true
	}
	c.needsInit = &v
	return v
}

// Interface is the symbol for a G++ interface declaration.
type Interface struct {
	Name string
	Decl *ast.InterfaceDecl
	File *diag.File
}

// Enum is the symbol for an enum.
type Enum struct {
	Name    string
	Decl    *ast.EnumDecl
	File    *diag.File
	Members map[string]bool
}

// Program holds the symbol tables for a package made of one or more files.
type Program struct {
	Files      []*ast.File
	Classes    map[string]*Class
	ClassOrder []*Class
	Interfaces map[string]*Interface
	Enums      map[string]*Enum
	GoTypes    map[string]ast.Expr // top-level Go type declarations (name -> type)

	// StaticRefs maps Class.Member / Enum.Member selector expressions to the
	// generated package-level Go identifier.
	StaticRefs map[*ast.SelectorExpr]string
}

// LoweringInfo is produced by code generation and consumed by the go/types
// based checks.
type LoweringInfo struct {
	// FuncClass records the class a generated function belongs to (methods,
	// init methods, constructors, static methods, property accessors).
	FuncClass map[*goast.FuncDecl]*Class
	// Implements records, per class, the lowered interface type expressions.
	Implements map[*Class][]ImplementsRef
	// PropertyFields are the temporary fields standing in for properties
	// during type checking, keyed by class then property name.
	PropertyFields map[*Class]map[string]*goast.Field
}

// ImplementsRef ties a lowered interface type expression to its source.
type ImplementsRef struct {
	Expr   goast.Expr
	Source ast.Expr
	Name   string
}

// TypeInfo is the result of type checking the lowered program.
type TypeInfo struct {
	Pkg  *types.Package
	Info *types.Info
}
