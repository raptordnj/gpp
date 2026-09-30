// Package codegen lowers a G++ syntax tree to Go syntax trees (go/ast) and
// prints them as gofmt-formatted Go source.
//
// Positions of generated Go nodes are expressed in a token.FileSet whose
// files mirror the original .gpp sources, so that go/types diagnostics point
// at .gpp lines and columns.
package codegen

import (
	"bytes"
	"fmt"
	goast "go/ast"
	"go/format"
	"go/token"
	"reflect"
	"strconv"

	"gpp/compiler/ast"
	"gpp/compiler/semantic"
)

// Output is the result of lowering one package.
type Output struct {
	Fset  *token.FileSet
	Files []*goast.File // one per input file, in order
	Info  *semantic.LoweringInfo
}

// Generator lowers G++ files to Go.
type Generator struct {
	prog *semantic.Program
	fset *token.FileSet
	info *semantic.LoweringInfo

	tf *token.File // file being generated

	// per-function state
	class *semantic.Class
}

// Lower converts all files of a G++ program to Go syntax trees.
func Lower(prog *semantic.Program) *Output {
	g := &Generator{
		prog: prog,
		fset: token.NewFileSet(),
		info: &semantic.LoweringInfo{
			FuncClass:      map[*goast.FuncDecl]*semantic.Class{},
			Implements:     map[*semantic.Class][]semantic.ImplementsRef{},
			PropertyFields: map[*semantic.Class]map[string]*goast.Field{},
		},
	}
	out := &Output{Fset: g.fset, Info: g.info}
	for _, f := range prog.Files {
		out.Files = append(out.Files, g.file(f))
	}
	return out
}

// pos maps a G++ position to a position in the generated file set.
func (g *Generator) pos(p ast.Pos) token.Pos {
	if !p.IsValid() || int(p) > g.tf.Size() {
		return token.NoPos
	}
	return g.tf.Pos(int(p))
}

func (g *Generator) file(f *ast.File) *goast.File {
	g.tf = g.fset.AddFile(f.Source.Name, -1, len(f.Source.Src)+1)
	g.tf.SetLinesForContent(f.Source.Src)
	out := &goast.File{
		Package: g.pos(f.Package.PackagePos),
		Name:    g.ident(f.Package.Name),
	}
	for _, d := range f.Decls {
		out.Decls = append(out.Decls, g.decl(d)...)
	}
	for _, d := range out.Decls {
		if gd, ok := d.(*goast.GenDecl); ok && gd.Tok == token.IMPORT {
			for _, s := range gd.Specs {
				out.Imports = append(out.Imports, s.(*goast.ImportSpec))
			}
		}
	}
	return out
}

func (g *Generator) decl(d ast.Decl) []goast.Decl {
	switch d := d.(type) {
	case *ast.GenDecl:
		return []goast.Decl{g.genDecl(d)}
	case *ast.FuncDecl:
		return []goast.Decl{g.funcDecl(d)}
	case *ast.ClassDecl:
		return g.classDecl(d)
	case *ast.InterfaceDecl:
		return []goast.Decl{g.interfaceDecl(d)}
	case *ast.EnumDecl:
		return g.enumDecl(d)
	}
	panic(fmt.Sprintf("codegen: unexpected declaration %T", d))
}

func (g *Generator) genDecl(d *ast.GenDecl) *goast.GenDecl {
	out := &goast.GenDecl{TokPos: g.pos(d.TokPos), Tok: d.Tok}
	if d.Lparen || len(d.Specs) != 1 {
		out.Lparen = g.pos(d.TokPos)
		out.Rparen = out.Lparen
	}
	for _, s := range d.Specs {
		switch s := s.(type) {
		case *ast.ImportSpec:
			spec := &goast.ImportSpec{Path: g.basicLit(s.Path)}
			if s.Name != nil {
				spec.Name = g.ident(s.Name)
			}
			out.Specs = append(out.Specs, spec)
		case *ast.ValueSpec:
			out.Specs = append(out.Specs, &goast.ValueSpec{
				Names:  g.idents(s.Names),
				Type:   g.exprOrNil(s.Type),
				Values: g.exprs(s.Values),
			})
		case *ast.TypeSpec:
			spec := &goast.TypeSpec{Name: g.ident(s.Name), TypeParams: g.fieldList(s.TypeParams), Type: g.expr(s.Type)}
			if s.Assign {
				spec.Assign = g.pos(s.Name.Pos())
			}
			out.Specs = append(out.Specs, spec)
		}
	}
	return out
}

