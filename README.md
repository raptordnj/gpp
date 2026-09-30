# G++

**G++ is Go with Java/C#-style object-oriented programming.** It is a superset
of Go: plain Go code is valid G++, and G++ adds classes, constructors,
inheritance, `super`, `override`, access modifiers, abstract classes,
interfaces with `implements`, static members, enums and properties.

```text
G++ = Go + OOP extensions
```

The G++ compiler (`gpp`) is written in Go and compiles `.gpp` files to
ordinary, gofmt-formatted Go source, which the standard Go toolchain then
turns into a native executable. Generated programs need no runtime library.

> **Naming:** `g++` is the GNU C++ compiler, so the G++ compiler command is
> **`gpp`** and source files use the **`.gpp`** extension.

## Why?

Go's simplicity, performance, concurrency and tooling are great, but some
people and codebases think in classes: explicit inheritance, constructors,
member visibility, abstract base classes, compile-time `implements` checks.
G++ adds exactly those constructs *on top of* Go instead of replacing it, and
lowers them to idiomatic Go (structs, embedding, pointer-receiver methods,
package-level functions), so you keep goroutines, channels, generics, error
values, the standard library and the Go backend.

## Installation

Requires Go 1.22 or newer (the Go toolchain is also the backend).

```bash
go install github.com/raptordnj/gpp/cmd/gpp@latest   # installs gpp into $(go env GOPATH)/bin
gpp version
```

Or from a clone:

```bash
git clone https://github.com/raptordnj/gpp.git && cd gpp
go install ./cmd/gpp
```

```text
G++ compiler 0.1.0
language: G++ Language Specification v0.1
backend: Go (go1.xx)
source: .gpp
```

## First program

`hello.gpp`:

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

```bash
gpp build hello.gpp     # produces ./hello
./hello                 # Hello, developer!
```

The generated Go (`gpp generate hello.gpp` → `.gpp/generated/hello.go`):

```go
type Person struct {
	name string
}

func NewPerson(name string) *Person {
	this := &Person{}
	this.initPerson(name)
	return this
}

func (this *Person) initPerson(name string) {
	this.name = name
}

func (this *Person) SayHello() {
	fmt.Println("Hello,", this.name)
}

type Developer struct {
	Person
}

func NewDeveloper(name string) *Developer {
	this := &Developer{}
	this.initDeveloper(name)
	return this
}

func (this *Developer) initDeveloper(name string) {
	this.Person.initPerson(name)
}

func (this *Developer) SayHello() {
	fmt.Println("Hello, developer!")
}

func main() {
	developer := NewDeveloper("Tohid")
	developer.SayHello()
}
```

## Commands

| Command | Description |
|---|---|
| `gpp build <file.gpp…\|dir> [-o out] [--keep]` | Compile to a native executable (default name: the source file name). |
| `gpp run <file.gpp…\|dir> [--keep] [-- args…]` | Compile, run, and clean up temporary artifacts (unless `--keep`). |
| `gpp check <file.gpp…\|dir>` | Lex, parse, run semantic analysis and type checking; no `go build`. |
| `gpp generate <file.gpp…\|dir> [-o dir]` | Write the Go source to `.gpp/generated/` for inspection. |
| `gpp fmt <file.gpp…> [--stdout]` | Normalize indentation and whitespace in place. |
| `gpp version` | Print version information. |

All `.gpp` files passed together (or found in a directory) form one package.
The Go toolchain runs in a work directory under `<source dir>/.gpp/`, so an
enclosing Go module (and its dependencies) is picked up automatically.

## Compiler architecture

```text
.gpp source
  → lexer        compiler/lexer      Go tokens + automatic semicolons; G++ words are contextual keywords
  → parser       compiler/parser     recursive descent, typed AST (compiler/ast)
  → semantic     compiler/semantic   symbol tables, inheritance/override/abstract/constructor/static rules
  → lowering     compiler/codegen    G++ AST → go/ast (classes → structs, embedding, NewX, methods)
  → type check   compiler/semantic   go/types on the lowered AST + access, override-signature and implements checks
  → printing     compiler/codegen    go/format → gofmt-formatted .go source
  → go build                         the standard Go toolchain produces the native binary
```

Generated Go nodes carry positions inside the original `.gpp` files, so every
diagnostic — including ordinary Go type errors — points at the `.gpp` source:

```text
main.gpp:8:7: cannot access private field 'password' of class 'User'
main.gpp:5:20: constructor for class 'User' expects 2 arguments, got 1
main.gpp:5:7: class 'Dog' does not implement abstract method 'Speak' from class 'Animal'
main.gpp:3:14: cannot use "hello" (untyped string constant) as int value in variable declaration
```

