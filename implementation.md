# G++ Language — Compiler Implementation Specification

You are an expert compiler engineer and Go developer.

Your task is to **design and implement a new programming language named `G++`**.

> Important naming note: `g++` is already the GNU C++ compiler command. Therefore, the language is called **G++**, source files use `.gpp`, but the CLI compiler executable MUST be named `gpp` to avoid conflict with GNU `g++`.

The compiler itself MUST be implemented in **Go**.

---

# 1. Language Goal

G++ is a **superset of Go**.

The language should preserve Go's:

* simplicity
* performance
* concurrency model
* package system
* type system
* interfaces
* structs
* slices
* maps
* channels
* goroutines
* generics
* error handling
* standard library compatibility

while adding **Java/C#-style object-oriented programming features**.

Conceptually:

```text
G++ = Go + OOP extensions
```

The primary compilation strategy is:

```text
.gpp source
    ↓
Lexer
    ↓
Parser
    ↓
AST
    ↓
Semantic Analysis
    ↓
Type Checking
    ↓
G++ OOP Lowering
    ↓
Go Code Generation
    ↓
.go source
    ↓
Go compiler/toolchain
    ↓
Native executable
```

Do NOT initially implement a native machine-code backend.

Use the existing Go compiler as the final backend.

---

# 2. Core Principle

Existing valid Go code should remain valid G++ code whenever possible.

For example, this should work:

```go
package main

import "fmt"

func main() {
    numbers := []int{1, 2, 3}

    for _, n := range numbers {
        fmt.Println(n)
    }
}
```

The file extension changes to:

```text
main.gpp
```

The G++ compiler should generate equivalent Go code.

Do not unnecessarily redesign Go syntax.

---

# 3. G++ OOP Syntax

## Classes

Support:

```gpp
class User {
    public ID int
    public Name string
}
```

Generate approximately:

```go
type User struct {
    ID   int
    Name string
}
```

---

# 4. Constructors

G++:

```gpp
class User {
    private name string
    private age int

    constructor(name string, age int) {
        this.name = name
        this.age = age
    }
}
```

Generate:

```go
type User struct {
    name string
    age  int
}

func NewUser(name string, age int) *User {
    return &User{
        name: name,
        age:  age,
    }
}
```

Usage:

```gpp
user := new User("Tohid", 35)
```

Generate:

```go
user := NewUser("Tohid", 35)
```

---

# 5. Methods

G++:

```gpp
class User {
    private name string

    public function SayHello() {
        fmt.Println("Hello", this.name)
    }
}
```

Generate:

```go
type User struct {
    name string
}

func (this *User) SayHello() {
    fmt.Println("Hello", this.name)
}
```

---

# 6. `this`

Support:

```gpp
this.name
this.GetName()
```

Inside instance methods, `this` refers to the current object.

Generate a Go pointer receiver unless semantic analysis determines that a value receiver is appropriate.

For the initial implementation, consistently use pointer receivers.

---

# 7. `new`

Support:

```gpp
user := new User("Tohid", 35)
```

This is G++ constructor syntax and MUST NOT conflict with Go's built-in `new(T)`.

Translate:

```gpp
new User(...)
```

into:

```go
NewUser(...)
```

For normal Go:

```gpp
user := new(User)
```

preserve normal Go semantics.

The parser must distinguish:

```text
new Type(...)
```

from:

```text
new(Type)
```

---

# 8. Access Modifiers

Support:

```text
public
private
protected
internal
```

Example:

```gpp
class User {
    private password string
    protected email string
    public name string
}
```

The compiler must enforce access rules during semantic analysis.

Do NOT rely solely on Go visibility rules.

Because Go has package-level visibility rather than Java/C#-style member visibility, the G++ compiler must validate access before code generation.

---

# 9. Inheritance

Support:

```gpp
class Animal {
    public function Speak() {
        fmt.Println("Animal")
    }
}

class Dog extends Animal {
    override function Speak() {
        fmt.Println("Woof")
    }
}
```

Use Go embedding internally:

```go
type Dog struct {
    Animal
}
```

Do not create a complicated runtime inheritance system unless absolutely necessary.

---

# 10. `super`

Support constructor calls:

```gpp
class Dog extends Animal {
    constructor(name string) {
        super(name)
    }
}
```

And parent method calls:

```gpp
super.Speak()
```

Translate these appropriately to Go embedding.

---

# 11. `override`

Support:

```gpp
class Dog extends Animal {
    override function Speak() {
        fmt.Println("Woof")
    }
}
```

The semantic analyzer MUST verify:

1. A parent method exists.
2. The method signature matches.
3. The method is overridable.
4. `override` is not used without a corresponding parent method.

Compiler errors should be clear.

Example:

```text
error: method 'Speak' marked override but no overridable method exists in parent class 'Animal'
```

---

