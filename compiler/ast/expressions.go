package ast

import "go/token"

// Ident is an identifier.
type Ident struct {
	NamePos Pos
	Name    string
}

// BasicLit is a literal of basic type.
type BasicLit struct {
	ValuePos Pos
	Kind     token.Token // INT, FLOAT, IMAG, CHAR or STRING
	Value    string
}

// CompositeLit is a composite literal: T{...}.
type CompositeLit struct {
	Type   Expr // may be nil for elided types
	Lbrace Pos
	Elts   []Expr
}

// FuncLit is a function literal.
type FuncLit struct {
	Type *FuncType
	Body *BlockStmt
}

// ParenExpr is a parenthesized expression.
type ParenExpr struct {
	Lparen Pos
	X      Expr
}

// SelectorExpr is x.Sel. It is also used for member access (MemberExpression).
type SelectorExpr struct {
	X   Expr
	Sel *Ident
}

// MemberExpression is the specification's name for selector expressions.
type MemberExpression = SelectorExpr

// IndexExpr is x[i] or a generic instantiation x[T1, T2].
type IndexExpr struct {
	X       Expr
	Lbrack  Pos
	Indices []Expr
}

// SliceExpr is x[lo:hi] or x[lo:hi:max].
type SliceExpr struct {
	X      Expr
	Lbrack Pos
	Low    Expr
	High   Expr
	Max    Expr
	Slice3 bool
}

// TypeAssertExpr is x.(T); Type is nil for x.(type) in type switches.
type TypeAssertExpr struct {
	X    Expr
	Type Expr
}

// CallExpr is a function call.
type CallExpr struct {
	Fun      Expr
	Lparen   Pos
	Args     []Expr
	Ellipsis bool // f(xs...)
}

// CallExpression is the specification's name for call expressions.
type CallExpression = CallExpr

// StarExpr is *x (dereference or pointer type).
type StarExpr struct {
	Star Pos
	X    Expr
}

// UnaryExpr is a unary operation (including &x and <-ch).
type UnaryExpr struct {
	OpPos Pos
	Op    token.Token
	X     Expr
}

// UnaryExpression is the specification's name for unary expressions.
type UnaryExpression = UnaryExpr

// BinaryExpr is a binary operation.
type BinaryExpr struct {
	X     Expr
	OpPos Pos
	Op    token.Token
	Y     Expr
}

// BinaryExpression is the specification's name for binary expressions.
type BinaryExpression = BinaryExpr

// KeyValueExpr is key: value inside composite literals.
type KeyValueExpr struct {
	Key   Expr
	Colon Pos
	Value Expr
}

// NewExpr is the G++ constructor call: new Type(args) or new Type<T>(args).
type NewExpr struct {
	NewPos   Pos
	Type     Expr   // *Ident or *SelectorExpr (pkg.Class)
	TypeArgs []Expr // generic type arguments, if any
	Args     []Expr
	Ellipsis bool
}

// NewExpression is the specification's name for G++ constructor calls.
type NewExpression = NewExpr

// SuperExpr is the G++ keyword super, used as super(...) or super.Member.
type SuperExpr struct {
	SuperPos Pos
}

// Type expressions.

// ArrayType is [Len]Elt, or []Elt when Len is nil.
type ArrayType struct {
	Lbrack Pos
	Len    Expr // nil for slices; *Ellipsis for [...]T
	Elt    Expr
}

// StructType is struct{...}.
type StructType struct {
	Struct Pos
	Fields *FieldList
}

// FuncType is a function signature.
type FuncType struct {
	Func       Pos
	TypeParams *FieldList
	Params     *FieldList
	Results    *FieldList
}

// InterfaceType is interface{...}.
type InterfaceType struct {
	Interface Pos
	Methods   *FieldList
}

// MapType is map[K]V.
type MapType struct {
	Map   Pos
	Key   Expr
	Value Expr
}

// ChanDir is a channel direction.
type ChanDir int

// Channel directions.
const (
	SEND ChanDir = 1 << iota
	RECV
)

// ChanType is chan T, chan<- T or <-chan T.
type ChanType struct {
	Begin Pos
	Dir   ChanDir
	Value Expr
}

// Ellipsis is ...T in variadic parameters or [...] in array literals.
type Ellipsis struct {
	EllipsisPos Pos
	Elt         Expr
}

// BadExpr is a placeholder for syntax errors.
type BadExpr struct {
	From Pos
}

func (x *Ident) Pos() Pos    { return x.NamePos }
func (x *BasicLit) Pos() Pos { return x.ValuePos }
func (x *CompositeLit) Pos() Pos {
	if x.Type != nil {
		return x.Type.Pos()
	}
	return x.Lbrace
}
func (x *FuncLit) Pos() Pos        { return x.Type.Pos() }
func (x *ParenExpr) Pos() Pos      { return x.Lparen }
func (x *SelectorExpr) Pos() Pos   { return x.X.Pos() }
func (x *IndexExpr) Pos() Pos      { return x.X.Pos() }
func (x *SliceExpr) Pos() Pos      { return x.X.Pos() }
func (x *TypeAssertExpr) Pos() Pos { return x.X.Pos() }
func (x *CallExpr) Pos() Pos       { return x.Fun.Pos() }
func (x *StarExpr) Pos() Pos       { return x.Star }
func (x *UnaryExpr) Pos() Pos      { return x.OpPos }
func (x *BinaryExpr) Pos() Pos     { return x.X.Pos() }
func (x *KeyValueExpr) Pos() Pos   { return x.Key.Pos() }
func (x *NewExpr) Pos() Pos        { return x.NewPos }
func (x *SuperExpr) Pos() Pos      { return x.SuperPos }
func (x *ArrayType) Pos() Pos      { return x.Lbrack }
func (x *StructType) Pos() Pos     { return x.Struct }
func (x *FuncType) Pos() Pos       { return x.Func }
func (x *InterfaceType) Pos() Pos  { return x.Interface }
func (x *MapType) Pos() Pos        { return x.Map }
func (x *ChanType) Pos() Pos       { return x.Begin }
func (x *Ellipsis) Pos() Pos       { return x.EllipsisPos }
func (x *BadExpr) Pos() Pos        { return x.From }

func (*Ident) exprNode()          {}
func (*BasicLit) exprNode()       {}
func (*CompositeLit) exprNode()   {}
func (*FuncLit) exprNode()        {}
func (*ParenExpr) exprNode()      {}
func (*SelectorExpr) exprNode()   {}
func (*IndexExpr) exprNode()      {}
func (*SliceExpr) exprNode()      {}
func (*TypeAssertExpr) exprNode() {}
func (*CallExpr) exprNode()       {}
func (*StarExpr) exprNode()       {}
func (*UnaryExpr) exprNode()      {}
func (*BinaryExpr) exprNode()     {}
func (*KeyValueExpr) exprNode()   {}
func (*NewExpr) exprNode()        {}
func (*SuperExpr) exprNode()      {}
func (*ArrayType) exprNode()      {}
func (*StructType) exprNode()     {}
func (*FuncType) exprNode()       {}
func (*InterfaceType) exprNode()  {}
func (*MapType) exprNode()        {}
func (*ChanType) exprNode()       {}
func (*Ellipsis) exprNode()       {}
func (*BadExpr) exprNode()        {}
