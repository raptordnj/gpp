package ast

import "go/token"

// BadStmt is a placeholder for syntax errors.
type BadStmt struct{ From Pos }

// DeclStmt is a var, const or type declaration inside a function.
type DeclStmt struct{ Decl *GenDecl }

// VariableDeclaration is the specification's name for local declarations.
type VariableDeclaration = DeclStmt

// EmptyStmt is an empty statement.
type EmptyStmt struct{ Semicolon Pos }

// LabeledStmt is label: stmt.
type LabeledStmt struct {
	Label *Ident
	Stmt  Stmt
}

// ExprStmt is an expression used as a statement.
type ExprStmt struct{ X Expr }

// SendStmt is ch <- v.
type SendStmt struct {
	Chan  Expr
	Arrow Pos
	Value Expr
}

// IncDecStmt is x++ or x--.
type IncDecStmt struct {
	X   Expr
	Tok token.Token
}

// AssignStmt is an assignment or short variable declaration.
type AssignStmt struct {
	Lhs    []Expr
	TokPos Pos
	Tok    token.Token
	Rhs    []Expr
}

// GoStmt is go f().
type GoStmt struct {
	Go   Pos
	Call *CallExpr
}

// DeferStmt is defer f().
type DeferStmt struct {
	Defer Pos
	Call  *CallExpr
}

// ReturnStmt is a return statement.
type ReturnStmt struct {
	Return  Pos
	Results []Expr
}

// BranchStmt is break, continue, goto or fallthrough.
type BranchStmt struct {
	TokPos Pos
	Tok    token.Token
	Label  *Ident
}

// BlockStmt is a braced statement list.
type BlockStmt struct {
	Lbrace Pos
	List   []Stmt
}

// IfStmt is an if statement.
type IfStmt struct {
	If   Pos
	Init Stmt
	Cond Expr
	Body *BlockStmt
	Else Stmt // *IfStmt or *BlockStmt
}

// CaseClause is a case of an expression or type switch.
type CaseClause struct {
	Case Pos
	List []Expr // nil for default
	Body []Stmt
}

// SwitchStmt is an expression switch.
type SwitchStmt struct {
	Switch Pos
	Init   Stmt
	Tag    Expr
	Body   *BlockStmt // of *CaseClause
}

// TypeSwitchStmt is a type switch.
type TypeSwitchStmt struct {
	Switch Pos
	Init   Stmt
	Assign Stmt // x := y.(type) or y.(type)
	Body   *BlockStmt
}

// CommClause is a case of a select statement.
type CommClause struct {
	Case Pos
	Comm Stmt // nil for default
	Body []Stmt
}

// SelectStmt is a select statement.
type SelectStmt struct {
	Select Pos
	Body   *BlockStmt // of *CommClause
}

// ForStmt is a for loop.
type ForStmt struct {
	For  Pos
	Init Stmt
	Cond Expr
	Post Stmt
	Body *BlockStmt
}

// RangeStmt is a for-range loop.
type RangeStmt struct {
	For        Pos
	Key, Value Expr // may be nil
	TokPos     Pos
	Tok        token.Token // ILLEGAL when Key is nil
	X          Expr
	Body       *BlockStmt
}

// Specification names for statements.
type (
	BlockStatement  = BlockStmt
	IfStatement     = IfStmt
	ForStatement    = ForStmt
	ReturnStatement = ReturnStmt
)

func (s *BadStmt) Pos() Pos        { return s.From }
func (s *DeclStmt) Pos() Pos       { return s.Decl.Pos() }
func (s *EmptyStmt) Pos() Pos      { return s.Semicolon }
func (s *LabeledStmt) Pos() Pos    { return s.Label.Pos() }
func (s *ExprStmt) Pos() Pos       { return s.X.Pos() }
func (s *SendStmt) Pos() Pos       { return s.Chan.Pos() }
func (s *IncDecStmt) Pos() Pos     { return s.X.Pos() }
func (s *AssignStmt) Pos() Pos     { return s.Lhs[0].Pos() }
func (s *GoStmt) Pos() Pos         { return s.Go }
func (s *DeferStmt) Pos() Pos      { return s.Defer }
func (s *ReturnStmt) Pos() Pos     { return s.Return }
func (s *BranchStmt) Pos() Pos     { return s.TokPos }
func (s *BlockStmt) Pos() Pos      { return s.Lbrace }
func (s *IfStmt) Pos() Pos         { return s.If }
func (s *CaseClause) Pos() Pos     { return s.Case }
func (s *SwitchStmt) Pos() Pos     { return s.Switch }
func (s *TypeSwitchStmt) Pos() Pos { return s.Switch }
func (s *CommClause) Pos() Pos     { return s.Case }
func (s *SelectStmt) Pos() Pos     { return s.Select }
func (s *ForStmt) Pos() Pos        { return s.For }
func (s *RangeStmt) Pos() Pos      { return s.For }

func (*BadStmt) stmtNode()        {}
func (*DeclStmt) stmtNode()       {}
func (*EmptyStmt) stmtNode()      {}
func (*LabeledStmt) stmtNode()    {}
func (*ExprStmt) stmtNode()       {}
func (*SendStmt) stmtNode()       {}
func (*IncDecStmt) stmtNode()     {}
func (*AssignStmt) stmtNode()     {}
func (*GoStmt) stmtNode()         {}
func (*DeferStmt) stmtNode()      {}
func (*ReturnStmt) stmtNode()     {}
func (*BranchStmt) stmtNode()     {}
func (*BlockStmt) stmtNode()      {}
func (*IfStmt) stmtNode()         {}
func (*CaseClause) stmtNode()     {}
func (*SwitchStmt) stmtNode()     {}
func (*TypeSwitchStmt) stmtNode() {}
func (*CommClause) stmtNode()     {}
func (*SelectStmt) stmtNode()     {}
func (*ForStmt) stmtNode()        {}
func (*RangeStmt) stmtNode()      {}