# 12. Interfaces

Support:

```gpp
interface Repository {
    function Find(id int) (*User, error)
    function Delete(id int) error
}
```

Generate:

```go
type Repository interface {
    Find(id int) (*User, error)
    Delete(id int) error
}
```

Support:

```gpp
class UserRepository implements Repository {
    ...
}
```

The compiler should verify that all interface methods are implemented.

Because Go interfaces are structural, `implements` is primarily a compile-time verification feature.

---

# 13. Abstract Classes

Support:

```gpp
abstract class Animal {
    abstract function Speak()

    public function Sleep() {
        fmt.Println("Sleeping")
    }
}
```

A subclass:

```gpp
class Dog extends Animal {
    override function Speak() {
        fmt.Println("Woof")
    }
}
```

The compiler must reject:

```gpp
class Dog extends Animal {
}
```

because `Speak()` remains unimplemented.

Abstract classes cannot be instantiated.

---

# 14. Static Members

Support:

```gpp
class Math {
    public static PI float64 = 3.1415926535

    public static function Add(a int, b int) int {
        return a + b
    }
}
```

Usage:

```gpp
Math.Add(10, 20)
Math.PI
```

Implement static members using Go package-level declarations or another simple deterministic lowering strategy.

Avoid generating unnecessary runtime objects.

---

# 15. Properties

Eventually support:

```gpp
class User {
    private _name string

    public property Name string {
        get {
            return this._name
        }

        set {
            this._name = value
        }
    }
}
```

Initially lower this to getter/setter methods.

Properties are NOT required for v0.1 unless implementation is straightforward.

---

# 16. Enums

Support:

```gpp
enum Status {
    Pending
    Processing
    Completed
    Cancelled
}
```

Generate approximately:

```go
type Status int

const (
    StatusPending Status = iota
    StatusProcessing
    StatusCompleted
    StatusCancelled
)
```

---

# 17. Generics

G++ MUST preserve Go generics.

Example:

```gpp
class Repository<T> {
    private items []T

    public function Add(item T) {
        this.items = append(this.items, item)
    }

    public function All() []T {
        return this.items
    }
}
```

Generate equivalent Go generics.

Do not invent a separate generic runtime.

Use Go's native generic type system.

---

# 18. Error Handling

Do NOT initially replace Go's error handling.

This:

```gpp
user, err := repository.Find(id)

if err != nil {
    return err
}
```

should remain normal Go-compatible syntax.

Do NOT introduce Java/C# exceptions in v0.1.

Exception support can be considered in a future version.

---

# 19. Goroutines and Channels

These MUST remain valid:

```gpp
go worker()

channel <- value

value := <-channel
```

Also preserve:

```gpp
select {
case value := <-channel:
    fmt.Println(value)

case <-time.After(time.Second):
    fmt.Println("timeout")
}
```

Do not wrap or replace Go concurrency primitives.

---

# 20. Package System

Initially use normal Go packages.

Example:

```gpp
package users

import (
    "database/sql"
    "fmt"
)
```

Generated Go should retain imports correctly.

The compiler must not attempt to recreate Go's package ecosystem.

---

# 21. Compiler CLI

The CLI executable should be:

```bash
gpp
```

Commands:

```bash
gpp build main.gpp
gpp run main.gpp
gpp check main.gpp
gpp generate main.gpp
gpp fmt main.gpp
gpp version
```

Examples:

```bash
gpp build main.gpp
```

should produce:

```text
main
```

or:

```bash
gpp build main.gpp -o app
```

should produce:

```text
app
```

---

# 22. `gpp generate`

This command should generate Go source without compiling.

Example:

```bash
gpp generate main.gpp
```

Output:

```text
.walu/
```

DO NOT call the directory `.walu`.

Use:

```text
.gpp/
```

For example:

```text
.gpp/generated/main.go
```

The user should be able to inspect generated Go code.

---

# 23. `gpp check`

This should perform:

```text
lexing
parsing
semantic analysis
type checking
```

without invoking `go build`.

Example:

```bash
gpp check main.gpp
```

---

# 24. `gpp run`

Implement:

```bash
gpp run main.gpp
```

by:

1. compiling `.gpp`
2. generating temporary Go
3. invoking the Go compiler
4. running the resulting executable
5. cleaning temporary artifacts unless `--keep` is specified

---

# 25. Project Structure

Create:

```text
gpp/
├── cmd/
│   └── gpp/
│       └── main.go
│
├── compiler/
│   ├── lexer/
│   │   ├── lexer.go
│   │   ├── token.go
│   │   └── lexer_test.go
│   │
│   ├── parser/
│   │   ├── parser.go
│   │   ├── expressions.go
│   │   ├── statements.go
│   │   ├── declarations.go
│   │   └── parser_test.go
│   │
│   ├── ast/
│   │   ├── ast.go
│   │   ├── declarations.go
│   │   ├── expressions.go
│   │   └── statements.go
│   │
│   ├── semantic/
│   │   ├── analyzer.go
│   │   ├── symbols.go
│   │   ├── types.go
│   │   └── semantic_test.go
│   │
│   ├── codegen/
│   │   ├── generator.go
│   │   ├── classes.go
│   │   ├── functions.go
│   │   ├── interfaces.go
│   │   └── codegen_test.go
│   │
│   └── compiler.go
│
├── runtime/
│   └── runtime.go
│
├── examples/
│   ├── hello.gpp
│   ├── class.gpp
│   ├── inheritance.gpp
│   ├── interface.gpp
│   ├── generic.gpp
│   └── goroutine.gpp
│
├── tests/
│   └── integration/
│
├── docs/
│   └── LANGUAGE.md
│
├── go.mod
├── README.md
└── LICENSE
```

Adjust the structure when necessary, but keep compiler responsibilities separated.

---

# 26. Lexer

Implement a proper lexer.

At minimum support:

```text
identifiers
keywords
integers
floats
strings
runes
operators
delimiters
comments
newlines where relevant
```

G++ keywords initially include:

```text
package
import
func
function
var
const
type
struct
interface
class
abstract
extends
implements
constructor
override
public
private
protected
internal
static
property
enum
this
super
new
return
if
else
for
range
switch
case
default
go
select
defer
map
chan
```

Do not make `function` mandatory if Go's `func` syntax is retained.

Ideally support both:

```gpp
func test() {}
```

and:

```gpp
function test() {}
```

with both producing Go `func`.

---

# 27. Parser

Use a maintainable parser architecture.

A recursive-descent parser or Pratt parser is acceptable.

Do not use regular expressions to parse the language.

The parser must produce a typed AST.

AST nodes should represent concepts such as:

```text
Program
PackageDeclaration
ImportDeclaration
FunctionDeclaration
ClassDeclaration
InterfaceDeclaration
EnumDeclaration
FieldDeclaration
MethodDeclaration
ConstructorDeclaration
VariableDeclaration
BlockStatement
IfStatement
ForStatement
ReturnStatement
CallExpression
MemberExpression
BinaryExpression
UnaryExpression
NewExpression
```

---

# 28. Semantic Analyzer

Implement symbol tables and scopes.

The semantic analyzer must understand:

```text
variables
functions
classes
fields
methods
constructors
interfaces
inheritance
generic parameters
access modifiers
method overriding
interface implementation
abstract methods
```

It must produce useful compiler errors.

Examples:

```text
unknown identifier 'foo'

unknown class 'User'

constructor for class 'User' expects 2 arguments, got 1

cannot access private field 'password'

class 'Dog' does not implement method 'Speak'

method 'Foo' marked override but no parent method exists

cannot instantiate abstract class 'Animal'
```

---

# 29. Type System

Initially reuse Go's type semantics wherever possible.

Support:

```text
bool
string
int
int8
int16
int32
int64
uint
uint8
uint16
uint32
uint64
float32
float64
complex64
complex128
byte
rune
arrays
slices
maps
channels
pointers
structs
interfaces
functions
generics
```

Do not create a second incompatible type system unnecessarily.

---

# 30. Code Generation

Generate valid, gofmt-compatible Go source.

Prefer the Go standard library's AST packages where practical:

```go
go/ast
go/parser
go/token
go/format
```

Do not construct Go source using uncontrolled string concatenation when AST generation can make the implementation safer.

Generated source MUST be passed through:

```bash
gofmt
```

or Go's formatting APIs.

---

# 31. Source Maps / Errors

Compiler errors should eventually reference the original `.gpp` file:

```text
main.gpp:14:9: cannot access private field 'password'
```

Do not expose generated `.go` line numbers as the primary error location.

Maintain source position information in AST nodes.

---

# 32. Testing Strategy

Every compiler feature must have tests.

Use at least:

### Lexer tests

```text
lexer_test.go
```

### Parser tests

```text
parser_test.go
```

### Semantic tests

```text
semantic_test.go
```

### Code generation tests

```text
codegen_test.go
```

### Integration tests

Compile actual `.gpp` files and verify their output.

Example:

```text
tests/integration/basic_class.gpp
tests/integration/inheritance.gpp
tests/integration/interface.gpp
```

Run:

```bash
go test ./...
```

The project must remain green after every change.

---

# 33. Bootstrap Example

The first milestone must successfully compile:

```gpp
package main

import "fmt"

class Person {
    private name string
    private age int

    constructor(name string, age int) {
        this.name = name
        this.age = age
    }

    public function SayHello() {
        fmt.Println("Hello, I am", this.name)
    }
}

function main() {
    person := new Person("Tohid", 35)
    person.SayHello()
}
```

Expected:

```bash
gpp build main.gpp
./main
```

Output:

```text
Hello, I am Tohid
```

---

# 34. Development Rules

You are an implementation agent, not a documentation-only agent.

When modifying the repository:

1. Inspect the existing project first.
2. Do not overwrite working code unnecessarily.
3. Implement one coherent feature at a time.
4. Add tests with every feature.
5. Run `go test ./...`.
6. Run `gofmt`.
7. Build the CLI.
8. Test actual `.gpp` programs.
9. Fix compilation errors before moving on.
10. Keep the implementation simple and idiomatic Go.

Never claim a feature is implemented unless it actually works.

---

# 35. Do Not Overengineer

For v0.1:

DO:

```text
Go-compatible syntax
classes
constructors
methods
this
new Class(...)
private/public/protected
extends
super
override
interfaces
implements
Go code generation
CLI
tests
```

DO NOT initially implement:

```text
custom VM
custom bytecode
LLVM backend
custom garbage collector
Java-style exception runtime
reflection runtime
JIT
custom standard library
custom package manager
native machine-code generator
```

Use Go's existing infrastructure wherever possible.

---

# 36. Versioning

Start with:

```text
G++ 0.1.0
```

Language specification:

```text
G++ Language Specification v0.1
```

Compiler version:

```bash
gpp version
```

should report something like:

```text
G++ compiler 0.1.0
backend: Go
source: .gpp
```

---

# 37. Documentation

Create:

```text
README.md
docs/LANGUAGE.md
```

README should explain:

* what G++ is
* why it exists
* installation
* first program
* compiler architecture
* `.gpp` extension
* Go compatibility
* OOP features
* examples
* current limitations
* roadmap

Do not claim full Go compatibility until it has actually been tested.

---

# 38. Implementation Order

Follow this order strictly unless the repository requires a different dependency order:

### Milestone 1

```text
Go project
CLI
lexer
tokens
parser
AST
basic Go-compatible program
```

### Milestone 2

```text
function
variables
expressions
statements
imports
Go code generation
```

### Milestone 3

```text
class
fields
methods
this
constructor
new Class(...)
```

### Milestone 4

```text
access modifiers
semantic analysis
symbol tables
```

### Milestone 5

```text
extends
super
override
inheritance validation
```

### Milestone 6

```text
interface
implements
abstract class
```

### Milestone 7

```text
generics
static members
enum
properties
```

### Milestone 8

```text
gpp build
gpp run
gpp check
gpp generate
gpp fmt
integration tests
```

---

# 39. Important Compatibility Rule

When G++ syntax conflicts with Go syntax, prefer a syntax that allows both to coexist.

For example:

```gpp
new User("A")
```

means G++ constructor syntax.

But:

```gpp
new(User)
```

means normal Go allocation.

Do not break valid Go unnecessarily.

---

# 40. Agent Behavior

At the beginning:

```text
1. Inspect repository.
2. Determine whether this is a new or existing project.
3. Inspect go.mod.
4. Inspect existing source files.
5. Create a concise implementation plan.
```

Then implement the first missing milestone.

After implementation:

```text
go fmt ./...
go test ./...
go vet ./...
go build ./cmd/gpp
```

If integration tests exist, execute them.

At the end of each task, report:

```text
Implemented:
- ...

Files changed:
- ...

Tests:
- ...

Build:
- ...

Remaining:
- ...
```

Do not stop after creating a design.

**Write the actual code.**

---

# 41. Definition of Done for v0.1

G++ v0.1 is considered successful when this works:

```bash
gpp build hello.gpp
```

with:

```gpp
package main

import "fmt"

class Person {
    private name string

    constructor(name string) {
        this.name = name
    }

    public function SayHello() {
        fmt.Println("Hello,", this.name)
    }
}

class Developer extends Person {
    constructor(name string) {
        super(name)
    }

    override function SayHello() {
        fmt.Println("Hello, developer!")
    }
}

function main() {
    developer := new Developer("Tohid")
    developer.SayHello()
}
```

and produces a native executable through the Go toolchain.

The generated Go source must be valid, formatted Go.

---

# Final Objective

Build a real, maintainable compiler for:

```text
G++ Language
Source: .gpp
Compiler: Go
Backend: Go compiler
Paradigm: Go + OOP
```

The long-term vision is:

```text
                 G++
                  │
       ┌──────────┴──────────┐
       │                     │
      Go                 OOP Extensions
       │                     │
 goroutines              classes
 channels                inheritance
 interfaces              constructors
 generics                properties
 packages                access control
 structs                 abstract classes
 errors                  override
 defer                   static
       │                     │
       └──────────┬──────────┘
                  │
             Go Backend
                  │
             Native Binary
```

Prioritize **correctness, Go interoperability, clean compiler architecture, useful diagnostics, and incremental working software** over implementing a large number of language features quickly.
