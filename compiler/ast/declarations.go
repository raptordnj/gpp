package ast

import "go/token"

// BadDecl is a placeholder for syntax errors.
type BadDecl struct{ From Pos }

// ImportSpec is a single import.
type ImportSpec struct {
	Name *Ident // alias, "." or "_"; nil if absent
	Path *BasicLit
}

// ImportDeclaration is the specification's name for import specs.
type ImportDeclaration = ImportSpec

// ValueSpec is a var or const spec.
type ValueSpec struct {
	Names  []*Ident
	Type   Expr
	Values []Expr
}

// TypeSpec is a type spec.
type TypeSpec struct {
	Name       *Ident
	TypeParams *FieldList
	Assign     bool // alias declaration
	Type       Expr
}

// Spec is *ImportSpec, *ValueSpec or *TypeSpec.
type Spec interface{ Node }

// GenDecl is an import, const, type or var declaration.
type GenDecl struct {
	TokPos Pos
	Tok    token.Token // IMPORT, CONST, TYPE, VAR
	Lparen bool
	Specs  []Spec
}

// FuncDecl is a Go function or method declared with func or function.
type FuncDecl struct {
	Recv *FieldList // nil for functions
	Name *Ident
	Type *FuncType
	Body *BlockStmt // nil for external functions
}

// FunctionDeclaration is the specification's name for function declarations.
type FunctionDeclaration = FuncDecl

// Access is a class member access modifier.
type Access int

// Access levels. AccessDefault means no modifier was written; it behaves as
// public.
const (
	AccessDefault Access = iota
	Public
	Private
	Protected
	Internal
)

func (a Access) String() string {
	switch a {
	case Private:
		return "private"
	case Protected:
		return "protected"
	case Internal:
		return "internal"
	}
	return "public"
}

// Modifiers are the modifiers written before a class member.
type Modifiers struct {
	Pos      Pos
	Access   Access
	Static   bool
	Abstract bool
	Override bool
}

// FieldDecl is a class field: [modifiers] name(s) Type [= value].
type FieldDecl struct {
	Mods  Modifiers
	Names []*Ident
	Type  Expr
	Value Expr // optional initializer
}

// MethodDecl is a class method.
type MethodDecl struct {
	Mods Modifiers
	Name *Ident
	Type *FuncType
	Body *BlockStmt // nil for abstract methods
}

// ConstructorDecl is a class constructor.
type ConstructorDecl struct {
	Mods           Modifiers
	ConstructorPos Pos
	Type           *FuncType
	Body           *BlockStmt
}

// PropertyDecl is a property with get/set accessors.
type PropertyDecl struct {
	Mods   Modifiers
	Name   *Ident
	Type   Expr
	Getter *BlockStmt // may be nil
	Setter *BlockStmt // may be nil; the new value is named "value"
}

// ClassDecl is a G++ class.
type ClassDecl struct {
	ClassPos   Pos
	Abstract   bool
	Name       *Ident
	TypeParams *FieldList // nil if not generic
	Extends    Expr       // parent class type; nil if none
	Implements []Expr
	Fields     []*FieldDecl
	Methods    []*MethodDecl
	Ctor       *ConstructorDecl // nil when no explicit constructor
	Properties []*PropertyDecl
}

// ClassDeclaration is the specification's name for classes.
type ClassDeclaration = ClassDecl

// Specification names for class members.
type (
	FieldDeclaration       = FieldDecl
	MethodDeclaration      = MethodDecl
	ConstructorDeclaration = ConstructorDecl
)

// InterfaceDecl is a G++-style interface declaration: interface Name { ... }.
type InterfaceDecl struct {
	InterfacePos Pos
	Name         *Ident
	TypeParams   *FieldList
	Methods      *FieldList // methods (Name + *FuncType) and embedded types
}

// InterfaceDeclaration is the specification's name for interfaces.
type InterfaceDeclaration = InterfaceDecl

// EnumDecl is an enum declaration.
type EnumDecl struct {
	EnumPos Pos
	Name    *Ident
	Members []*Ident
}

// EnumDeclaration is the specification's name for enums.
type EnumDeclaration = EnumDecl

func (d *BadDecl) Pos() Pos { return d.From }
func (s *ImportSpec) Pos() Pos {
	if s.Name != nil {
		return s.Name.Pos()
	}
	return s.Path.Pos()
}
func (s *ValueSpec) Pos() Pos       { return s.Names[0].Pos() }
func (s *TypeSpec) Pos() Pos        { return s.Name.Pos() }
func (d *GenDecl) Pos() Pos         { return d.TokPos }
func (d *FuncDecl) Pos() Pos        { return d.Type.Pos() }
func (d *FieldDecl) Pos() Pos       { return d.Names[0].Pos() }
func (d *MethodDecl) Pos() Pos      { return d.Name.Pos() }
func (d *ConstructorDecl) Pos() Pos { return d.ConstructorPos }
func (d *PropertyDecl) Pos() Pos    { return d.Name.Pos() }
func (d *ClassDecl) Pos() Pos       { return d.ClassPos }
func (d *InterfaceDecl) Pos() Pos   { return d.InterfacePos }
func (d *EnumDecl) Pos() Pos        { return d.EnumPos }
func (*BadDecl) declNode()          {}
func (*GenDecl) declNode()          {}
func (*FuncDecl) declNode()         {}
func (*ClassDecl) declNode()        {}
func (*InterfaceDecl) declNode()    {}
func (*EnumDecl) declNode()         {}

// Walk calls fn for every node of the tree rooted at n in depth-first order.
// If fn returns false the children of that node are skipped.
func Walk(n Node, fn func(Node) bool) {
	walk(n, fn)
}
