package integration

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/raptordnj/gpp/compiler"
)

// TestPrograms compiles every tests/integration/*.gpp and ../../examples/*.gpp
// program with the G++ compiler, builds it with the Go toolchain, runs it and
// compares stdout with the matching .out file.
func TestPrograms(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}
	files, _ := filepath.Glob("*.gpp")
	examples, _ := filepath.Glob("../../examples/*.gpp")
	files = append(files, examples...)
	if len(files) == 0 {
		t.Fatal("no integration programs found")
	}
	for _, file := range files {
		file := file
		t.Run(filepath.Base(file), func(t *testing.T) {
			t.Parallel()
			want, err := os.ReadFile(strings.TrimSuffix(file, ".gpp") + ".out")
			if err != nil {
				t.Fatalf("missing expected output: %v", err)
			}
			src, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			res, err := compiler.CompileSource(file, src)
			if err != nil {
				t.Fatalf("compile error:\n%v", err)
			}
			dir := t.TempDir()
			var goFiles []string
			for _, f := range res.Files {
				p := filepath.Join(dir, f.Name)
				if err := os.WriteFile(p, f.Code, 0o644); err != nil {
					t.Fatal(err)
				}
				goFiles = append(goFiles, f.Name)
			}
			bin := filepath.Join(dir, "prog")
			build := exec.Command("go", append([]string{"build", "-o", bin}, goFiles...)...)
			build.Dir = dir
			if out, err := build.CombinedOutput(); err != nil {
				t.Fatalf("go build failed: %v\n%s\n%s", err, out, res.Files[0].Code)
			}
			var stdout bytes.Buffer
			run := exec.Command(bin)
			run.Stdout = &stdout
			if err := run.Run(); err != nil {
				t.Fatalf("program failed: %v", err)
			}
			if got := stdout.String(); got != string(want) {
				t.Fatalf("output mismatch\n--- got\n%s--- want\n%s", got, want)
			}
		})
	}
}
