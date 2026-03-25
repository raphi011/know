package parser

import (
	"testing"
)

func TestLexer_Keywords(t *testing.T) {
	tests := []struct {
		input string
		want  tokenType
	}{
		{"LIST", tokLIST},
		{"list", tokLIST},
		{"List", tokLIST},
		{"TABLE", tokTABLE},
		{"TASK", tokTASK},
		{"FROM", tokFROM},
		{"WHERE", tokWHERE},
		{"SORT", tokSORT},
		{"LIMIT", tokLIMIT},
		{"ASC", tokASC},
		{"DESC", tokDESC},
		{"CONTAIN", tokCONTAIN},
		{"CONTAINS", tokCONTAINS},
		{"WITHOUT", tokWITHOUT},
		{"ID", tokID},
		{"AS", tokAS},
	}
	for _, tt := range tests {
		l := newLexer(tt.input)
		tok := l.next()
		if tok.typ != tt.want {
			t.Errorf("keyword %q: got type %d, want %d", tt.input, tok.typ, tt.want)
		}
	}
}

func TestLexer_StringLiterals(t *testing.T) {
	tests := []struct {
		input   string
		wantVal string
		wantTyp tokenType
	}{
		{`"hello"`, "hello", tokSTRING},
		{`""`, "", tokSTRING},
		{`"with spaces"`, "with spaces", tokSTRING},
		{`"unterminated`, `"unterminated`, tokILLEGAL},
	}
	for _, tt := range tests {
		l := newLexer(tt.input)
		tok := l.next()
		if tok.typ != tt.wantTyp {
			t.Errorf("string %q: got type %d, want %d", tt.input, tok.typ, tt.wantTyp)
		}
		if tok.val != tt.wantVal {
			t.Errorf("string %q: got val %q, want %q", tt.input, tok.val, tt.wantVal)
		}
	}
}

func TestLexer_IntLiterals(t *testing.T) {
	l := newLexer("42")
	tok := l.next()
	if tok.typ != tokINT || tok.val != "42" {
		t.Errorf("got %+v, want INT 42", tok)
	}
}

func TestLexer_Identifiers(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"title", "title"},
		{"updated_at", "updated_at"},
		{"/projects/go", "/projects/go"},
		{"/daily", "/daily"},
		{"file.name", "file.name"},
		{"my-field", "my-field"},
	}
	for _, tt := range tests {
		l := newLexer(tt.input)
		tok := l.next()
		if tok.typ != tokIDENT {
			t.Errorf("ident %q: got type %d, want IDENT", tt.input, tok.typ)
		}
		if tok.val != tt.want {
			t.Errorf("ident %q: got val %q, want %q", tt.input, tok.val, tt.want)
		}
	}
}

func TestLexer_Operators(t *testing.T) {
	l := newLexer("= ,")
	eq := l.next()
	if eq.typ != tokEQ {
		t.Errorf("got %+v, want EQ", eq)
	}
	comma := l.next()
	if comma.typ != tokCOMMA {
		t.Errorf("got %+v, want COMMA", comma)
	}
}

func TestLexer_IllegalChars(t *testing.T) {
	l := newLexer("@")
	tok := l.next()
	if tok.typ != tokILLEGAL {
		t.Errorf("got %+v, want ILLEGAL", tok)
	}
}

func TestLexer_FullQuery(t *testing.T) {
	input := `TABLE title, labels AS "Tags" FROM /projects WHERE status = "active" SORT updated_at DESC LIMIT 10`

	l := newLexer(input)
	expected := []struct {
		typ tokenType
		val string
	}{
		{tokTABLE, "TABLE"},
		{tokIDENT, "title"},
		{tokCOMMA, ","},
		{tokIDENT, "labels"},
		{tokAS, "AS"},
		{tokSTRING, "Tags"},
		{tokFROM, "FROM"},
		{tokIDENT, "/projects"},
		{tokWHERE, "WHERE"},
		{tokIDENT, "status"},
		{tokEQ, "="},
		{tokSTRING, "active"},
		{tokSORT, "SORT"},
		{tokIDENT, "updated_at"},
		{tokDESC, "DESC"},
		{tokLIMIT, "LIMIT"},
		{tokINT, "10"},
		{tokEOF, ""},
	}

	for i, exp := range expected {
		tok := l.next()
		if tok.typ != exp.typ || tok.val != exp.val {
			t.Errorf("token[%d]: got {%d %q}, want {%d %q}", i, tok.typ, tok.val, exp.typ, exp.val)
		}
	}
}

func TestLexer_WhitespaceVariations(t *testing.T) {
	input := "LIST\n\ttitle\n  FROM\n/docs"
	l := newLexer(input)

	tok := l.next()
	if tok.typ != tokLIST {
		t.Errorf("got %+v, want LIST", tok)
	}
	tok = l.next()
	if tok.typ != tokIDENT || tok.val != "title" {
		t.Errorf("got %+v, want IDENT title", tok)
	}
	tok = l.next()
	if tok.typ != tokFROM {
		t.Errorf("got %+v, want FROM", tok)
	}
	tok = l.next()
	if tok.typ != tokIDENT || tok.val != "/docs" {
		t.Errorf("got %+v, want IDENT /docs", tok)
	}
}
