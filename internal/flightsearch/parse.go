package flightsearch

import (
	"fmt"
	"strings"
)

// Query is a parsed, validated search query ready to be compiled to SQL.
type Query struct {
	root node
}

// node is one element of the query AST. Values are validated and converted at
// parse time, so emitting SQL cannot fail.
type node interface {
	sql(b *sqlBuilder) string
}

type andNode struct{ children []node }
type orNode struct{ children []node }
type notNode struct{ child node }

// leafNode holds a pre-compiled condition. fn appends bind values via the
// builder and returns the SQL fragment.
type leafNode struct {
	fn func(b *sqlBuilder) string
}

const (
	maxQueryLength = 1000
	maxQueryTerms  = 50
)

// Parse parses and validates a search query. The returned error message is
// safe to surface to API clients.
func Parse(input string) (*Query, error) {
	if len(input) > maxQueryLength {
		return nil, fmt.Errorf("query is too long (max %d characters)", maxQueryLength)
	}
	p := &parser{input: input}
	expr, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	p.skipSpace()
	if p.pos < len(p.input) {
		return nil, fmt.Errorf("unexpected %q at position %d", rest(p.input, p.pos), p.pos+1)
	}
	if expr == nil {
		return nil, fmt.Errorf("query is empty")
	}
	return &Query{root: expr}, nil
}

func rest(s string, pos int) string {
	r := s[pos:]
	if len(r) > 12 {
		r = r[:12] + "…"
	}
	return r
}

type parser struct {
	input string
	pos   int
	terms int
}

func (p *parser) skipSpace() {
	for p.pos < len(p.input) && (p.input[p.pos] == ' ' || p.input[p.pos] == '\t' || p.input[p.pos] == '\n' || p.input[p.pos] == '\r') {
		p.pos++
	}
}

func (p *parser) peek() byte {
	if p.pos < len(p.input) {
		return p.input[p.pos]
	}
	return 0
}

// parseOr := andExpr (OR andExpr)*
func (p *parser) parseOr() (node, error) {
	first, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	if first == nil {
		return nil, nil
	}
	children := []node{first}
	for {
		save := p.pos
		if !p.consumeKeyword("OR") && !p.consumeSymbol("||") {
			break
		}
		next, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		if next == nil {
			return nil, fmt.Errorf("expected a condition after OR at position %d", save+1)
		}
		children = append(children, next)
	}
	if len(children) == 1 {
		return children[0], nil
	}
	return &orNode{children: children}, nil
}

// parseAnd := unary ((AND)? unary)*  — adjacency is implicit AND.
func (p *parser) parseAnd() (node, error) {
	first, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	if first == nil {
		return nil, nil
	}
	children := []node{first}
	for {
		save := p.pos
		explicit := p.consumeKeyword("AND") || p.consumeSymbol("&&")
		// OR binds looser: stop so parseOr can consume it.
		if !explicit && p.peekKeyword("OR") {
			break
		}
		next, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		if next == nil {
			if explicit {
				return nil, fmt.Errorf("expected a condition after AND at position %d", save+1)
			}
			break
		}
		children = append(children, next)
	}
	if len(children) == 1 {
		return children[0], nil
	}
	return &andNode{children: children}, nil
}

// parseUnary := (NOT | '-') unary | '(' orExpr ')' | condition
func (p *parser) parseUnary() (node, error) {
	p.skipSpace()
	if p.consumeKeyword("NOT") || p.consumeSymbol("-") {
		child, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		if child == nil {
			return nil, fmt.Errorf("expected a condition after NOT at position %d", p.pos+1)
		}
		return &notNode{child: child}, nil
	}
	if p.peek() == '(' {
		open := p.pos
		p.pos++
		inner, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		p.skipSpace()
		if p.peek() != ')' {
			return nil, fmt.Errorf("missing closing parenthesis for group opened at position %d", open+1)
		}
		p.pos++
		if inner == nil {
			return nil, fmt.Errorf("empty parentheses at position %d", open+1)
		}
		return inner, nil
	}
	if p.peek() == ')' || p.pos >= len(p.input) {
		return nil, nil
	}
	return p.parseCondition()
}

// operator characters that terminate a bare word
const opChars = ":=<>!"

