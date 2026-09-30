package ast

import "reflect"

func walkList[T Node](list []T, fn func(Node) bool) {
	for _, n := range list {
		walk(n, fn)
	}
}

func walkFields(l *FieldList, fn func(Node) bool) {
	if l != nil {
		walk(l, fn)
	}
}

func walk(n Node, fn func(Node) bool) {
	if n == nil || isNil(n) || !fn(n) {
		return
	}
	switch n := n.(type) {
	case *File:
		walkList(n.Decls, fn)
	case *Field:
		walkList(n.Names, fn)
		walk(n.Type, fn)
		if n.Tag != nil {
			walk(n.Tag, fn)
		}
	case *FieldList:
		walkList(n.List, fn)

	// expressions
	case *CompositeLit:
		if n.Type != nil {
			walk(n.Type, fn)
		}
		walkList(n.Elts, fn)
	case *FuncLit:
		walk(n.Type, fn)
		walk(n.Body, fn)
	case *ParenExpr:
		walk(n.X, fn)
	case *SelectorExpr:
		walk(n.X, fn)
		walk(n.Sel, fn)
	case *IndexExpr:
		walk(n.X, fn)
		walkList(n.Indices, fn)
	case *SliceExpr:
		walk(n.X, fn)
		walk(n.Low, fn)
		walk(n.High, fn)
		walk(n.Max, fn)
	case *TypeAssertExpr:
		walk(n.X, fn)
		walk(n.Type, fn)
	case *CallExpr:
		walk(n.Fun, fn)
		walkList(n.Args, fn)
	case *StarExpr:
		walk(n.X, fn)
	case *UnaryExpr:
		walk(n.X, fn)
	case *BinaryExpr:
		walk(n.X, fn)
		walk(n.Y, fn)
	case *KeyValueExpr:
		walk(n.Key, fn)
		walk(n.Value, fn)
	case *NewExpr:
		walk(n.Type, fn)
		walkList(n.TypeArgs, fn)
		walkList(n.Args, fn)
	case *ArrayType:
		walk(n.Len, fn)
		walk(n.Elt, fn)
	case *StructType:
		walkFields(n.Fields, fn)
	case *FuncType:
		walkFields(n.TypeParams, fn)
		walkFields(n.Params, fn)
		walkFields(n.Results, fn)
	case *InterfaceType:
		walkFields(n.Methods, fn)
	case *MapType:
		walk(n.Key, fn)
		walk(n.Value, fn)
	case *ChanType:
		walk(n.Value, fn)
	case *Ellipsis:
		walk(n.Elt, fn)

	// statements
	case *DeclStmt:
		walk(n.Decl, fn)
	case *LabeledStmt:
		walk(n.Label, fn)
		walk(n.Stmt, fn)
	case *ExprStmt:
		walk(n.X, fn)
	case *SendStmt:
		walk(n.Chan, fn)
		walk(n.Value, fn)
	case *IncDecStmt:
		walk(n.X, fn)
	case *AssignStmt:
		walkList(n.Lhs, fn)
		walkList(n.Rhs, fn)
	case *GoStmt:
		walk(n.Call, fn)
	case *DeferStmt:
		walk(n.Call, fn)
	case *ReturnStmt:
		walkList(n.Results, fn)
	case *BranchStmt:
		walk(n.Label, fn)
	case *BlockStmt:
		walkList(n.List, fn)
	case *IfStmt:
		walk(n.Init, fn)
		walk(n.Cond, fn)
		walk(n.Body, fn)
		walk(n.Else, fn)
	case *CaseClause:
		walkList(n.List, fn)
		walkList(n.Body, fn)
	case *SwitchStmt:
		walk(n.Init, fn)
		walk(n.Tag, fn)
		walk(n.Body, fn)
	case *TypeSwitchStmt:
		walk(n.Init, fn)
		walk(n.Assign, fn)
		walk(n.Body, fn)
	case *CommClause:
		walk(n.Comm, fn)
		walkList(n.Body, fn)
	case *SelectStmt:
		walk(n.Body, fn)
	case *ForStmt:
		walk(n.Init, fn)
		walk(n.Cond, fn)
		walk(n.Post, fn)
		walk(n.Body, fn)
	case *RangeStmt:
		walk(n.Key, fn)
		walk(n.Value, fn)
		walk(n.X, fn)
		walk(n.Body, fn)

	// declarations
	case *ImportSpec:
		walk(n.Name, fn)
		walk(n.Path, fn)
	case *ValueSpec:
		walkList(n.Names, fn)
		walk(n.Type, fn)
		walkList(n.Values, fn)
	case *TypeSpec:
		walk(n.Name, fn)
		walkFields(n.TypeParams, fn)
		walk(n.Type, fn)
	case *GenDecl:
		walkList(n.Specs, fn)
	case *FuncDecl:
		walkFields(n.Recv, fn)
		walk(n.Name, fn)
		walk(n.Type, fn)
		walk(n.Body, fn)
	case *ClassDecl:
		walk(n.Name, fn)
		walkFields(n.TypeParams, fn)
		walk(n.Extends, fn)
		walkList(n.Implements, fn)
		walkList(n.Fields, fn)
		walk(n.Ctor, fn)
		walkList(n.Properties, fn)
		walkList(n.Methods, fn)
	case *FieldDecl:
		walkList(n.Names, fn)
		walk(n.Type, fn)
		walk(n.Value, fn)
	case *MethodDecl:
		walk(n.Name, fn)
		walk(n.Type, fn)
		walk(n.Body, fn)
	case *ConstructorDecl:
		walk(n.Type, fn)
		walk(n.Body, fn)
	case *PropertyDecl:
		walk(n.Name, fn)
		walk(n.Type, fn)
		walk(n.Getter, fn)
		walk(n.Setter, fn)
	case *InterfaceDecl:
		walk(n.Name, fn)
		walkFields(n.TypeParams, fn)
		walkFields(n.Methods, fn)
	case *EnumDecl:
		walk(n.Name, fn)
		walkList(n.Members, fn)
	}
}

// isNil reports whether an interface holds a typed nil pointer.
func isNil(n Node) bool {
	v := reflect.ValueOf(n)
	return v.Kind() == reflect.Pointer && v.IsNil()
}
