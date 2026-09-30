package parser

import (
	"strings"
	"testing"

	"github.com/raptordnj/gpp/compiler/ast"
)

func parse(t *testing.T, src string) *ast.File {
	t.Helper()
	f, err := ParseSource("t.gpp", []byte(src))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	return f
}

func TestParseGoProgram(t *testing.T) {
	f := parse(t, `package main

import (
	"fmt"
	str "strings"
)

type Point struct{ X, Y int }

func (p *Point) Move(dx int) { p.X += dx }

func main() {
	numbers := []int{1, 2, 3}
	for _, n := range numbers {
		fmt.Println(n, str.ToUpper("x"))
	}
	if p := (Point{1, 2}); p.X > 0 {
		p.Move(1)
	}
	switch v := any(1).(type) {
	case int:
		_ = v
	}
	ch := make(chan int)
	go func() { ch <- 1 }()
	select {
	case x := <-ch:
		_ = x
	default:
	}
}
`)
	if f.Package.Name.Name != "main" || len(f.Imports) != 2 {
		t.Fatalf("bad package/imports: %+v", f.Imports)
	}
	if len(f.Decls) != 4 {
		t.Fatalf("expected 4 declarations, got %d", len(f.Decls))
	}
}

func TestParseClass(t *testing.T) {
	f := parse(t, `package main

abstract class Animal<T> extends Base implements Speaker, fmt.Stringer {
	private name string = "x"
	public static Count int
	protected a, b int

	constructor(name string) {
		super(name)
		this.name = name
	}

	abstract function Speak() string
	public function Sleep() {}
	override func String() string { return "" }
	public static function Make() *Animal[int] { return nil }

	public property Name string {
		get { return this.name }
		set { this.name = value }
	}
}
`)
	c, ok := f.Decls[0].(*ast.ClassDecl)
	if !ok {
		t.Fatalf("expected class, got %T", f.Decls[0])
	}
	if !c.Abstract || c.Name.Name != "Animal" || c.TypeParams.NumFields() != 1 {
		t.Errorf("bad class header: %+v", c)
	}
	if c.Extends.(*ast.Ident).Name != "Base" || len(c.Implements) != 2 {
		t.Errorf("bad extends/implements")
	}
	if len(c.Fields) != 3 || c.Fields[0].Mods.Access != ast.Private || c.Fields[0].Value == nil {
		t.Errorf("bad fields: %+v", c.Fields)
	}
	if !c.Fields[1].Mods.Static || len(c.Fields[2].Names) != 2 {
		t.Errorf("bad static/grouped field")
	}
	if c.Ctor == nil || len(c.Ctor.Body.List) != 2 {
		t.Fatalf("bad constructor")
	}
	if len(c.Methods) != 4 || c.Methods[0].Body != nil || !c.Methods[0].Mods.Abstract || !c.Methods[2].Mods.Override || !c.Methods[3].Mods.Static {
		t.Errorf("bad methods")
	}
	if len(c.Properties) != 1 || c.Properties[0].Getter == nil || c.Properties[0].Setter == nil {
		t.Errorf("bad property")
	}
}

func TestParseNewVersusGoNew(t *testing.T) {
	f := parse(t, `package main
func main() {
	a := new User("x", 1)
	b := new(User)
	c := new Box<int>()
	d := new pkg.Thing()
	e := new Pair[string, int]("k", 1)
}`)
	body := f.Decls[0].(*ast.FuncDecl).Body.List
	rhs := func(i int) ast.Expr { return body[i].(*ast.AssignStmt).Rhs[0] }
	if n, ok := rhs(0).(*ast.NewExpr); !ok || len(n.Args) != 2 {
		t.Errorf("new User(...) should be a NewExpr, got %T", rhs(0))
	}
	if call, ok := rhs(1).(*ast.CallExpr); !ok || call.Fun.(*ast.Ident).Name != "new" {
		t.Errorf("new(User) should be a Go call, got %T", rhs(1))
	}
	if n, ok := rhs(2).(*ast.NewExpr); !ok || len(n.TypeArgs) != 1 {
		t.Errorf("new Box<int>() should have type args")
	}
	if n, ok := rhs(3).(*ast.NewExpr); !ok {
		t.Errorf("new pkg.Thing() should be a NewExpr")
	} else if _, ok := n.Type.(*ast.SelectorExpr); !ok {
		t.Errorf("qualified class expected")
	}
	if n, ok := rhs(4).(*ast.NewExpr); !ok || len(n.TypeArgs) != 2 {
		t.Errorf("new Pair[string, int] should have 2 type args")
	}
}

