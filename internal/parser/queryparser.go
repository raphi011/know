package parser

import (
	"fmt"
	"strconv"
)

type queryParser struct {
	lex  *lexer
	cur  token
	peek token
}

func newParser(input string) *queryParser {
	p := &queryParser{lex: newLexer(input)}
	// Prime cur and peek.
	p.cur = p.lex.next()
	p.peek = p.lex.next()
	return p
}

func (p *queryParser) advance() {
	p.cur = p.peek
	p.peek = p.lex.next()
}

func (p *queryParser) expect(typ tokenType) (token, error) {
	if p.cur.typ != typ {
		return p.cur, fmt.Errorf("expected %s, got %q", tokenName(typ), p.cur.val)
	}
	tok := p.cur
	p.advance()
	return tok, nil
}

func parseQueryBlock(raw string) QueryBlock {
	p := newParser(raw)
	block := QueryBlock{
		SortField: DefaultSortField,
		Limit:     DefaultLimit,
	}

	if err := p.parseFormatDecl(&block); err != nil {
		block.Error = err.Error()
		return block
	}

	if err := p.parseClauses(&block); err != nil {
		block.Error = err.Error()
		return block
	}

	return block
}

func (p *queryParser) parseFormatDecl(block *QueryBlock) error {
	switch p.cur.typ {
	case tokLIST:
		block.Format = FormatList
		p.advance()
		return p.parseListFields(block)
	case tokTABLE:
		block.Format = FormatTable
		p.advance()
		return p.parseTableFields(block)
	case tokTASK:
		block.Format = FormatTask
		p.advance()
		return nil
	case tokEOF:
		return fmt.Errorf("empty query block")
	default:
		return fmt.Errorf("query must start with LIST, TABLE, or TASK, got %q", p.cur.val)
	}
}

// parseListFields parses optional WITHOUT ID and a single optional field after LIST.
func (p *queryParser) parseListFields(block *QueryBlock) error {
	if p.cur.typ == tokWITHOUT {
		p.advance()
		if _, err := p.expect(tokID); err != nil {
			return fmt.Errorf("expected ID after WITHOUT")
		}
		block.WithoutID = true
	}

	// LIST allows at most one field (displayed alongside the link).
	if p.cur.typ == tokIDENT {
		field, err := p.parseField()
		if err != nil {
			return err
		}
		if field.Alias != "" {
			return fmt.Errorf("AS aliases are only supported in TABLE format")
		}
		block.Fields = append(block.Fields, field)
	}

	return nil
}

// parseTableFields parses optional WITHOUT ID and a comma-separated field list after TABLE.
func (p *queryParser) parseTableFields(block *QueryBlock) error {
	if p.cur.typ == tokWITHOUT {
		p.advance()
		if _, err := p.expect(tokID); err != nil {
			return fmt.Errorf("expected ID after WITHOUT")
		}
		block.WithoutID = true
	}

	// TABLE fields are optional — if none given, defaults apply in render.
	if p.cur.typ == tokIDENT {
		fields, err := p.parseFieldList()
		if err != nil {
			return err
		}
		block.Fields = fields
	}

	return nil
}

func (p *queryParser) parseFieldList() ([]ShowField, error) {
	var fields []ShowField

	field, err := p.parseField()
	if err != nil {
		return nil, err
	}
	fields = append(fields, field)

	for p.cur.typ == tokCOMMA {
		p.advance() // skip comma
		field, err = p.parseField()
		if err != nil {
			return nil, err
		}
		fields = append(fields, field)
	}

	return fields, nil
}

func (p *queryParser) parseField() (ShowField, error) {
	tok, err := p.expect(tokIDENT)
	if err != nil {
		return ShowField{}, fmt.Errorf("expected field name, got %q", p.cur.val)
	}

	field := ShowField{Name: tok.val}

	if p.cur.typ == tokAS {
		p.advance()
		alias, err := p.expect(tokSTRING)
		if err != nil {
			return ShowField{}, fmt.Errorf("expected quoted alias after AS")
		}
		field.Alias = alias.val
	}

	return field, nil
}

