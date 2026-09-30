package lexer

import (
	"go/token"
	"unicode"
	"unicode/utf8"

	"gpp/compiler/diag"
)

// Lexer scans a source file into tokens.
type Lexer struct {
	file         *diag.File
	src          []byte
	off          int
	insertSemi   bool
	keepComments bool
	Errors       diag.ErrorList
}

// New creates a lexer. When keepComments is true COMMENT tokens are emitted.
func New(file *diag.File, keepComments bool) *Lexer {
	return &Lexer{file: file, src: file.Src, keepComments: keepComments}
}

// Tokenize scans the whole file, returning all tokens ending with EOF.
func Tokenize(file *diag.File, keepComments bool) ([]Token, diag.ErrorList) {
	l := New(file, keepComments)
	var toks []Token
	for {
		t := l.Next()
		toks = append(toks, t)
		if t.Kind == token.EOF {
			break
		}
	}
	return toks, l.Errors
}

func (l *Lexer) errorf(off int, format string, args ...any) {
	l.Errors.Add(l.file.Position(diag.Pos(off)), format, args...)
}

func (l *Lexer) peek(n int) byte {
	if l.off+n < len(l.src) {
		return l.src[l.off+n]
	}
	return 0
}

func isLetter(r rune) bool {
	return r == '_' || 'a' <= r && r <= 'z' || 'A' <= r && r <= 'Z' || r >= utf8.RuneSelf && unicode.IsLetter(r)
}

func isDigit(r rune) bool {
	return '0' <= r && r <= '9' || r >= utf8.RuneSelf && unicode.IsDigit(r)
}

