// Command gpp is the G++ compiler.
//
//	gpp build [-o output] [--keep] files...|dir
//	gpp run [--keep] files...|dir [-- args...]
//	gpp check files...|dir
//	gpp generate [-o dir] files...|dir
//	gpp fmt [--stdout] files...
//	gpp version
package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"gpp/compiler"
	"gpp/compiler/formatter"
)

const usage = `G++ compiler %s

Usage:
  gpp build [-o output] [--keep] <file.gpp...|dir>   compile to a native executable
  gpp run [--keep] <file.gpp...|dir> [-- args...]    compile and run
  gpp check <file.gpp...|dir>                        parse, analyze and type check
  gpp generate [-o dir] <file.gpp...|dir>            write Go source (default .gpp/generated)
  gpp fmt [--stdout] <file.gpp...>                   format source files in place
  gpp version                                        print version information
`

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, usage, compiler.Version)
		return 2
	}
	cmd, rest := args[0], args[1:]
	var err error
	switch cmd {
	case "build":
		err = cmdBuild(rest)
	case "run":
		var code int
		code, err = cmdRun(rest)
		if err == nil {
			return code
		}
	case "check":
		err = cmdCheck(rest)
	case "generate", "gen":
		err = cmdGenerate(rest)
	case "fmt":
		err = cmdFmt(rest)
	case "version", "--version", "-v":
		fmt.Printf("G++ compiler %s\nlanguage: G++ Language Specification %s\nbackend: Go (%s)\nsource: .gpp\n",
			compiler.Version, compiler.SpecVersion, runtime.Version())
		return 0
	case "help", "-h", "--help":
		fmt.Printf(usage, compiler.Version)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "gpp: unknown command %q\n\n", cmd)
		fmt.Fprintf(os.Stderr, usage, compiler.Version)
		return 2
	}
	if err != nil {
		var exitErr *exitError
		if errors.As(err, &exitErr) {
			return exitErr.code
		}
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

type exitError struct{ code int }

func (e *exitError) Error() string { return fmt.Sprintf("exit status %d", e.code) }

// flags is a tiny flag parser that allows flags before or after files.
type flags struct {
	output   string
	keep     bool
	stdout   bool
	files    []string
	progArgs []string
}

func parseFlags(args []string, allowed ...string) (*flags, error) {
	f := &flags{}
	ok := map[string]bool{}
	for _, a := range allowed {
		ok[a] = true
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			f.progArgs = args[i+1:]
			return f, nil
		case (a == "-o" || a == "--output") && ok["-o"]:
			if i+1 >= len(args) {
				return nil, fmt.Errorf("gpp: %s requires an argument", a)
			}
			i++
			f.output = args[i]
		case (a == "--keep" || a == "-keep") && ok["--keep"]:
			f.keep = true
		case (a == "--stdout" || a == "-stdout") && ok["--stdout"]:
			f.stdout = true
		case strings.HasPrefix(a, "-") && a != "-":
			return nil, fmt.Errorf("gpp: unknown flag %s", a)
		default:
			f.files = append(f.files, a)
		}
	}
	if len(f.files) == 0 {
		return nil, fmt.Errorf("gpp: no input files")
	}
	return f, nil
}

// compileArgs compiles the given files or directory.
func compileArgs(args []string) (*compiler.Result, string, error) {
	paths, err := compiler.ExpandPaths(args)
	if err != nil {
		return nil, "", err
	}
	srcs, err := compiler.LoadSources(paths)
	if err != nil {
		return nil, "", err
	}
	dir := filepath.Dir(paths[0])
	for _, p := range paths[1:] {
		if filepath.Dir(p) != dir {
			return nil, "", fmt.Errorf("gpp: all source files must be in the same directory")
		}
	}
	res, err := compiler.Compile(srcs)
	return res, dir, err
}

func writeGenerated(res *compiler.Result, dir string) ([]string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	var paths []string
	for _, f := range res.Files {
		p := filepath.Join(dir, f.Name)
		if err := os.WriteFile(p, f.Code, 0o644); err != nil {
			return nil, err
		}
		paths = append(paths, p)
	}
	return paths, nil
}

func cmdCheck(args []string) error {
	f, err := parseFlags(args)
	if err != nil {
		return err
	}
	res, _, err := compileArgs(f.files)
	if err != nil {
		return err
	}
	if res.TypeCheckIncomplete {
		fmt.Fprintln(os.Stderr, "gpp: note: some imports could not be loaded; type checking of their uses is left to the Go toolchain")
	}
	return nil
}