## Go compatibility

G++ keeps Go's syntax and semantics: packages and imports, structs, interfaces,
slices, maps, channels, goroutines, `select`, `defer`, generics (including
constraints and unions), closures, labels, type switches and error values.
The G++ words (`class`, `this`, `super`, `new`, `function`, `public`, …) are
*contextual* keywords, so Go code that uses them as identifiers still parses.
`new(T)` keeps its Go meaning; only `new Type(args)` is a G++ constructor call.

This is tested, not assumed: the test suite parses real Go files from the
standard library as G++, lowers them back to Go and checks that the result is
structurally identical to the original. Running it over the whole
`$GOROOT/src` tree (`GPP_FULL_GOROOT=1 go test ./tests/integration/`) passes
for every file (about 6,900 files with Go 1.27). Comments are not preserved in
generated code.

## OOP features

- **Classes** with fields (with initializers), methods, one constructor — `class`, `constructor`, `this`
- **`new Class(args)`** constructor calls, including generic `new Box<int>()` and cross-package `new pkg.Class()`
- **Access modifiers** `public`, `private`, `protected`, `internal`, enforced by the compiler
- **Inheritance** with `extends` (Go embedding), `super(...)` constructor chaining and `super.Method()`
- **`override`** with verification of parent method existence, visibility and exact signature
- **Abstract classes and methods**; concrete subclasses must implement every abstract method
- **Interfaces** with `function` syntax, and **`implements`** verified at compile time
- **Static fields and methods** (`Math.PI`, `Math.Add(1, 2)`)
- **Enums** lowered to typed `iota` constants with a `String()` method
- **Generic classes** — `class Repository<T>` or `class Repository[T any]`
- **Properties** with `get`/`set`, lowered to `GetX()`/`SetX(value)` methods

See [docs/LANGUAGE.md](docs/LANGUAGE.md) for the full language description.

## Examples

| File | Shows |
|---|---|
| [examples/hello.gpp](examples/hello.gpp) | inheritance, `super`, `override` |
| [examples/class.gpp](examples/class.gpp) | fields, initializers, statics, enums, properties |
| [examples/inheritance.gpp](examples/inheritance.gpp) | abstract classes, `super.Method()`, multi-level inheritance |
| [examples/interface.gpp](examples/interface.gpp) | interfaces and `implements` |
| [examples/generic.gpp](examples/generic.gpp) | generic classes and Go generic functions |
| [examples/goroutine.gpp](examples/goroutine.gpp) | goroutines, channels, `select` with classes |

```bash
gpp run examples/inheritance.gpp
```

## Current limitations (v0.1)

- **Static dispatch.** Inheritance is Go embedding, so a method of a parent
  class that calls `this.M()` always calls the parent's `M`, even when a
  subclass overrides it. Polymorphism works through interfaces (as in Go). A
  call of an abstract method through `this` is rejected at compile time
  because it could never reach an override.
- A subclass pointer is not assignable to a parent-class pointer
  (`*Dog` is not a `*Animal`); use interfaces for polymorphic variables.
- One constructor per class (no overloading); no method overloading.
- Static members are not inherited and cannot be declared in generic classes;
  inside a class, static members must be qualified (`Math.Add`, not `Add`).
- Class names should not be shadowed by local variables (`Class.Member` is
  resolved syntactically).
- Enum members cannot have explicit values.
- Property compound assignment (`p.X += 1`) evaluates the receiver twice.
- Member names are emitted unchanged, so Go's export rule (capitalization)
  still decides visibility across Go packages; the G++ modifiers are enforced
  inside the compiled package.
- Comments are not carried into generated Go. `gpp fmt` normalizes
  indentation and whitespace only; it does not reflow code.
- If an imported package cannot be loaded by the type checker (for example a
  module dependency outside the standard library), the type-dependent checks
  for its uses are left to `go build`.
- No exceptions; errors are Go error values.

## Roadmap

- Virtual dispatch for overridden methods called from parent classes
- Interop metadata so G++ classes can be extended across packages with full checks
- Source maps for runtime panics / stack traces
- Comment preservation and a full pretty-printer for `gpp fmt`
- Auto-properties, explicit enum values, `final` classes/methods
- Language server (diagnostics, completion)

## Development

```bash
go fmt ./...
go vet ./...
go test ./...
go build ./cmd/gpp
```

## License

MIT — see [LICENSE](LICENSE).
