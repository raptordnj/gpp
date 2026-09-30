# G++ Language Specification v0.1

G++ is a superset of the Go programming language. Every construct described in
the [Go specification](https://go.dev/ref/spec) keeps its syntax and meaning in
G++. This document describes only what G++ adds and how each addition is
lowered to Go. Source files use the `.gpp` extension.

## 1. Lexical elements

G++ uses Go's tokens, literals, comments and automatic semicolon insertion.

### 1.1 Contextual keywords

In addition to Go's 25 keywords, G++ recognizes these words where a G++
construct can start:

```text
function class abstract extends implements constructor override
public private protected internal static property enum this super new
```

They are *contextual*: outside G++ constructs they are ordinary identifiers,
so Go code such as `class := 1` remains valid. `this` names the receiver of
class methods, and `new` keeps its Go meaning when followed by `(`.

### 1.2 Generic brackets

Generic type parameters and arguments may be written with Go brackets
(`Box[int]`) or with angle brackets (`Box<int>`). Angle brackets are accepted
wherever a type is expected, in class and interface headers, after `extends`
and `implements`, and after `new`. A closing `>` at the end of a line ends the
statement, just like `]`. Nested closings (`Box<List<int>>`) are split
automatically. In expression position (other than after `new`), use Go
brackets: `Map[int, string](xs, f)`.

## 2. Functions

`function` is a synonym for `func`:

```gpp
function add(a int, b int) int { return a + b }
func sub(a int, b int) int { return a - b }
```

## 3. Classes

```ebnf
ClassDecl   = [ "abstract" ] "class" identifier [ TypeParams ]
              [ "extends" ClassRef ] [ "implements" ClassRef { "," ClassRef } ]
              "{" { Member ";" } "}" .
TypeParams  = "<" TypeParam { "," TypeParam } ">" | "[" TypeParamList "]" .
TypeParam   = identifier [ Constraint ] .            // default constraint: any
ClassRef    = TypeName [ TypeArgs ] .
Member      = Modifiers ( Field | Method | Constructor | Property ) .
Modifiers   = { "public" | "private" | "protected" | "internal"
              | "static" | "abstract" | "override" } .
Field       = IdentifierList Type [ "=" Expression ] .
Method      = ( "function" | "func" ) identifier Signature [ Block ] .
Constructor = "constructor" Parameters Block .
Property    = "property" identifier Type "{" [ "get" Block ] [ "set" Block ] "}" .
```

A class declares a named struct type. Lowering:

| G++ | Go |
|---|---|
| `class C { x int }` | `type C struct { x int }` |
| `class C extends P` | `type C struct { P; ... }` (embedding) |
| `class C<T>` | `type C[T any] struct { ... }` |
| instance method `M` | `func (this *C) M(...)` |
| constructor | `func NewC(params) *C` and `func (this *C) initC(params)` |
| `new C(args)` | `NewC(args)` |
| `new pkg.C(args)` | `pkg.NewC(args)` |
| `new C<T>(args)` | `NewC[T](args)` |
| static field `F` | `var C_F T = value` |
| static method `M` | `func C_M(...)` |
| `C.F`, `C.M(...)` | `C_F`, `C_M(...)` |
| property `P` | `func (this *C) GetP() T`, `func (this *C) SetP(value T)` |

### 3.1 Fields

Fields may have initializers: `private count int = 10`. Initializers run
during construction, after the parent class has been initialized and before
the constructor body. A field initializer requires a single field name.

### 3.2 Constructors and `new`

A class has at most one `constructor`. `new C(args)` allocates a `*C`, runs
the construction sequence and returns the pointer:

1. the parent's construction (explicit `super(args)` or implicit `super()`),
2. field initializers, in declaration order,
3. the constructor body.

Every non-abstract class gets a `NewC` function, even without a declared
constructor. The init method `initC` is only generated when construction
needs code; otherwise `NewC` returns `&C{}`.

The compiler checks the number of constructor arguments:

```text
constructor for class 'User' expects 2 arguments, got 1
```

`new(T)` is Go's built-in allocation and is unaffected (it runs no G++
constructor).

A `private` constructor can only be used inside the class (for example for
singletons created by a static method).

### 3.3 Methods and `this`

Methods use pointer receivers named `this`. `this` is available in instance
methods, constructors, field initializers and property accessors. Using it
anywhere else is an error.

### 3.4 Access modifiers

| Modifier | Accessible from |
|---|---|
| `public` (default when omitted) | anywhere |
| `internal` | anywhere in the same package |
| `protected` | the declaring class and its subclasses |
| `private` | the declaring class only |

Access is checked by the compiler using full type information (after type
inference), for fields, methods, properties, static members and constructors:

```text
main.gpp:14:9: cannot access private field 'password' of class 'User'
```

