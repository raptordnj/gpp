// Package compiler drives the G++ pipeline:
//
//	.gpp source → lexer → parser → AST → semantic analysis →
//	OOP lowering (go/ast) → type checking (go/types) → gofmt'ed Go source
package compiler

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gpp/compiler/ast"
	"gpp/compiler/codegen"
	"gpp/compiler/diag"
	"gpp/compiler/parser"
	"gpp/compiler/semantic"
)

// Version is the compiler version.
const Version = "0.1.0"

// SpecVersion is the language specification version implemented.
const SpecVersion = "v0.1"

// Source is a G++ source file.
type Source struct {
	Name string // path used in diagnostics
	Src  []byte
}

// GoFile is a generated Go file.
type GoFile struct {
	Source string // originating .gpp file
	Name   string // file name of the generated Go file (e.g. main.go)
	Code   []byte
}

// Result is the output of a successful compilation.
type Result struct {
	Package string
	Files   []GoFile
	// TypeCheckIncomplete is set when some imports could not be loaded,
	// so part of type checking is deferred to the Go toolchain.
	TypeCheckIncomplete bool
}

// Compile compiles the given sources, which must form a single package.
// On failure the returned error is a diag.ErrorList with .gpp positions.
func Compile(sources []Source) (*Result, error) {
	var errs diag.ErrorList
	var files []*ast.File
	for _, s := range sources {
		f, err := parser.ParseFile(diag.NewFile(s.Name, s.Src))
		if err != nil {
			if l, ok := err.(diag.ErrorList); ok {
				errs = append(errs, l...)
				continue
			}
			return nil, err
		}
		files = append(files, f)
	}
	if len(errs) > 0 {
		return nil, errs
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no G++ source files")
	}
	pkg := files[0].Package.Name.Name
	for _, f := range files[1:] {
		if f.Package.Name.Name != pkg {
			errs.Add(f.Source.Position(f.Package.Name.Pos()), "package %s; expected package %s", f.Package.Name.Name, pkg)
		}
	}
	if len(errs) > 0 {
		return nil, errs
	}

	prog, errs := semantic.Analyze(files)
	if len(errs) > 0 {
		return nil, errs
	}

	out := codegen.Lower(prog)
	tc := semantic.TypeCheck(out.Fset, out.Files, prog, out.Info)
	if len(tc.Errors) > 0 {
		return nil, tc.Errors
	}
	if out.HasProperties() {
		if errs := codegen.RewriteProperties(out, prog, tc.TypeInfo); len(errs) > 0 {
			return nil, errs
		}
		tc = semantic.TypeCheck(out.Fset, out.Files, prog, out.Info)
		if len(tc.Errors) > 0 {
			return nil, tc.Errors
		}
	}

	res := &Result{Package: pkg, TypeCheckIncomplete: tc.Incomplete}
	for i, f := range out.Files {
		code, err := codegen.Print(f)
		if err != nil {
			return nil, fmt.Errorf("%s: internal error: %v", sources[i].Name, err)
		}
		base := strings.TrimSuffix(filepath.Base(sources[i].Name), filepath.Ext(sources[i].Name))
		res.Files = append(res.Files, GoFile{Source: sources[i].Name, Name: base + ".go", Code: code})
	}
	return res, nil
}

// CompileSource compiles a single in-memory source file.
func CompileSource(name string, src []byte) (*Result, error) {
	return Compile([]Source{{Name: name, Src: src}})
}

// ExpandPaths turns command-line arguments (files or directories) into the
// list of .gpp files to compile.
func ExpandPaths(args []string) ([]string, error) {
	var out []string
	for _, a := range args {
		st, err := os.Stat(a)
		if err != nil {
			return nil, err
		}
		if !st.IsDir() {
			out = append(out, a)
			continue
		}
		matches, err := filepath.Glob(filepath.Join(a, "*.gpp"))
		if err != nil {
			return nil, err
		}
		if len(matches) == 0 {
			return nil, fmt.Errorf("no .gpp files in %s", a)
		}
		sort.Strings(matches)
		out = append(out, matches...)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no input files")
	}
	return out, nil
}

// LoadSources reads the files named by paths.
func LoadSources(paths []string) ([]Source, error) {
	var srcs []Source
	for _, p := range paths {
		if filepath.Ext(p) != ".gpp" {
			return nil, fmt.Errorf("%s: not a G++ source file (expected .gpp extension)", p)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		srcs = append(srcs, Source{Name: p, Src: b})
	}
	return srcs, nil
}
