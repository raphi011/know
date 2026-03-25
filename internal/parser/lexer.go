package parser

import "strings"

type tokenType int

const (
	// Keywords
	tokLIST     tokenType = iota
	tokTABLE              // TABLE
	tokTASK               // TASK
	tokFROM               // FROM
	tokWHERE              // WHERE
	tokSORT               // SORT
	tokLIMIT              // LIMIT
	tokASC                // ASC
	tokDESC               // DESC
	tokCONTAIN            // CONTAIN
	tokCONTAINS           // CONTAINS
	tokWITHOUT            // WITHOUT
	tokID                 // ID
	tokAS                 // AS

	// Literals
	tokSTRING // "double-quoted"
	tokINT    // 42

	// Other
	tokIDENT   // identifiers and paths
	tokEQ      // =
	tokCOMMA   // ,
	tokEOF     // end of input
	tokILLEGAL // unrecognized
)

var keywords = map[string]tokenType{
	"list":     tokLIST,
	"table":    tokTABLE,
	"task":     tokTASK,
	"from":     tokFROM,
	"where":    tokWHERE,
	"sort":     tokSORT,
	"limit":    tokLIMIT,
	"asc":      tokASC,
	"desc":     tokDESC,
	"contain":  tokCONTAIN,
	"contains": tokCONTAINS,
	"without":  tokWITHOUT,
	"id":       tokID,
	"as":       tokAS,
}

type token struct {
	typ tokenType
	val string
}

type lexer struct {
	input string
	pos   int
}

func newLexer(input string) *lexer {
	return &lexer{input: input}
}

func (l *lexer) next() token {
	l.skipWhitespace()

	if l.pos >= len(l.input) {
		return token{typ: tokEOF}
	}

	ch := l.input[l.pos]

	switch {
	case ch == '"':
		return l.lexString()
	case ch == '=':
		l.pos++
		return token{typ: tokEQ, val: "="}
	case ch == ',':
		l.pos++
		return token{typ: tokCOMMA, val: ","}
	case isDigit(ch):
		return l.lexInt()
	case isIdentStart(ch):
		return l.lexIdent()
	default:
		l.pos++
		return token{typ: tokILLEGAL, val: string(ch)}
	}
}

func (l *lexer) skipWhitespace() {
	for l.pos < len(l.input) {
		ch := l.input[l.pos]
		if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' {
			l.pos++
		} else {
			break
		}
	}
}

func (l *lexer) lexString() token {
	l.pos++ // skip opening quote
	start := l.pos
	for l.pos < len(l.input) {
		if l.input[l.pos] == '"' {
			val := l.input[start:l.pos]
			l.pos++ // skip closing quote
			return token{typ: tokSTRING, val: val}
		}
		l.pos++
	}
	return token{typ: tokILLEGAL, val: l.input[start-1:]}
}

func (l *lexer) lexInt() token {
	start := l.pos
	for l.pos < len(l.input) && isDigit(l.input[l.pos]) {
		l.pos++
	}
	return token{typ: tokINT, val: l.input[start:l.pos]}
}

func (l *lexer) lexIdent() token {
	start := l.pos
	for l.pos < len(l.input) && isIdentChar(l.input[l.pos]) {
		l.pos++
	}
	val := l.input[start:l.pos]

	if typ, ok := keywords[strings.ToLower(val)]; ok {
		return token{typ: typ, val: val}
	}
	return token{typ: tokIDENT, val: val}
}

func isDigit(ch byte) bool {
	return ch >= '0' && ch <= '9'
}

func isIdentStart(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '_' || ch == '/' || ch == '.'
}

func isIdentChar(ch byte) bool {
	return isIdentStart(ch) || isDigit(ch) || ch == '-'
}
