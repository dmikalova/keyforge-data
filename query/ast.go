// Package query is the search language that search.keyforge.cards and
// decks.keyforge.cards share (ADR 0004).
//
// Parse turns a query string into an AST, with a Schema that names the
// fields. The AST does not depend on a backend: Registry evaluates it in
// memory, and a SQL compiler can read the same AST.
//
// The grammar, with the lowest precedence first:
//
//	query   = or
//	or      = and { "or" and }
//	and     = unary { ["and"] unary }
//	unary   = ("not" | "-") unary | primary
//	primary = "(" or ")" | term
//	term    = field op value { "," value } | "#" value | word | "quoted words"
//	op      = ":" | "=" | "!=" | "<" | "<=" | ">" | ">="
//
// A comma list of values is an OR of terms: "set:aoa,wc" is
// "(set:aoa or set:wc)". Enum and flag fields take a list with ":" or "=",
// and a keyed field takes a list of keys with ":". A comma inside quotes is
// part of the value.
//
// A directive term (sort:, order:, unique:, seed:, fuzzy:) changes the
// display or the name matching, and does not filter.| "#" value | word | "quoted words"
//
//	op      = ":" | "=" | "!=" | "<" | "<=" | ">" | ">="
//
// A directive term (sort:, order:, unique:, seed:) changes the display and
// does not filter. Directives are allowed only at the top level, joined by
// AND.
package query

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// Span is a byte range in the query string.
type Span struct {
	Start, End int
}

// Node is one node of the AST.
type Node interface {
	// Span is the part of the query that the node comes from.
	Span() Span
	node()
}

// And matches when every child matches.
type And struct {
	Children []Node
	Pos      Span
}

// Or matches when any child matches.
type Or struct {
	Children []Node
	Pos      Span
	// List is true for a comma list such as "set:aoa,wc". Each child is a
	// Term of the same field and operator, and String writes the list form.
	List bool
}

// Not matches when its child does not match.
type Not struct {
	Child Node
	Pos   Span
}

// Op is a comparison operator.
type Op string

// The operators. OpHas is ":", which means "contains" for text and "equals"
// for numbers and enums.
const (
	OpHas Op = ":"
	OpEq  Op = "="
	OpNe  Op = "!="
	OpLt  Op = "<"
	OpLe  Op = "<="
	OpGt  Op = ">"
	OpGe  Op = ">="
)

// Term is one condition on a field.
type Term struct {
	// Field is the field's full name, not its alias.
	Field string
	// Key is the key of a keyed field, for example "assault" in
	// "keyword:assault>=2". It is empty for other fields.
	Key string
	// Op is the operator.
	Op Op
	// Value is the value.
	Value Value
	Pos   Span
}

// Word is a bare word or a quoted phrase without a field.
type Word struct {
	Text   string
	Quoted bool
	Pos    Span
}

// Value is the value of a term.
type Value struct {
	// Text is the value as written, without quotes.
	Text string
	// Quoted is true when the value was in quotes.
	Quoted bool
	// Num is the number, for a number value.
	Num int
	// X is true for the value X of a number field.
	X bool
	// Range is true for a range "lo..hi". Num is lo and Hi is hi.
	Range bool
	Hi    int
}

// Directive is a term that changes the display.
type Directive struct {
	Name  string
	Value string
	Pos   Span
}

// Query is a parsed query.
type Query struct {
	// Root is the filter. It is nil for a query with no filter terms.
	Root Node
	// Directives are the display directives, in order.
	Directives []Directive
	// Source is the query string.
	Source string
}

// Directive returns the value of the last directive with the name.
func (q *Query) Directive(name string) (string, bool) {
	for _, v := range slices.Backward(q.Directives) {
		if v.Name == name {
			return v.Value, true
		}
	}
	return "", false
}

// Span returns the part of the query that the AND comes from.
func (n *And) Span() Span { return n.Pos }

