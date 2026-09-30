package integration

import (
	"bytes"
	goast "go/ast"
	"go/format"
	goparser "go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"

	"gpp/compiler/ast"
	"gpp/compiler/codegen"
	"gpp/compiler/diag"
	"gpp/compiler/parser"
	"gpp/compiler/semantic"
)

// canonical parses Go source with the standard parser and prints it without
// comments or positions, giving a canonical form for comparison.
func canonical(t *testing.T, name string, src []byte) string {
	t.Helper()
	fset := token.NewFileSet()
	f, err := goparser.ParseFile(fset, name, src, goparser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("go/parser: %v", err)
	}
	f.Comments = nil
	goast.Inspect(f, func(n goast.Node) bool {
		if n == nil {
			return false
		}
		v := reflect.ValueOf(n)
		if v.Kind() != reflect.Pointer || v.IsNil() || v.Elem().Kind() != reflect.Struct {
			return true
		}
		e := v.Elem()
		for i := 0; i < e.NumField(); i++ {
			fl := e.Field(i)
			if fl.Type() == reflect.TypeOf(token.NoPos) && fl.CanSet() {
				if fl.Int() != 0 && (e.Type().Field(i).Name == "Ellipsis" || e.Type().Field(i).Name == "Lparen" || e.Type().Field(i).Name == "Rparen" || e.Type().Field(i).Name == "Assign") {
					fl.SetInt(1)
				} else {
					fl.SetInt(0)
				}
			}
			if e.Type().Field(i).Name == "Doc" || e.Type().Field(i).Name == "Comment" {
				fl.Set(reflect.Zero(fl.Type()))
			}
		}
		return true
	})
	// Import grouping is not preserved (imports are re-sorted by the
	// formatter), so compare imports as a sorted set.
	var buf bytes.Buffer
	var imports []string
	for _, imp := range f.Imports {
		name := ""
		if imp.Name != nil {
			name = imp.Name.Name + " "
		}
		imports = append(imports, name+imp.Path.Value)
	}
	sort.Strings(imports)
	buf.WriteString(strings.Join(imports, "\n") + "\n")
	for _, d := range f.Decls {
		if gd, ok := d.(*goast.GenDecl); ok && gd.Tok == token.IMPORT {
			continue
		}
		if err := format.Node(&buf, token.NewFileSet(), d); err != nil {
			t.Fatal(err)
		}
		buf.WriteString("\n")
	}
	return buf.String()
}

// TestGoStdlibRoundTrip parses real Go files from the standard library as
// G++, lowers them back to Go and checks the result is structurally identical
// to the original.
func TestGoStdlibRoundTrip(t *testing.T) {
	root := runtime.GOROOT()
	var files []string
	if os.Getenv("GPP_FULL_GOROOT") != "" {
		filepath.Walk(filepath.Join(root, "src"), func(p string, info os.FileInfo, err error) error {
			if err == nil && strings.HasSuffix(p, ".go") && !strings.Contains(p, "testdata") {
				files = append(files, p)
			}
			return nil
		})
	}
	for _, pkg := range []string{"strings", "bytes", "sort", "container/list", "encoding/json", "net/url", "text/template/parse", "go/ast", "sync", "time"} {
		m, _ := filepath.Glob(filepath.Join(root, "src", pkg, "*.go"))
		for _, f := range m {
			if !strings.HasSuffix(f, "_test.go") {
				files = append(files, f)
			}
		}
	}
	if len(files) == 0 {
		t.Skip("GOROOT sources not available")
	}
	for _, path := range files {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		t.Run(filepath.Base(filepath.Dir(path))+"/"+filepath.Base(path), func(t *testing.T) {
			f, err := parser.ParseFile(diag.NewFile(path, src))
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			prog, errs := semantic.Analyze([]*ast.File{f})
			if len(errs) > 0 {
				t.Fatalf("analyze: %v", errs)
			}
			out := codegen.Lower(prog)
			gen, err := codegen.Print(out.Files[0])
			if err != nil {
				t.Fatalf("print: %v", err)
			}
			want := canonical(t, path, src)
			got := canonical(t, "generated.go", gen)
			if got != want {
				wl, gl := strings.Split(want, "\n"), strings.Split(got, "\n")
				for i := range wl {
					if i >= len(gl) || wl[i] != gl[i] {
						g := ""
						if i < len(gl) {
							g = gl[i]
						}
						t.Fatalf("mismatch at canonical line %d:\nwant: %s\ngot:  %s", i+1, wl[i], g)
					}
				}
				t.Fatalf("generated output has extra lines")
			}
		})
	}
}