func TestParseInterfaceAndEnum(t *testing.T) {
	f := parse(t, `package main
interface Repository {
	function Find(id int) (*User, error)
	Delete(id int) error
	fmt.Stringer
}
enum Status {
	Pending
	Done
}
enum Color { Red, Green, Blue }
`)
	i := f.Decls[0].(*ast.InterfaceDecl)
	if len(i.Methods.List) != 3 {
		t.Errorf("expected 3 interface elements, got %d", len(i.Methods.List))
	}
	if e := f.Decls[1].(*ast.EnumDecl); len(e.Members) != 2 {
		t.Errorf("bad enum")
	}
	if e := f.Decls[2].(*ast.EnumDecl); len(e.Members) != 3 {
		t.Errorf("bad comma enum")
	}
}

func TestFunctionKeyword(t *testing.T) {
	f := parse(t, "package main\nfunction add(a, b int) int { return a + b }\nfunc sub(a int, b int) int { return a - b }\n")
	for _, d := range f.Decls {
		if _, ok := d.(*ast.FuncDecl); !ok {
			t.Errorf("expected function, got %T", d)
		}
	}
}

func TestContextualKeywordsAsIdentifiers(t *testing.T) {
	parse(t, `package main
func main() {
	class := 1
	static, public := 2, 3
	function := func() {}
	_ = class + static + public
	function()
}`)
}

func TestSyntaxErrors(t *testing.T) {
	cases := map[string]string{
		"package main\nfunc main() { x := }":                           "expected expression",
		"package main\nclass A { public function f() }":                "missing body",
		"package main\nclass A { abstract function f() {} }":           "cannot have a body",
		"package main\nclass A { constructor() {}\nconstructor() {} }": "already has a constructor",
		"package main\nclass A { public private x int }":               "multiple access modifiers",
		"func main() {}": "package statement must be first",
		"package main\nfunc main() {\n\tfoo(1,\n\t2\n)}": "missing ','",
	}
	for src, want := range cases {
		_, err := ParseSource("t.gpp", []byte(src))
		if err == nil {
			t.Errorf("%q: expected error containing %q", src, want)
			continue
		}
		if !strings.Contains(err.Error(), want) {
			t.Errorf("%q: got %q, want it to contain %q", src, err, want)
		}
		if !strings.HasPrefix(err.Error(), "t.gpp:") {
			t.Errorf("error should reference the .gpp file: %v", err)
		}
	}
}

func TestAngleGenericsInTypes(t *testing.T) {
	f := parse(t, `package main
func main() {
	var c Container<int>
	var n Box<List<int>> = x
	m := map[string]Box<int>{}
	b := new Box<List<int>>()
	ok := 1 < 2 && 3 >> 1 > 0
}`)
	body := f.Decls[0].(*ast.FuncDecl).Body.List
	if len(body) != 5 {
		t.Fatalf("expected 5 statements (automatic semicolon after '>'), got %d", len(body))
	}
	spec := body[1].(*ast.DeclStmt).Decl.Specs[0].(*ast.ValueSpec)
	outer, ok := spec.Type.(*ast.IndexExpr)
	if !ok {
		t.Fatalf("expected generic type, got %T", spec.Type)
	}
	if _, ok := outer.Indices[0].(*ast.IndexExpr); !ok {
		t.Fatalf("expected nested generic type argument, got %T", outer.Indices[0])
	}
}

func TestGoGenericsEdgeCases(t *testing.T) {
	parse(t, `package main
type Set[T *int | string, V any] struct{}
type Arr [N * 2]int
type P[T interface{ ~int }] []T
func appendString[Bytes []byte | string](dst []byte, src Bytes) []byte { return dst }
func (r *R) N[Int intType](n Int) Int { return n }
`)
}
