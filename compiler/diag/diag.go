// Package diag provides source files, positions and compiler diagnostics
// that always refer to the original .gpp source.
package diag

import (
	"fmt"
	"sort"
	"strings"
)

// Pos is a byte offset into a source file. NoPos marks synthesized nodes.
type Pos int

// NoPos is the zero position used for compiler-synthesized nodes.
const NoPos Pos = -1

// IsValid reports whether p refers to a real source location.
func (p Pos) IsValid() bool { return p >= 0 }

// File is a .gpp source file.
type File struct {
	Name  string
	Src   []byte
	lines []int // offsets of the first byte of each line
}

// NewFile creates a source file and computes its line table.
func NewFile(name string, src []byte) *File {
	f := &File{Name: name, Src: src, lines: []int{0}}
	for i, b := range src {
		if b == '\n' {
			f.lines = append(f.lines, i+1)
		}
	}
	return f
}

// Position is a human readable file:line:column location.
type Position struct {
	Filename string
	Line     int
	Column   int
}

func (p Position) String() string {
	if p.Line == 0 {
		return p.Filename
	}
	return fmt.Sprintf("%s:%d:%d", p.Filename, p.Line, p.Column)
}

// Position converts an offset into a line/column position (1-based).
func (f *File) Position(p Pos) Position {
	if f == nil {
		return Position{}
	}
	if !p.IsValid() {
		return Position{Filename: f.Name}
	}
	off := int(p)
	i := sort.Search(len(f.lines), func(i int) bool { return f.lines[i] > off }) - 1
	if i < 0 {
		i = 0
	}
	return Position{Filename: f.Name, Line: i + 1, Column: off - f.lines[i] + 1}
}

// Error is a single diagnostic.
type Error struct {
	Pos Position
	Msg string
}

func (e *Error) Error() string {
	if e.Pos.Filename == "" {
		return "error: " + e.Msg
	}
	return e.Pos.String() + ": " + e.Msg
}

// ErrorList collects diagnostics.
type ErrorList []*Error

// Add appends a diagnostic.
func (l *ErrorList) Add(pos Position, format string, args ...any) {
	*l = append(*l, &Error{Pos: pos, Msg: fmt.Sprintf(format, args...)})
}

// Sort orders diagnostics by file and position and removes duplicates.
func (l *ErrorList) Sort() {
	s := *l
	sort.SliceStable(s, func(i, j int) bool {
		a, b := s[i].Pos, s[j].Pos
		if a.Filename != b.Filename {
			return a.Filename < b.Filename
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return a.Column < b.Column
	})
	out := s[:0]
	for i, e := range s {
		if i > 0 && e.Error() == s[i-1].Error() {
			continue
		}
		out = append(out, e)
	}
	*l = out
}

// Err returns the list as an error, or nil when empty.
func (l ErrorList) Err() error {
	if len(l) == 0 {
		return nil
	}
	return l
}

func (l ErrorList) Error() string {
	var b strings.Builder
	for i, e := range l {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(e.Error())
	}
	return b.String()
}