// Next returns the next token.
func (l *Lexer) Next() Token {
redo:
	// skip whitespace, stopping at newlines that need a semicolon
	for l.off < len(l.src) {
		c := l.src[l.off]
		if c == '\n' && l.insertSemi {
			break
		}
		if c != ' ' && c != '\t' && c != '\r' && c != '\n' {
			break
		}
		l.off++
	}
	start := l.off
	if l.off >= len(l.src) {
		if l.insertSemi {
			l.insertSemi = false
			return Token{Kind: token.SEMICOLON, Lit: "\n", Pos: diag.Pos(start)}
		}
		return Token{Kind: token.EOF, Pos: diag.Pos(start)}
	}

	c := l.src[l.off]
	if c == '\n' { // only reached when insertSemi
		l.off++
		l.insertSemi = false
		return Token{Kind: token.SEMICOLON, Lit: "\n", Pos: diag.Pos(start)}
	}

	r, w := utf8.DecodeRune(l.src[l.off:])
	switch {
	case isLetter(r):
		l.off += w
		for l.off < len(l.src) {
			r, w := utf8.DecodeRune(l.src[l.off:])
			if !isLetter(r) && !isDigit(r) {
				break
			}
			l.off += w
		}
		lit := string(l.src[start:l.off])
		kind := token.Lookup(lit)
		switch kind {
		case token.IDENT, token.BREAK, token.CONTINUE, token.FALLTHROUGH, token.RETURN:
			l.insertSemi = true
		default:
			l.insertSemi = false
		}
		return Token{Kind: kind, Lit: lit, Pos: diag.Pos(start)}
	case isDigit(r) || c == '.' && isDigit(rune(l.peek(1))):
		kind := l.scanNumber()
		l.insertSemi = true
		return Token{Kind: kind, Lit: string(l.src[start:l.off]), Pos: diag.Pos(start)}
	}

	l.off++
	var kind Kind
	insertSemi := false
	switch c {
	case '"':
		l.scanString(start)
		return l.lit(token.STRING, start)
	case '`':
		l.scanRawString(start)
		return l.lit(token.STRING, start)
	case '\'':
		l.scanRune(start)
		return l.lit(token.CHAR, start)
	case '/':
		if l.peek(0) == '/' || l.peek(0) == '*' {
			hadNewline := l.scanComment(start)
			if l.insertSemi && hadNewline {
				// the comment ends the line: emit the automatic semicolon first
				l.insertSemi = false
				if !l.keepComments {
					return Token{Kind: token.SEMICOLON, Lit: "\n", Pos: diag.Pos(start)}
				}
				l.off = start // rescan the comment after the semicolon
				return Token{Kind: token.SEMICOLON, Lit: "\n", Pos: diag.Pos(start)}
			}
			if l.keepComments {
				return Token{Kind: token.COMMENT, Lit: string(l.src[start:l.off]), Pos: diag.Pos(start)}
			}
			goto redo
		}
		kind = l.switch2(token.QUO, token.QUO_ASSIGN)
	case '.':
		if l.peek(0) == '.' && l.peek(1) == '.' {
			l.off += 2
			kind = token.ELLIPSIS
		} else {
			kind = token.PERIOD
		}
	case ',':
		kind = token.COMMA
	case ';':
		kind = token.SEMICOLON
	case '(':
		kind = token.LPAREN
	case ')':
		kind, insertSemi = token.RPAREN, true
	case '[':
		kind = token.LBRACK
	case ']':
		kind, insertSemi = token.RBRACK, true
	case '{':
		kind = token.LBRACE
	case '}':
		kind, insertSemi = token.RBRACE, true
	case ':':
		kind = l.switch2(token.COLON, token.DEFINE)
	case '+':
		kind = l.switch3(token.ADD, token.ADD_ASSIGN, '+', token.INC)
		insertSemi = kind == token.INC
	case '-':
		kind = l.switch3(token.SUB, token.SUB_ASSIGN, '-', token.DEC)
		insertSemi = kind == token.DEC
	case '*':
		kind = l.switch2(token.MUL, token.MUL_ASSIGN)
	case '%':
		kind = l.switch2(token.REM, token.REM_ASSIGN)
	case '^':
		kind = l.switch2(token.XOR, token.XOR_ASSIGN)
	case '<':
		if l.peek(0) == '-' {
			l.off++
			kind = token.ARROW
		} else {
			kind = l.switch4(token.LSS, token.LEQ, '<', token.SHL, token.SHL_ASSIGN)
		}
	case '>':
		kind = l.switch4(token.GTR, token.GEQ, '>', token.SHR, token.SHR_ASSIGN)
	case '=':
		kind = l.switch2(token.ASSIGN, token.EQL)
	case '!':
		kind = l.switch2(token.NOT, token.NEQ)
	case '&':
		if l.peek(0) == '^' {
			l.off++
			kind = l.switch2(token.AND_NOT, token.AND_NOT_ASSIGN)
		} else {
			kind = l.switch3(token.AND, token.AND_ASSIGN, '&', token.LAND)
		}
	case '|':
		kind = l.switch3(token.OR, token.OR_ASSIGN, '|', token.LOR)
	case '~':
		kind = token.TILDE
	default:
		l.off = start + w
		l.errorf(start, "invalid character %q", r)
		l.insertSemi = false
		return Token{Kind: token.ILLEGAL, Lit: string(r), Pos: diag.Pos(start)}
	}
	l.insertSemi = insertSemi
	return Token{Kind: kind, Pos: diag.Pos(start)}
}

func (l *Lexer) lit(kind Kind, start int) Token {
	l.insertSemi = true
	return Token{Kind: kind, Lit: string(l.src[start:l.off]), Pos: diag.Pos(start)}
}

func (l *Lexer) switch2(k0, k1 Kind) Kind {
	if l.peek(0) == '=' {
		l.off++
		return k1
	}
	return k0
}

func (l *Lexer) switch3(k0, k1 Kind, ch2 byte, k2 Kind) Kind {
	if l.peek(0) == '=' {
		l.off++
		return k1
	}
	if l.peek(0) == ch2 {
		l.off++
		return k2
	}
	return k0
}

func (l *Lexer) switch4(k0, k1 Kind, ch2 byte, k2, k3 Kind) Kind {
	if l.peek(0) == '=' {
		l.off++
		return k1
	}
	if l.peek(0) == ch2 {
		l.off++
		if l.peek(0) == '=' {
			l.off++
			return k3
		}
		return k2
	}
	return k0
}