// parseCondition reads either `field OP value` or a bare free-text term.
func (p *parser) parseCondition() (node, error) {
	p.terms++
	if p.terms > maxQueryTerms {
		return nil, fmt.Errorf("query has too many terms (max %d)", maxQueryTerms)
	}

	start := p.pos
	if p.peek() == '"' || p.peek() == '\'' {
		// Quoted bare term
		val, err := p.readQuoted()
		if err != nil {
			return nil, err
		}
		return newFreeTextLeaf(val)
	}

	word := p.readWord()
	if word == "" {
		return nil, fmt.Errorf("unexpected %q at position %d", rest(p.input, p.pos), p.pos+1)
	}

	op := p.readOperator()
	if op == "" {
		// Bare free-text term
		return newFreeTextLeaf(word)
	}

	field, ok := LookupField(word)
	if !ok {
		return nil, fmt.Errorf("unknown field %q at position %d", word, start+1)
	}

	value, err := p.readValue()
	if err != nil {
		return nil, err
	}
	if value == "" {
		return nil, fmt.Errorf("missing value for %q at position %d", word+op, start+1)
	}

	return newFieldLeaf(field, op, value)
}

// readWord reads characters up to whitespace, parentheses, or an operator.
func (p *parser) readWord() string {
	start := p.pos
	for p.pos < len(p.input) {
		c := p.input[p.pos]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '(' || c == ')' || strings.IndexByte(opChars, c) >= 0 {
			break
		}
		p.pos++
	}
	return p.input[start:p.pos]
}

// readOperator reads the comparison operator directly following a field name.
func (p *parser) readOperator() string {
	two := ""
	if p.pos+1 < len(p.input) {
		two = p.input[p.pos : p.pos+2]
	}
	switch two {
	case ">=", "<=", "!=":
		p.pos += 2
		return two
	}
	switch p.peek() {
	case ':', '=', '>', '<':
		op := string(p.peek())
		p.pos++
		return op
	}
	return ""
}

// readValue reads a (possibly quoted) value after an operator. Unquoted
// values run to whitespace or a parenthesis, so `date:2024-05` and
// `totalTime>1:30` keep their '-' and ':' characters.
func (p *parser) readValue() (string, error) {
	if p.peek() == '"' || p.peek() == '\'' {
		return p.readQuoted()
	}
	start := p.pos
	for p.pos < len(p.input) {
		c := p.input[p.pos]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '(' || c == ')' {
			break
		}
		p.pos++
	}
	return p.input[start:p.pos], nil
}

func (p *parser) readQuoted() (string, error) {
	quote := p.input[p.pos]
	start := p.pos
	p.pos++
	var sb strings.Builder
	for p.pos < len(p.input) {
		c := p.input[p.pos]
		if c == '\\' && p.pos+1 < len(p.input) {
			sb.WriteByte(p.input[p.pos+1])
			p.pos += 2
			continue
		}
		if c == quote {
			p.pos++
			return sb.String(), nil
		}
		sb.WriteByte(c)
		p.pos++
	}
	return "", fmt.Errorf("unterminated quote at position %d", start+1)
}

// consumeKeyword consumes a case-insensitive keyword if it appears as a
// standalone word (followed by whitespace, a parenthesis, or end of input).
func (p *parser) consumeKeyword(kw string) bool {
	p.skipSpace()
	if p.matchKeywordAt(p.pos, kw) {
		p.pos += len(kw)
		return true
	}
	return false
}

func (p *parser) peekKeyword(kw string) bool {
	save := p.pos
	p.skipSpace()
	ok := p.matchKeywordAt(p.pos, kw)
	p.pos = save
	return ok
}

func (p *parser) matchKeywordAt(pos int, kw string) bool {
	end := pos + len(kw)
	if end > len(p.input) || !strings.EqualFold(p.input[pos:end], kw) {
		return false
	}
	if end == len(p.input) {
		return true
	}
	c := p.input[end]
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '(' || c == ')'
}

// consumeSymbol consumes an exact symbol token ("&&", "||", "-").
func (p *parser) consumeSymbol(sym string) bool {
	p.skipSpace()
	end := p.pos + len(sym)
	if end > len(p.input) || p.input[p.pos:end] != sym {
		return false
	}
	// '-' only negates when attached to a following term (e.g. -remarks:x),
	// avoiding surprises for stray dashes.
	if sym == "-" && (end >= len(p.input) || p.input[end] == ' ') {
		return false
	}
	p.pos = end
	return true
}
