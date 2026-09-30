package semantic_test

import (
	"strings"
	"testing"

	"gpp/compiler"
)

// check compiles src and returns the diagnostics (empty when it compiles).
func check(t *testing.T, src string) string {
	t.Helper()
	_, err := compiler.CompileSource("main.gpp", []byte(src))
	if err == nil {
		return ""
	}
	return err.Error()
}

func TestValidProgramHasNoErrors(t *testing.T) {
	src := `package main
import "fmt"
abstract class Shape {
	protected name string
	constructor(name string) { this.name = name }
	abstract function Area() float64
	public function Name() string { return this.name }
}
class Square extends Shape {
	private side float64
	constructor(side float64) {
		super("square")
		this.side = side
	}
	override function Area() float64 { return this.side * this.side }
}
func main() { fmt.Println(new Square(2).Area(), new Square(1).Name()) }
`
	if errs := check(t, src); errs != "" {
		t.Fatalf("unexpected errors:\n%s", errs)
	}
}

func TestDiagnostics(t *testing.T) {
	cases := []struct {
		name, src, want string
	}{
		{"private field", `package main
class User { private password string }
func main() { u := new User(); _ = u.password }`, "main.gpp:3:38: cannot access private field 'password' of class 'User'"},

		{"private method", `package main
class User { private function secret() {} }
func main() { new User().secret() }`, "cannot access private method 'secret' of class 'User'"},

		{"private in subclass", `package main
class A { private x int }
class B extends A { public function F() int { return this.x } }
func main() {}`, "cannot access private field 'x' of class 'A'"},

		{"protected outside", `package main
class A { protected x int }
func main() { _ = new A().x }`, "cannot access protected field 'x' of class 'A'"},

		{"unknown class", `package main
func main() { _ = new User() }`, "unknown class 'User'"},

		{"unknown identifier", `package main
func main() { _ = foo }`, "unknown identifier 'foo'"},

		{"ctor arg count", `package main
class User { constructor(name string, age int) {} }
func main() { _ = new User("a") }`, "constructor for class 'User' expects 2 arguments, got 1"},

		{"abstract instantiation", `package main
abstract class Animal {}
func main() { _ = new Animal() }`, "cannot instantiate abstract class 'Animal'"},

		{"abstract not implemented", `package main
abstract class Animal { abstract function Speak() }
class Dog extends Animal {}
func main() {}`, "class 'Dog' does not implement abstract method 'Speak' from class 'Animal'"},

		{"abstract in concrete class", `package main
class Animal { abstract function Speak() }
func main() {}`, "must be declared abstract"},

		{"override missing", `package main
class Animal {}
class Dog extends Animal { override function Speak() {} }
func main() {}`, "method 'Speak' marked override but no overridable method exists in parent class 'Animal'"},

		{"override without parent", `package main
class Dog { override function Speak() {} }
func main() {}`, "has no parent class"},

		{"override private", `package main
class Animal { private function Speak() {} }
class Dog extends Animal { override function Speak() {} }
func main() {}`, "no overridable method exists"},

		{"override signature", `package main
class Animal { public function Speak() {} }
class Dog extends Animal { override function Speak() string { return "" } }
func main() {}`, "method 'Speak' overrides 'Animal.Speak' with a different signature: have () string, want ()"},

		{"hiding without override", `package main
class Animal { public function Speak() {} }
class Dog extends Animal { public function Speak() {} }
func main() {}`, "mark it 'override'"},

		{"implements missing", `package main
interface Repo { function Find(id int) error
	function Delete(id int) error }
class R implements Repo { public function Find(id int) error { return nil } }
func main() {}`, "class 'R' does not implement interface 'Repo': missing method 'Delete'"},

		{"implements wrong signature", `package main
interface Repo { function Find(id int) error }
class R implements Repo { public function Find(id string) error { return nil } }
func main() {}`, "method 'Find' has the wrong signature"},

		{"implements unknown", `package main
class R implements Nope {}
func main() {}`, "unknown interface 'Nope'"},

		{"extends unknown", `package main
class R extends Nope {}
func main() {}`, "unknown class 'Nope'"},

		{"inheritance cycle", `package main
class A extends B {}
class B extends A {}
func main() {}`, "inheritance cycle"},

		{"super outside ctor", `package main
class A {}
class B extends A { public function F() { super() } }
func main() {}`, "super(...) can only be called as the first statement of a constructor"},

		{"super missing", `package main
class A { constructor(x int) {} }
class B extends A { constructor() {} }
func main() {}`, "must call super(...)"},

		{"super arg count", `package main
class A { constructor(x int) {} }
class B extends A { constructor() { super() } }
func main() {}`, "super constructor of class 'A' expects 1 argument, got 0"},

		{"super member unknown", `package main
class A {}
class B extends A { public function F() { super.Nope() } }
func main() {}`, "parent class 'A' has no member 'Nope'"},

		{"this outside class", `package main
func main() { this.x = 1 }`, "'this' can only be used inside instance methods"},

		{"this in static", `package main
class M { public x int
	public static function F() int { return this.x } }
func main() {}`, "'this' can only be used inside instance methods"},

		{"static private", `package main
class M { private static secret int }
func main() { _ = M.secret }`, "cannot access private static field 'secret' of class 'M'"},

		{"static unknown", `package main
class M {}
func main() { _ = M.nope }`, "class 'M' has no static member 'nope'"},

		{"instance via class", `package main
class M { public function F() {} }
func main() { M.F() }`, "without an object"},

		{"enum member", `package main
enum Color { Red }
func main() { _ = Color.Blue }`, "enum 'Color' has no member 'Blue'"},

		{"private constructor", `package main
class S { private constructor() {} }
func main() { _ = new S() }`, "cannot access private constructor of class 'S'"},

		{"read-only property", `package main
class P { public property X int { get { return 1 } } }
func main() { p := new P(); p.X = 2 }`, "property 'X' of class 'P' is read-only"},

		{"type error position", `package main
func main() {
	var x int = "hello"
	_ = x
}`, "main.gpp:3:14:"},

		{"abstract call through this", `package main
abstract class A { abstract function F()
	public function G() { this.F() } }
func main() {}`, "cannot call abstract method 'F' through 'this'"},

		{"generic static", `package main
class Box<T> { public static N int }
func main() {}`, "generic class 'Box' cannot declare static field"},

		{"duplicate member", `package main
class A { public x int
	public function x() {} }
func main() {}`, "duplicate member 'x'"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := check(t, c.src)
			if got == "" {
				t.Fatalf("expected error containing %q, got none", c.want)
			}
			if !strings.Contains(got, c.want) {
				t.Fatalf("got:\n%s\nwant substring:\n%s", got, c.want)
			}
		})
	}
}

func TestAccessRules(t *testing.T) {
	// protected access from a subclass and private access inside the class are fine
	src := `package main
class A {
	private p int
	protected q int
	internal r int
	public function SetP(v int) { this.p = v }
}
class B extends A {
	public function Q() int { return this.q + this.r }
}
func main() {
	b := new B()
	b.SetP(1)
	_ = b.Q() + b.r
}`
	if errs := check(t, src); errs != "" {
		t.Fatalf("unexpected errors:\n%s", errs)
	}
}
