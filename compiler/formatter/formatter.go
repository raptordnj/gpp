// Package formatter implements "gpp fmt": a conservative, token-based
// formatter for G++ source. It re-indents lines with tabs according to
// bracket nesting (gofmt style, with case/default labels at switch level),
// trims trailing whitespace, collapses runs of blank lines and ensures a
// final newline. Comments and the contents of multi-line literals are kept
// verbatim. The input must parse successfully.
package formatter

import (
	"bytes"
	"go/token"
	"strings"

	"github.com/raptordnj/gpp/compiler/diag"
	"github.com/raptordnj/gpp/compiler/lexer"
	"github.com/raptordnj/gpp/compiler/parser"
)

type lineInfo struct {
	depth     int  // bracket depth before the first token
	first     bool // has a first token
	firstTok  token.Token
	verbatim  bool // line starts inside a multi-line token
	lastTok   token.Token
	hasTokens bool
}

// Format formats G++ source code.
func Format(name string, src []byte) ([]byte, error) {
	file := diag.NewFile(name, src)
	if _, err := parser.ParseFile(file); err != nil {
		return nil, err
	}
	toks, errs := lexer.Tokenize(file, true)
	if len(errs) > 0 {
		return nil, errs
	}
	lines := strings.Split(string(src), "\n")
	info := make([]lineInfo, len(lines)+1)

	depth := 0
	for _, t := range toks {
		if t.Kind == token.EOF || t.Kind == token.SEMICOLON && t.Lit == "\n" {
			continue
		}
		start := file.Position(t.Pos)
		ln := start.Line - 1
		switch t.Kind {
		case token.RBRACE, token.RPAREN, token.RBRACK:
			depth--
		}
		li := &info[ln]
		if !li.first {
			li.first = true
			li.firstTok = t.Kind
			li.depth = depth
			if isCloser(t.Kind) {
				li.depth = depth // already decremented
			}
		}
		li.lastTok = t.Kind
		li.hasTokens = true
		switch t.Kind {
		case token.LBRACE, token.LPAREN, token.LBRACK:
			depth++
		}
		// multi-line tokens (raw strings, block comments)
		if n := strings.Count(tokenText(t, src), "\n"); n > 0 {
			for k := 1; k <= n; k++ {
				info[ln+k].verbatim = true
			}
			end := &info[ln+n]
			end.lastTok = t.Kind
		}
	}

	var out bytes.Buffer
	blank := 0
	prevCode := -1 // index of the previous line with tokens
	for i, line := range lines {
		if i == len(lines)-1 && line == "" {
			break // trailing newline handled below
		}
		li := info[i]
		if li.verbatim {
			out.WriteString(strings.TrimRight(line, " \t\r"))
			out.WriteByte('\n')
			blank = 0
			continue
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			blank++
			if blank > 1 || out.Len() == 0 {
				continue
			}
			out.WriteByte('\n')
			continue
		}
		blank = 0
		indent := li.depth
		if li.firstTok == token.CASE || li.firstTok == token.DEFAULT {
			indent--
		}
		// Continuation of an expression from the previous line, unless an
		// opened bracket already provides the indentation.
		if prevCode >= 0 && continues(info[prevCode].lastTok) && !isCloser(li.firstTok) && li.depth == info[prevCode].depth {
			indent++
		}
		if indent < 0 {
			indent = 0
		}
		out.WriteString(strings.Repeat("\t", indent))
		out.WriteString(trimmed)
		out.WriteByte('\n')
		if li.hasTokens {
			prevCode = i
		}
	}
	res := bytes.TrimRight(out.Bytes(), "\n")
	return append(res, '\n'), nil
}

func tokenText(t lexer.Token, src []byte) string {
	if t.Kind == token.STRING || t.Kind == token.COMMENT {
		return t.Lit
	}
	return ""
}

func isCloser(k token.Token) bool {
	return k == token.RBRACE || k == token.RPAREN || k == token.RBRACK
}

// continues reports whether a line ending with tok continues on the next
// line as part of the same expression.
func continues(tok token.Token) bool {
	switch tok {
	case token.ADD, token.SUB, token.MUL, token.QUO, token.REM, token.AND, token.OR,
		token.XOR, token.SHL, token.SHR, token.AND_NOT, token.LAND, token.LOR,
		token.EQL, token.NEQ, token.LSS, token.LEQ, token.GTR, token.GEQ,
		token.PERIOD, token.ASSIGN, token.DEFINE, token.ARROW:
		return true
	}
	return false
}
