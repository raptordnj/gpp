package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const hello = `package main

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
`

func TestCLI(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "hello.gpp")
	if err := os.WriteFile(src, []byte(hello), 0o644); err != nil {
		t.Fatal(err)
	}

	if code := run([]string{"check", src}); code != 0 {
		t.Fatalf("check exited with %d", code)
	}

	if code := run([]string{"generate", src}); code != 0 {
		t.Fatalf("generate exited with %d", code)
	}
	gen, err := os.ReadFile(filepath.Join(dir, ".gpp", "generated", "hello.go"))
	if err != nil {
		t.Fatalf("generated file missing: %v", err)
	}
	if !strings.Contains(string(gen), "func NewDeveloper(name string) *Developer") {
		t.Fatalf("unexpected generated code:\n%s", gen)
	}

	bin := filepath.Join(dir, "app")
	if code := run([]string{"build", src, "-o", bin}); code != 0 {
		t.Fatalf("build exited with %d", code)
	}
	out, err := exec.Command(bin).Output()
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "Hello, developer!\n" {
		t.Fatalf("unexpected output %q", out)
	}
	if _, err := os.Stat(filepath.Join(dir, ".gpp", "build")); !os.IsNotExist(err) {
		t.Errorf("build work directory should be cleaned up")
	}

	bad := filepath.Join(dir, "bad.gpp")
	os.WriteFile(bad, []byte("package main\nfunc main() { _ = new Missing() }\n"), 0o644)
	if code := run([]string{"check", bad}); code != 1 {
		t.Fatalf("check of invalid program exited with %d, want 1", code)
	}
	if code := run([]string{"nonsense"}); code != 2 {
		t.Fatalf("unknown command exited with %d, want 2", code)
	}
	if code := run([]string{"version"}); code != 0 {
		t.Fatalf("version exited with %d", code)
	}
}