func (p *queryParser) parseClauses(block *QueryBlock) error {
	hasFrom := false
	hasSort := false
	hasLimit := false

	for p.cur.typ != tokEOF {
		switch p.cur.typ {
		case tokFROM:
			if hasFrom {
				return fmt.Errorf("duplicate FROM clause")
			}
			hasFrom = true
			if err := p.parseFrom(block); err != nil {
				return err
			}
		case tokWHERE:
			if err := p.parseWhere(block); err != nil {
				return err
			}
		case tokSORT:
			if hasSort {
				return fmt.Errorf("duplicate SORT clause")
			}
			hasSort = true
			if err := p.parseSort(block); err != nil {
				return err
			}
		case tokLIMIT:
			if hasLimit {
				return fmt.Errorf("duplicate LIMIT clause")
			}
			hasLimit = true
			if err := p.parseLimit(block); err != nil {
				return err
			}
		case tokILLEGAL:
			return fmt.Errorf("unexpected character: %q", p.cur.val)
		default:
			return fmt.Errorf("unexpected token: %q", p.cur.val)
		}
	}
	return nil
}

func (p *queryParser) parseFrom(block *QueryBlock) error {
	p.advance() // skip FROM
	if p.cur.typ != tokIDENT {
		return fmt.Errorf("expected path after FROM, got %q", p.cur.val)
	}
	folder := p.cur.val
	block.Folder = &folder
	p.advance()
	return nil
}

func (p *queryParser) parseWhere(block *QueryBlock) error {
	p.advance() // skip WHERE

	if p.cur.typ != tokIDENT {
		return fmt.Errorf("expected field name after WHERE, got %q", p.cur.val)
	}
	field := p.cur.val
	p.advance()

	var cond Condition
	cond.Field = field

	switch p.cur.typ {
	case tokCONTAIN:
		cond.Op = OpContain
		p.advance()
	case tokEQ:
		cond.Op = OpEqual
		p.advance()
	default:
		return fmt.Errorf("expected operator (CONTAIN or =) after %q, got %q", field, p.cur.val)
	}

	val, err := p.expect(tokSTRING)
	if err != nil {
		return fmt.Errorf("expected quoted value after operator, got %q", p.cur.val)
	}
	cond.Value = val.val

	block.Conditions = append(block.Conditions, cond)
	return nil
}

func (p *queryParser) parseSort(block *QueryBlock) error {
	p.advance() // skip SORT

	if p.cur.typ != tokIDENT {
		return fmt.Errorf("expected field name after SORT, got %q", p.cur.val)
	}
	block.SortField = p.cur.val
	p.advance()

	switch p.cur.typ {
	case tokASC:
		block.SortDesc = false
		p.advance()
	case tokDESC:
		block.SortDesc = true
		p.advance()
	}

	return nil
}

func (p *queryParser) parseLimit(block *QueryBlock) error {
	p.advance() // skip LIMIT

	tok, err := p.expect(tokINT)
	if err != nil {
		return fmt.Errorf("expected number after LIMIT, got %q", p.cur.val)
	}

	n, err := strconv.Atoi(tok.val)
	if err != nil || n <= 0 {
		return fmt.Errorf("invalid LIMIT value: %s", tok.val)
	}
	block.Limit = n
	return nil
}

func tokenName(typ tokenType) string {
	switch typ {
	case tokInvalid:
		return "INVALID"
	case tokLIST:
		return "LIST"
	case tokTABLE:
		return "TABLE"
	case tokTASK:
		return "TASK"
	case tokFROM:
		return "FROM"
	case tokWHERE:
		return "WHERE"
	case tokSORT:
		return "SORT"
	case tokLIMIT:
		return "LIMIT"
	case tokASC:
		return "ASC"
	case tokDESC:
		return "DESC"
	case tokCONTAIN:
		return "CONTAIN"
	case tokWITHOUT:
		return "WITHOUT"
	case tokID:
		return "ID"
	case tokAS:
		return "AS"
	case tokSTRING:
		return "STRING"
	case tokINT:
		return "INT"
	case tokIDENT:
		return "IDENT"
	case tokEQ:
		return "="
	case tokCOMMA:
		return ","
	case tokEOF:
		return "EOF"
	case tokILLEGAL:
		return "ILLEGAL"
	default:
		return "UNKNOWN"
	}
}