func (g *Generator) funcDecl(d *ast.FuncDecl) *goast.FuncDecl {
	g.class = nil
	return &goast.FuncDecl{
		Recv: g.fieldList(d.Recv),
		Name: g.ident(d.Name),
		Type: g.funcType(d.Type),
		Body: g.blockOrNil(d.Body),
	}
}

// ---------------------------------------------------------------------------
// helpers

func (g *Generator) ident(id *ast.Ident) *goast.Ident {
	return &goast.Ident{NamePos: g.pos(id.NamePos), Name: id.Name}
}

func (g *Generator) idents(ids []*ast.Ident) []*goast.Ident {
	var out []*goast.Ident
	for _, id := range ids {
		out = append(out, g.ident(id))
	}
	return out
}

func newIdent(name string, pos token.Pos) *goast.Ident {
	return &goast.Ident{NamePos: pos, Name: name}
}

func (g *Generator) basicLit(b *ast.BasicLit) *goast.BasicLit {
	return &goast.BasicLit{ValuePos: g.pos(b.ValuePos), Kind: b.Kind, Value: b.Value}
}

func stringLit(s string) *goast.BasicLit {
	return &goast.BasicLit{Kind: token.STRING, Value: strconv.Quote(s)}
}

func (g *Generator) fieldList(l *ast.FieldList) *goast.FieldList {
	if l == nil {
		return nil
	}
	out := &goast.FieldList{Opening: g.pos(l.Opening)}
	for _, f := range l.List {
		out.List = append(out.List, g.field(f))
	}
	return out
}

func (g *Generator) field(f *ast.Field) *goast.Field {
	out := &goast.Field{Names: g.idents(f.Names), Type: g.expr(f.Type)}
	if f.Tag != nil {
		out.Tag = g.basicLit(f.Tag)
	}
	return out
}

func (g *Generator) funcType(t *ast.FuncType) *goast.FuncType {
	return &goast.FuncType{
		Func:       g.pos(t.Func),
		TypeParams: g.fieldList(t.TypeParams),
		Params:     g.paramsOrEmpty(t.Params),
		Results:    g.fieldList(t.Results),
	}
}

func (g *Generator) paramsOrEmpty(l *ast.FieldList) *goast.FieldList {
	if l == nil {
		return &goast.FieldList{}
	}
	return g.fieldList(l)
}

// ---------------------------------------------------------------------------
// printing

// Print formats a generated Go file as gofmt-compatible source.
func Print(f *goast.File) ([]byte, error) {
	stripPositions(f)
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "// Code generated by gpp (G++ compiler). DO NOT EDIT.\n\npackage %s\n", f.Name.Name)
	for _, d := range f.Decls {
		buf.WriteString("\n")
		if err := format.Node(&buf, token.NewFileSet(), d); err != nil {
			return nil, err
		}
		buf.WriteString("\n")
	}
	src, err := format.Source(buf.Bytes())
	if err != nil {
		return buf.Bytes(), fmt.Errorf("formatting generated Go: %w", err)
	}
	return src, nil
}

var posType = reflect.TypeOf(token.NoPos)

var meaningfulPos = map[string]bool{"Ellipsis": true, "Assign": true, "Lparen": true, "Rparen": true}

// stripPositions clears all positions so the printer lays the code out
// canonically instead of following the unrelated .gpp positions.
func stripPositions(n goast.Node) {
	goast.Inspect(n, func(n goast.Node) bool {
		if n == nil {
			return false
		}
		v := reflect.ValueOf(n)
		if v.Kind() != reflect.Pointer || v.IsNil() {
			return true
		}
		v = v.Elem()
		if v.Kind() != reflect.Struct {
			return true
		}
		for i := 0; i < v.NumField(); i++ {
			f := v.Field(i)
			if f.Type() != posType || !f.CanSet() {
				continue
			}
			// Some positions carry syntax: f(xs...), type A = B and
			// parenthesized declarations. Keep them valid but neutral.
			if f.Int() != 0 && meaningfulPos[v.Type().Field(i).Name] {
				f.SetInt(1)
			} else {
				f.SetInt(0)
			}
		}
		return true
	})
}
