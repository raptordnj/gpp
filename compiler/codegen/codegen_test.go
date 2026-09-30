package codegen_test

import (
	"go/format"
	"strings"
	"testing"

	"github.com/raptordnj/gpp/compiler"
)

func generate(t *testing.T, src string) string {
	t.Helper()
	res, err := compiler.CompileSource("main.gpp", []byte(src))
	if err != nil {
		t.Fatalf("compile error:\n%v", err)
	}
	code := res.Files[0].Code
	formatted, err := format.Source(code)
	if err != nil {
		t.Fatalf("generated code is not valid Go: %v\n%s", err, code)
	}
	if string(formatted) != string(code) {
		t.Fatalf("generated code is not gofmt-formatted:\n%s", code)
	}
	return string(code)
}

func contains(t *testing.T, code string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(code, w) {
			t.Errorf("generated code does not contain %q:\n%s", w, code)
		}
	}
}

func TestPlainGoIsPreserved(t *testing.T) {
	code := generate(t, `package main

import "fmt"

func main() {
	numbers := []int{1, 2, 3}
	for _, n := range numbers {
		fmt.Println(n)
	}
}
`)
	contains(t, code, "package main", `import "fmt"`, "numbers := []int{1, 2, 3}", "for _, n := range numbers {")
}

func TestClassLowering(t *testing.T) {
	code := generate(t, `package main
import "fmt"
class User {
	private name string
	private age int = 18
	constructor(name string) {
		this.name = name
	}
	public function SayHello() {
		fmt.Println("Hello", this.name, this.age)
	}
}
function main() {
	user := new User("Tohid")
	user.SayHello()
	other := new(User)
	_ = other
}`)
	contains(t, code,
		"type User struct {\n\tname string\n\tage  int\n}",
		"func NewUser(name string) *User {\n\tthis := &User{}\n\tthis.initUser(name)\n\treturn this\n}",
		"func (this *User) initUser(name string) {\n\tthis.age = 18\n\tthis.name = name\n}",
		"func (this *User) SayHello() {",
		`user := NewUser("Tohid")`,
		"other := new(User)",
	)
}

func TestClassWithoutInit(t *testing.T) {
	code := generate(t, "package main\nclass Empty { public X int }\nfunc main() { _ = new Empty() }")
	contains(t, code, "func NewEmpty() *Empty {\n\treturn &Empty{}\n}", "_ = NewEmpty()")
	if strings.Contains(code, "initEmpty") {
		t.Errorf("unexpected init method:\n%s", code)
	}
}

func TestInheritanceLowering(t *testing.T) {
	code := generate(t, `package main
import "fmt"
class Person {
	private name string
	constructor(name string) { this.name = name }
	public function SayHello() { fmt.Println("Hello,", this.name) }
}
class Developer extends Person {
	constructor(name string) { super(name) }
	override function SayHello() {
		super.SayHello()
		fmt.Println("developer!")
	}
}
function main() { new Developer("x").SayHello() }`)
	contains(t, code,
		"type Developer struct {\n\tPerson\n}",
		"func (this *Developer) initDeveloper(name string) {\n\tthis.Person.initPerson(name)\n}",
		"this.Person.SayHello()",
	)
}

func TestAbstractClassLowering(t *testing.T) {
	code := generate(t, `package main
abstract class Animal { abstract function Speak() string }
class Dog extends Animal { override function Speak() string { return "Woof" } }
func main() { _ = new Dog().Speak() }`)
	contains(t, code, `panic("abstract method Animal.Speak called")`, "func NewDog() *Dog")
	if strings.Contains(code, "func NewAnimal") {
		t.Errorf("abstract classes must not get a constructor function")
	}
}

func TestInterfaceLowering(t *testing.T) {
	code := generate(t, `package main
class User {}
interface Repository {
	function Find(id int) (*User, error)
	function Delete(id int) error
}
class Repo implements Repository {
	public function Find(id int) (*User, error) { return nil, nil }
	public function Delete(id int) error { return nil }
}
func main() {}`)
	contains(t, code,
		"type Repository interface {\n\tFind(id int) (*User, error)\n\tDelete(id int) error\n}",
		"var _ Repository = (*Repo)(nil)",
	)
}

func TestStaticAndEnumLowering(t *testing.T) {
	code := generate(t, `package main
class Math {
	public static PI float64 = 3.14
	public static function Add(a int, b int) int { return a + b }
}
enum Status { Pending
	Done }
func main() {
	_ = Math.Add(1, 2)
	_ = Math.PI
	s := Status.Done
	_ = s
}`)
	contains(t, code,
		"var Math_PI float64 = 3.14",
		"func Math_Add(a int, b int) int {",
		"_ = Math_Add(1, 2)",
		"_ = Math_PI",
		"type Status int",
		"StatusPending Status = iota",
		"s := StatusDone",
		"func (e Status) String() string {",
	)
}

func TestGenericClassLowering(t *testing.T) {
	code := generate(t, `package main
class Repository<T> {
	private items []T
	public function Add(item T) { this.items = append(this.items, item) }
	public function All() []T { return this.items }
}
class Pair<K comparable, V any> { public Key K
	public Value V }
func main() {
	r := new Repository<int>()
	r.Add(1)
	_ = new Pair[string, int]()
}`)
	contains(t, code,
		"type Repository[T any] struct {",
		"func NewRepository[T any]() *Repository[T] {",
		"func (this *Repository[T]) Add(item T) {",
		"r := NewRepository[int]()",
		"type Pair[K comparable, V any] struct {",
		"_ = NewPair[string, int]()",
	)
}

func TestPropertyLowering(t *testing.T) {
	code := generate(t, `package main
class User {
	private _name string
	public property Name string {
		get { return this._name }
		set { this._name = value }
	}
}
func main() {
	u := new User()
	u.Name = "a"
	u.Name += "b"
	_ = u.Name
}`)
	contains(t, code,
		"func (this *User) GetName() string {",
		"func (this *User) SetName(value string) {",
		`u.SetName("a")`,
		`u.SetName(u.GetName() + "b")`,
		"_ = u.GetName()",
		"type User struct {\n\t_name string\n}",
	)
}