// scanComment consumes a comment starting at '/'. It reports whether the
// comment contains or ends at a newline (relevant for semicolon insertion).
func (l *Lexer) scanComment(start int) bool {
	if l.src[l.off] == '/' { // line comment; do not consume the newline
		for l.off < len(l.src) && l.src[l.off] != '\n' {
			l.off++
		}
		return true
	}
	l.off++ // '*'
	newline := false
	for {
		if l.off >= len(l.src) {
			l.errorf(start, "comment not terminated")
			return true
		}
		if l.src[l.off] == '\n' {
			newline = true
		}
		if l.src[l.off] == '*' && l.peek(1) == '/' {
			l.off += 2
			return newline
		}
		l.off++
	}
}

func isHex(c byte) bool {
	return '0' <= c && c <= '9' || 'a' <= c && c <= 'f' || 'A' <= c && c <= 'F'
}

// scanNumber scans integer, float and imaginary literals. It is permissive;
// exact validation is left to the Go type checker, which receives the
// literal text unchanged.
func (l *Lexer) scanNumber() Kind {
	kind := token.INT
	digits := func(hex bool) {
		for l.off < len(l.src) {
			c := l.src[l.off]
			if c == '_' || '0' <= c && c <= '9' || hex && isHex(c) {
				l.off++
				continue
			}
			break
		}
	}
	hex := false
	if l.src[l.off] == '0' && l.off+1 < len(l.src) {
		switch l.src[l.off+1] {
		case 'x', 'X':
			hex = true
			l.off += 2
		case 'b', 'B', 'o', 'O':
			l.off += 2
		}
	}
	digits(hex)
	if l.off < len(l.src) && l.src[l.off] == '.' && l.peek(1) != '.' {
		kind = token.FLOAT
		l.off++
		digits(hex)
	}
	if l.off < len(l.src) {
		c := l.src[l.off]
		if !hex && (c == 'e' || c == 'E') || hex && (c == 'p' || c == 'P') {
			kind = token.FLOAT
			l.off++
			if l.off < len(l.src) && (l.src[l.off] == '+' || l.src[l.off] == '-') {
				l.off++
			}
			digits(false)
		}
	}
	if l.off < len(l.src) && l.src[l.off] == 'i' {
		kind = token.IMAG
		l.off++
	}
	return kind
}

func (l *Lexer) scanEscape(quote byte) {
	if l.off >= len(l.src) {
		return
	}
	c := l.src[l.off]
	l.off++
	switch c {
	case 'a', 'b', 'f', 'n', 'r', 't', 'v', '\\', quote:
	case 'x':
		l.off += 2
	case 'u':
		l.off += 4
	case 'U':
		l.off += 8
	default:
		if '0' <= c && c <= '7' {
			l.off += 2
		} else {
			l.errorf(l.off-2, "unknown escape sequence")
		}
	}
	if l.off > len(l.src) {
		l.off = len(l.src)
	}
}

func (l *Lexer) scanString(start int) {
	for {
		if l.off >= len(l.src) || l.src[l.off] == '\n' {
			l.errorf(start, "string literal not terminated")
			return
		}
		c := l.src[l.off]
		l.off++
		if c == '"' {
			return
		}
		if c == '\\' {
			l.scanEscape('"')
		}
	}
}

func (l *Lexer) scanRawString(start int) {
	for {
		if l.off >= len(l.src) {
			l.errorf(start, "raw string literal not terminated")
			return
		}
		c := l.src[l.off]
		l.off++
		if c == '`' {
			return
		}
	}
}

func (l *Lexer) scanRune(start int) {
	n := 0
	for {
		if l.off >= len(l.src) || l.src[l.off] == '\n' {
			l.errorf(start, "rune literal not terminated")
			return
		}
		c := l.src[l.off]
		if c == '\'' {
			l.off++
			break
		}
		if c == '\\' {
			l.off++
			l.scanEscape('\'')
		} else {
			_, w := utf8.DecodeRune(l.src[l.off:])
			l.off += w
		}
		n++
	}
	if n != 1 {
		l.errorf(start, "invalid rune literal")
	}
}