func cmdGenerate(args []string) error {
	f, err := parseFlags(args, "-o")
	if err != nil {
		return err
	}
	res, dir, err := compileArgs(f.files)
	if err != nil {
		return err
	}
	outDir := f.output
	if outDir == "" {
		outDir = filepath.Join(dir, ".gpp", "generated")
	}
	paths, err := writeGenerated(res, outDir)
	if err != nil {
		return err
	}
	for _, p := range paths {
		fmt.Println(p)
	}
	return nil
}

// goBuild writes the generated sources to a work directory under
// <srcdir>/.gpp and invokes the Go toolchain. The work directory lives next
// to the sources so that an enclosing Go module (and its dependencies) is
// used when present.
func goBuild(res *compiler.Result, srcDir, workName, output string) (string, error) {
	if res.Package != "main" {
		return "", fmt.Errorf("gpp: cannot build package %s: executables require package main", res.Package)
	}
	work := filepath.Join(srcDir, ".gpp", workName)
	paths, err := writeGenerated(res, work)
	if err != nil {
		return work, err
	}
	absOut, err := filepath.Abs(output)
	if err != nil {
		return work, err
	}
	goArgs := []string{"build", "-o", absOut}
	for _, p := range paths {
		goArgs = append(goArgs, filepath.Base(p))
	}
	cmd := exec.Command("go", goArgs...)
	cmd.Dir = work
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return work, fmt.Errorf("gpp: go build failed: %v (generated sources kept in %s)", err, work)
	}
	return work, nil
}

func defaultOutput(files []string) string {
	name := strings.TrimSuffix(filepath.Base(files[0]), ".gpp")
	if st, err := os.Stat(files[0]); err == nil && st.IsDir() {
		abs, _ := filepath.Abs(files[0])
		name = filepath.Base(abs)
	}
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return name
}

func cmdBuild(args []string) error {
	f, err := parseFlags(args, "-o", "--keep")
	if err != nil {
		return err
	}
	res, dir, err := compileArgs(f.files)
	if err != nil {
		return err
	}
	output := f.output
	if output == "" {
		output = defaultOutput(f.files)
	}
	work, err := goBuild(res, dir, "build", output)
	if err != nil {
		return err
	}
	if !f.keep {
		os.RemoveAll(work)
		removeIfEmpty(filepath.Dir(work))
	}
	return nil
}

func cmdRun(args []string) (int, error) {
	f, err := parseFlags(args, "--keep")
	if err != nil {
		return 0, err
	}
	res, dir, err := compileArgs(f.files)
	if err != nil {
		return 0, err
	}
	tmp, err := os.MkdirTemp("", "gpp-run-")
	if err != nil {
		return 0, err
	}
	bin := filepath.Join(tmp, defaultOutput(f.files))
	work, err := goBuild(res, dir, "run", bin)
	cleanup := func() {
		if !f.keep {
			os.RemoveAll(tmp)
			os.RemoveAll(work)
			removeIfEmpty(filepath.Dir(work))
		}
	}
	if err != nil {
		cleanup()
		return 0, err
	}
	cmd := exec.Command(bin, f.progArgs...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	runErr := cmd.Run()
	cleanup()
	if f.keep {
		fmt.Fprintf(os.Stderr, "gpp: kept generated sources in %s and binary %s\n", work, bin)
	}
	var ee *exec.ExitError
	if errors.As(runErr, &ee) {
		return ee.ExitCode(), nil
	}
	if runErr != nil {
		return 0, runErr
	}
	return 0, nil
}

func removeIfEmpty(dir string) {
	entries, err := os.ReadDir(dir)
	if err == nil && len(entries) == 0 {
		os.Remove(dir)
	}
}

func cmdFmt(args []string) error {
	f, err := parseFlags(args, "--stdout")
	if err != nil {
		return err
	}
	paths, err := compiler.ExpandPaths(f.files)
	if err != nil {
		return err
	}
	for _, p := range paths {
		src, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		out, err := formatter.Format(p, src)
		if err != nil {
			return err
		}
		if f.stdout {
			os.Stdout.Write(out)
			continue
		}
		if string(out) != string(src) {
			if err := os.WriteFile(p, out, 0o644); err != nil {
				return err
			}
			fmt.Println(p)
		}
	}
	return nil
}