Member names are emitted unchanged; Go's capitalization rule still controls
visibility across Go packages.

### 3.5 Inheritance: `extends` and `super`

A class may extend one parent class. The child embeds the parent struct, so
all non-private parent members are promoted and the child satisfies every
interface the parent satisfies.

- `super(args)` calls the parent constructor. It must be the first statement
  of the constructor. If omitted, the parent is initialized with no
  arguments; if the parent constructor requires arguments, the child must
  declare a constructor that calls `super(...)`.
- `super.M(...)` calls the parent's implementation of `M`; `super.f`
  accesses a parent field. Lowered to `this.P.M(...)`.
- Inheritance cycles are rejected.
- A class may also extend a class from another G++/Go package
  (`extends pkg.Base`); `super(args)` then lowers to
  `this.Base = *pkg.NewBase(args)`, and override checks happen during type checking.

**Dispatch is static** (as with Go embedding): a parent method calling
`this.M()` invokes the parent's `M` even if a subclass overrides it. Dynamic
dispatch is available through interfaces. Calling an *abstract* method
through `this` is therefore an error.

### 3.6 `override`

A method that replaces a parent method must be marked `override`. The
compiler verifies that:

1. the class has a parent class,
2. a non-private instance method with that name exists in an ancestor,
3. the signatures are identical,
4. the override does not reduce visibility.

A method that has the same name as a parent method but lacks `override` is
an error.

```text
error: method 'Speak' marked override but no overridable method exists in parent class 'Animal'
error: method 'Speak' overrides 'Animal.Speak' with a different signature: have () string, want ()
```

### 3.7 Abstract classes

`abstract class` may declare `abstract function` members without bodies.

- Abstract classes cannot be instantiated (`new Animal()` is an error) and
  have no `NewC` function.
- A concrete class must implement every inherited abstract method (with
  `override`); otherwise: `class 'Dog' does not implement abstract method
  'Speak' from class 'Animal'`.
- Abstract methods are lowered to stubs that panic, so the embedding class
  still satisfies interfaces.
- Only abstract classes may declare abstract methods; abstract methods cannot
  be private.

### 3.8 Static members

`static` fields and methods belong to the class. They are accessed as
`Class.member`, lowered to package-level declarations `Class_member`.
Static members are not inherited, cannot use `this` or `super`, and cannot be
declared in generic classes.

### 3.9 Properties

```gpp
public property Name string {
    get { return this._name }
    set { this._name = value }
}
```

Properties are accessed like fields; the compiler rewrites reads to
`GetName()` and writes to `SetName(v)` using type information. The setter's
parameter is named `value`. A property without `set` is read-only; without
`get` it is write-only. `++`, `--` and compound assignments are supported;
taking the address of a property is not.

### 3.10 Generic classes

Class type parameters use Go's generic type system; no separate generic
runtime exists.

```gpp
class Repository<T> {
    private items []T
    public function Add(item T) { this.items = append(this.items, item) }
}
repo := new Repository<string>()
```

Constraints are written after the parameter name: `class Pair<K comparable, V any>`.

## 4. Interfaces

```gpp
interface Repository {
    function Find(id int) (*User, error)
    function Delete(id int) error
}
```

lowers to `type Repository interface { ... }`. The `function` keyword is
optional; embedded interfaces and type constraints are allowed. Go's
`type I interface { ... }` syntax remains available. Interfaces may be
generic: `interface Container<T> { function Get() T }`.

### 4.1 `implements`

`class C implements I, J` is a compile-time check: `*C` (including inherited
methods) must implement every listed interface. Because Go interfaces are
structural, `implements` never changes behavior. For non-generic classes the
check is also emitted as `var _ I = (*C)(nil)`.

```text
class 'UserRepository' does not implement interface 'Repository': missing method 'Delete'
```

## 5. Enums

```gpp
enum Status {
    Pending
    Processing
    Completed
}
```

Members are separated by newlines or commas. Lowering:

```go
type Status int

const (
	StatusPending Status = iota
	StatusProcessing
	StatusCompleted
)

func (e Status) String() string { ... } // "Pending", "Processing", ...
```

Members are referenced as `Status.Pending` (or directly as the generated
`StatusPending`). Referring to a missing member is an error.

## 6. Everything else is Go

Error handling (`error` values), goroutines, channels, `select`, `defer`,
packages and imports, generics, closures, and the whole standard library work
exactly as in Go. G++ v0.1 has no exceptions.

## 7. Diagnostics

All diagnostics use `file.gpp:line:column: message` positions in the original
source, including errors found by Go type checking of the lowered program.

## 8. Toolchain

See the README for the `gpp` commands (`build`, `run`, `check`, `generate`,
`fmt`, `version`).