// Span returns the part of the query that the OR comes from.
func (n *Or) Span() Span { return n.Pos }

// Span returns the part of the query that the NOT comes from.
func (n *Not) Span() Span { return n.Pos }

// Span returns the part of the query that the term comes from.
func (n *Term) Span() Span { return n.Pos }

// Span returns the part of the query that the word comes from.
func (n *Word) Span() Span { return n.Pos }

func (*And) node()  {}
func (*Or) node()   {}
func (*Not) node()  {}
func (*Term) node() {}
func (*Word) node() {}

// String formats the query in canonical form: full field names, lowercase
// operators, and parentheses only where precedence needs them. Parse of the
// result gives the same AST.
func (q *Query) String() string {
	var parts []string
	if q.Root != nil {
		parts = append(parts, format(q.Root, precOr))
	}
	for _, d := range q.Directives {
		parts = append(parts, d.Name+":"+quoteIfNeeded(d.Value))
	}
	return strings.Join(parts, " ")
}

const (
	precOr = iota
	precAnd
	precUnary
)

func format(n Node, outer int) string {
	switch n := n.(type) {
	case *Or:
		if n.List {
			if s, ok := formatList(n.Children); ok {
				return s
			}
		}
		s := joinNodes(n.Children, " or ", precOr)
		if outer > precOr {
			return "(" + s + ")"
		}
		return s
	case *And:
		s := joinNodes(n.Children, " ", precAnd)
		if outer > precAnd {
			return "(" + s + ")"
		}
		return s
	case *Not:
		return "-" + format(n.Child, precUnary)
	case *Term:
		return formatTerm(n)
	case *Word:
		if n.Quoted {
			return quote(n.Text)
		}
		return n.Text
	default:
		panic(fmt.Sprintf("query: unknown node %T", n))
	}
}

func joinNodes(nodes []Node, sep string, prec int) string {
	parts := make([]string, len(nodes))
	for i, c := range nodes {
		parts[i] = format(c, prec+1)
	}
	return strings.Join(parts, sep)
}

func formatTerm(t *Term) string {
	var b strings.Builder
	b.WriteString(t.Field)
	if t.Key != "" {
		b.WriteString(":" + quoteKey(t.Key))
		if t.Value.Text == "" {
			return b.String() // the key alone
		}
	}
	b.WriteString(string(t.Op))
	b.WriteString(formatValue(t.Value))
	return b.String()
}

// formatList writes a comma list: the first term in full, then the values
// (or the keys of a keyed field) of the other terms. It returns false if a
// child is not a term, so the caller writes a plain OR.
func formatList(children []Node) (string, bool) {
	parts := make([]string, len(children))
	for i, c := range children {
		t, ok := c.(*Term)
		if !ok {
			return "", false
		}
		switch {
		case i == 0:
			parts[i] = formatTerm(t)
		case t.Key != "":
			parts[i] = quoteKey(t.Key)
		default:
			parts[i] = formatValue(t.Value)
		}
	}
	return strings.Join(parts, ","), true
}

func formatValue(v Value) string {
	switch {
	case v.Range:
		return strconv.Itoa(v.Num) + ".." + strconv.Itoa(v.Hi)
	case v.Quoted:
		return quote(v.Text)
	default:
		return quoteIfNeeded(v.Text)
	}
}

func quoteIfNeeded(s string) string {
	if s == "" || strings.ContainsAny(s, " \t\"()") {
		return quote(s)
	}
	return s
}

// quoteKey quotes a key that needs quotes, or that has a comma: an unquoted
// comma starts a list.
func quoteKey(s string) string {
	if strings.Contains(s, ",") {
		return quote(s)
	}
	return quoteIfNeeded(s)
}

// quote wraps s in quote marks. The language has no escapes: a word cannot
// hold a quote mark, so none is needed.
func quote(s string) string {
	return `"` + s + `"`
}
