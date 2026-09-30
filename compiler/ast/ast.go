// Package ast declares the typed syntax tree for G++ programs.
//
// The Go subset mirrors the structure of the standard go/ast package so that
// lowering to Go is a direct mapping; G++ extensions (classes, interfaces,
// enums, new-expressions, super) have dedicated nodes.
package ast

import "github.com/raptordnj/gpp/compiler/diag"

// Pos is a byte offset into the source file.
type Pos = diag.Pos

// Node is any syntax tree node.
type Node interface {
	Pos() Pos
}

// Expr is an expression or type node.
type Expr interface {
	Node
	exprNode()
}

// Stmt is a statement node.
type Stmt interface {
	Node
	stmtNode()
}

// Decl is a top-level declaration node.
type Decl interface {
	Node
	declNode()
}

// File is a whole G++ compilation unit (the "Program").
type File struct {
	Source  *diag.File
	Package *PackageDecl
	Imports []*ImportSpec // all imports, for convenience
	Decls   []Decl
}

// Program is an alias used by the language specification.
type Program = File

// PackageDecl is the package clause.
type PackageDecl struct {
	PackagePos Pos
	Name       *Ident
}

// Pos returns the position of the package keyword.
func (d *PackageDecl) Pos() Pos { return d.PackagePos }

// Field is a parameter, result, struct field or interface element.
type Field struct {
	Names []*Ident // nil for anonymous parameters, embedded fields and interface embeddings
	Type  Expr
	Tag   *BasicLit
}

// Pos returns the position of the first name or the type.
func (f *Field) Pos() Pos {
	if len(f.Names) > 0 {
		return f.Names[0].Pos()
	}
	return f.Type.Pos()
}

// FieldList is a parenthesized, bracketed or braced list of fields.
type FieldList struct {
	Opening Pos
	List    []*Field
}

// Pos returns the position of the opening delimiter.
func (l *FieldList) Pos() Pos { return l.Opening }

// NumFields counts the fields, expanding grouped names.
func (l *FieldList) NumFields() int {
	if l == nil {
		return 0
	}
	n := 0
	for _, f := range l.List {
		if len(f.Names) == 0 {
			n++
		} else {
			n += len(f.Names)
		}
	}
	return n
}

// Pos returns the position of the package clause.
func (f *File) Pos() Pos {
	if f.Package != nil {
		return f.Package.Pos()
	}
	return 0
}
