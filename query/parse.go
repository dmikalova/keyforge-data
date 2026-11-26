package query

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// Error is a parse error at a place in the query.
type Error struct {
	Pos Span
	Msg string
	// Suggest is a "did you mean" value. It can be empty.
	Suggest string
}

func (e *Error) Error() string {
	s := fmt.Sprintf("%s (at %d)", e.Msg, e.Pos.Start)
	if e.Suggest != "" {
		s += fmt.Sprintf("; did you mean %q?", e.Suggest)
	}
	return s
}

// NumberField is the field that "#" names.
const NumberField = "number"

// FlagField is the name of the flag field.
const FlagField = "is"

// Parse parses a query with the fields of the schema.
func Parse(src string, schema Schema) (*Query, error) {
	p := &parser{toks: lex(src), schema: schema, src: src}
	q := &Query{Source: src}
	if p.peek().kind == tokEOF {
		return q, nil
	}
	root, err := p.or(0)
	if err != nil {
		return nil, err
	}
	if t := p.peek(); t.kind != tokEOF {
		return nil, &Error{Pos: t.pos, Msg: fmt.Sprintf("unexpected %q", t.text)}
	}
	root, q.Directives, err = splitDirectives(root)
	if err != nil {
		return nil, err
	}
	q.Root = root
	return q, nil
}

type parser struct {
	toks   []token
	i      int
	schema Schema
	src    string
}

func (p *parser) peek() token { return p.toks[p.i] }

func (p *parser) next() token {
	t := p.toks[p.i]
	if t.kind != tokEOF {
		p.i++
	}
	return t
}

func (p *parser) or(depth int) (Node, error) {
	first, err := p.and(depth)
	if err != nil {
		return nil, err
	}
	children := []Node{first}
	for p.peek().kind == tokOr {
		p.next()
		n, err := p.and(depth)
		if err != nil {
			return nil, err
		}
		children = append(children, n)
	}
	if len(children) == 1 {
		return first, nil
	}
	return &Or{Children: flattenOr(children), Pos: spanOf(children)}, nil
}

func (p *parser) and(depth int) (Node, error) {
	var children []Node
	for {
		t := p.peek()
		switch t.kind {
		case tokEOF, tokOr, tokRParen:
			if len(children) == 0 {
				return nil, &Error{Pos: t.pos, Msg: "expected a term"}
			}
			if len(children) == 1 {
				return children[0], nil
			}
			return &And{Children: flattenAnd(children), Pos: spanOf(children)}, nil
		case tokAnd:
			p.next()
			if len(children) == 0 {
				return nil, &Error{Pos: t.pos, Msg: `"and" needs a term before it`}
			}
			continue
		}
		n, err := p.unary(depth)
		if err != nil {
			return nil, err
		}
		children = append(children, n)
	}
}

func (p *parser) unary(depth int) (Node, error) {
	t := p.peek()
	if t.kind == tokNot {
		p.next()
		child, err := p.unary(depth)
		if err != nil {
			return nil, err
		}
		return &Not{Child: child, Pos: Span{t.pos.Start, child.Span().End}}, nil
	}
	return p.primary(depth)
}

// maxDepth limits nested parentheses.
const maxDepth = 64

func (p *parser) primary(depth int) (Node, error) {
	t := p.next()
	switch t.kind {
	case tokLParen:
		if depth >= maxDepth {
			return nil, &Error{Pos: t.pos, Msg: "too many nested parentheses"}
		}
		n, err := p.or(depth + 1)
		if err != nil {
			return nil, err
		}
		if c := p.next(); c.kind != tokRParen {
			return nil, &Error{Pos: t.pos, Msg: `"(" has no matching ")"`}
		}
		return n, nil
	case tokWord:
		return p.word(t)
	case tokError:
		return nil, &Error{Pos: t.pos, Msg: "a quote has no closing quote"}
	case tokRParen:
		return nil, &Error{Pos: t.pos, Msg: `")" has no matching "("`}
	default:
		return nil, &Error{Pos: t.pos, Msg: "expected a term"}
	}
}

// word parses a word token into a term or a bare word.
func (p *parser) word(t token) (Node, error) {
	if strings.HasPrefix(t.text, "#") && (len(t.quoted) == 0 || !t.quoted[0]) {
		return p.term(t, NumberField, 1)
	}
	end := 0
	for end < len(t.text) && isNameByte(t.text[end]) && !t.quoted[end] {
		end++
	}
	if end == 0 || end == len(t.text) || t.quoted[end] || !isOpByte(t.text[end]) {
		return &Word{Text: t.text, Quoted: t.anyQuoted, Pos: t.pos}, nil
	}
	return p.term(t, t.text[:end], end)
}

// term parses `field op value`, where the op starts at byte at of the token.
func (p *parser) term(t token, name string, at int) (Node, error) {
	f, ok := p.lookup(name)
	if !ok {
		return nil, &Error{
			Pos:     Span{t.pos.Start, t.pos.Start + len(name)},
			Msg:     fmt.Sprintf("unknown field %q", name),
			Suggest: closest(name, p.fieldNames()),
		}
	}
	op, n := readOp(t.text[at:])
	if f.Kind == KindNumber && f.Name == NumberField && at == 1 && n == 0 {
		op = OpHas // "#001"
	} else if n == 0 {
		return nil, &Error{Pos: t.pos, Msg: "expected an operator after " + name}
	}
	valStart := at + n
	if listable(f, op) {
		if parts := splitList(t, valStart); len(parts) > 1 {
			return list(t, f, op, parts)
		}
	}
	text := t.text[valStart:]
	quoted := slices.Contains(t.quoted[valStart:], true) || (t.anyQuoted && text == "")
	term, err := value(t, f, op, text, quoted)
	if err != nil {
		return nil, err
	}
	if f.Kind == KindDirective {
		return &directiveNode{Directive{Name: f.Name, Value: term.Value.Text, Pos: t.pos}}, nil
	}
	return term, nil
}

// value parses one value of a term.
func value(t token, f FieldInfo, op Op, text string, quoted bool) (*Term, error) {
	term := &Term{Field: f.Name, Op: op, Pos: t.pos}
	switch f.Kind {
	case KindText, KindName:
		if op != OpHas && op != OpEq && op != OpNe {
			return nil, opError(t, f, op)
		}
		term.Value = Value{Text: text, Quoted: quoted}
	case KindEnum, KindFlag, KindDirective:
		if op != OpHas && op != OpEq && op != OpNe {
			return nil, opError(t, f, op)
		}
		v, err := enumValue(t, f, text)
		if err != nil {
			return nil, err
		}
		term.Value = Value{Text: v, Quoted: quoted}
	case KindNumber:
		v, err := numberValue(t, f, op, text)
		if err != nil {
			return nil, err
		}
		term.Value = v
	case KindKeyed:
		if err := keyedValue(t, f, op, text, term); err != nil {
			return nil, err
		}
	}
	return term, nil
}

// listable reports whether a term of the field can have a comma list of
// values: an enum or a flag with ":" or "=", or the keys of a keyed field
// with ":".
func listable(f FieldInfo, op Op) bool {
	switch f.Kind {
	case KindEnum, KindFlag:
		return op == OpHas || op == OpEq
	case KindKeyed:
		return op == OpHas
	default:
		return false
	}
}

// listPart is one value of a comma list.
type listPart struct {
	text   string
	quoted bool
}

// splitList splits the value of a term, from byte start of the token, at the
// commas outside quotes.
func splitList(t token, start int) []listPart {
	var out []listPart
	var cur listPart
	for i := start; i < len(t.text); i++ {
		if t.text[i] == ',' && !t.quoted[i] {
			out = append(out, cur)
			cur = listPart{}
			continue
		}
		cur.text += string(t.text[i])
		cur.quoted = cur.quoted || t.quoted[i]
	}
	return append(out, cur)
}

// list parses a comma list into an OR of terms: "set:aoa,wc" matches a card
// in AoA or in WC. A keyed field takes only keys in a list.
func list(t token, f FieldInfo, op Op, parts []listPart) (Node, error) {
	or := &Or{Pos: t.pos, List: true}
	for _, part := range parts {
		if part.text == "" && !part.quoted {
			return nil, &Error{
				Pos: t.pos,
				Msg: "a list of " + f.Name + " values has an empty value",
			}
		}
		term, err := value(t, f, op, part.text, part.quoted)
		if err != nil {
			return nil, err
		}
		if f.Kind == KindKeyed && (term.Op != op || term.Value.Text != "") {
			return nil, &Error{Pos: t.pos, Msg: "a list of " + f.Name + " values takes only keys"}
		}
		or.Children = append(or.Children, term)
	}
	return or, nil
}

func (p *parser) lookup(name string) (FieldInfo, bool) {
	return p.schema.Field(strings.ToLower(name))
}

func (p *parser) fieldNames() []string {
	var out []string
	for _, f := range p.schema.Fields() {
		out = append(out, f.Name)
		if f.Alias != "" {
			out = append(out, f.Alias)
		}
	}
	return out
}

func opError(t token, f FieldInfo, op Op) error {
	return &Error{Pos: t.pos, Msg: fmt.Sprintf("%s does not take %q", f.Name, op)}
}

func enumValue(t token, f FieldInfo, text string) (string, error) {
	if text == "" {
		return "", &Error{Pos: t.pos, Msg: "expected a value for " + f.Name}
	}
	if len(f.Values) == 0 {
		return text, nil
	}
	want := Normalize(text)
	var prefixed []string
	// Two spellings of one value, for example a set slug and a set name,
	// are one match.
	seen := map[string]bool{}
	for _, v := range f.Values {
		n := Normalize(v)
		if n == want {
			return v, nil
		}
		if want != "" && strings.HasPrefix(n, want) && !seen[n] {
			seen[n] = true
			prefixed = append(prefixed, v)
		}
	}
	// A prefix of one value is enough: "h:ek" is house:Ekwidon.
	switch len(prefixed) {
	case 1:
		return prefixed[0], nil
	case 0:
	default:
		return "", &Error{
			Pos: t.pos,
			Msg: fmt.Sprintf("%q can be more than one %s value: %s",
				text, f.Name, strings.Join(prefixed, ", ")),
		}
	}
	return "", &Error{
		Pos:     t.pos,
		Msg:     fmt.Sprintf("%q is not a %s value", text, f.Name),
		Suggest: closest(text, f.Values),
	}
}

func numberValue(t token, f FieldInfo, op Op, text string) (Value, error) {
	if text == "" {
		return Value{}, &Error{Pos: t.pos, Msg: "expected a number for " + f.Name}
	}
	if strings.EqualFold(text, "x") {
		if op != OpHas && op != OpEq && op != OpNe {
			return Value{}, opError(t, f, op)
		}
		return Value{Text: text, X: true}, nil
	}
	if lo, hi, ok := strings.Cut(text, ".."); ok {
		a, errA := strconv.Atoi(lo)
		b, errB := strconv.Atoi(hi)
		if errA != nil || errB != nil || op != OpHas {
			return Value{}, &Error{
				Pos: t.pos,
				Msg: fmt.Sprintf("%q is not a range like 2..4", text),
			}
		}
		return Value{Text: text, Num: a, Hi: b, Range: true}, nil
	}
	n, err := strconv.Atoi(text)
	if err != nil {
		if f.Name == NumberField && (op == OpHas || op == OpEq || op == OpNe) {
			return Value{Text: text}, nil // a lettered number, for example S01
		}
		return Value{}, &Error{Pos: t.pos, Msg: fmt.Sprintf("%q is not a number", text)}
	}
	return Value{Text: text, Num: n}, nil
}

func keyedValue(t token, f FieldInfo, op Op, text string, term *Term) error {
	if op != OpHas {
		// "enhance>=2": a total, with no key.
		v, err := numberValue(t, f, op, text)
		term.Value = v
		return err
	}
	end := 0
	for end < len(text) && !isOpByte(text[end]) {
		end++
	}
	key := text[:end]
	if key == "" {
		return &Error{Pos: t.pos, Msg: "expected a value for " + f.Name}
	}
	if len(f.Values) > 0 {
		k, err := enumValue(t, f, key)
		if err != nil {
			return err
		}
		key = k
	}
	term.Key = key
	rest := text[end:]
	if rest == "" {
		// A key alone: the item has the key.
		return nil
	}
	op2, n := readOp(rest)
	if n == 0 {
		return &Error{Pos: t.pos, Msg: "expected an operator after " + key}
	}
	term.Op = op2
	v, err := numberValue(t, f, op2, rest[n:])
	term.Value = v
	return err
}

// readOp reads an operator at the start of s and returns its length.
func readOp(s string) (Op, int) {
	for _, op := range []Op{OpNe, OpLe, OpGe, OpHas, OpEq, OpLt, OpGt} {
		if strings.HasPrefix(s, string(op)) {
			return op, len(op)
		}
	}
	return "", 0
}

func isNameByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_'
}

func isOpByte(c byte) bool {
	return c == ':' || c == '=' || c == '!' || c == '<' || c == '>'
}

// directiveNode carries a directive from the parser to splitDirectives.
type directiveNode struct{ d Directive }

func (n *directiveNode) Span() Span { return n.d.Pos }
func (*directiveNode) node()        {}

// splitDirectives takes the directives out of the top-level AND. A directive
// anywhere else is an error.
func splitDirectives(root Node) (Node, []Directive, error) {
	var dirs []Directive
	if d, ok := root.(*directiveNode); ok {
		return nil, []Directive{d.d}, nil
	}
	if and, ok := root.(*And); ok {
		var keep []Node
		for _, c := range and.Children {
			if d, ok := c.(*directiveNode); ok {
				dirs = append(dirs, d.d)
				continue
			}
			keep = append(keep, c)
		}
		switch len(keep) {
		case 0:
			root = nil
		case 1:
			root = keep[0]
		default:
			root = &And{Children: keep, Pos: spanOf(keep)}
		}
	}
	if root != nil {
		if d := findDirective(root); d != nil {
			return nil, nil, &Error{
				Pos: d.d.Pos,
				Msg: d.d.Name + ": cannot be inside or, not, or parentheses",
			}
		}
	}
	return root, dirs, nil
}

func findDirective(n Node) *directiveNode {
	switch n := n.(type) {
	case *directiveNode:
		return n
	case *And:
		for _, c := range n.Children {
			if d := findDirective(c); d != nil {
				return d
			}
		}
	case *Or:
		for _, c := range n.Children {
			if d := findDirective(c); d != nil {
				return d
			}
		}
	case *Not:
		return findDirective(n.Child)
	}
	return nil
}

// flattenAnd merges nested ANDs: (a b) c becomes a b c.
func flattenAnd(children []Node) []Node {
	var out []Node
	for _, c := range children {
		if n, ok := c.(*And); ok {
			out = append(out, n.Children...)
			continue
		}
		out = append(out, c)
	}
	return out
}

// flattenOr merges nested ORs: (a or b) or c becomes a or b or c.
func flattenOr(children []Node) []Node {
	var out []Node
	for _, c := range children {
		if n, ok := c.(*Or); ok && !n.List {
			out = append(out, n.Children...)
			continue
		}
		out = append(out, c)
	}
	return out
}

func spanOf(nodes []Node) Span {
	return Span{nodes[0].Span().Start, nodes[len(nodes)-1].Span().End}
}

// closest returns the candidate nearest to s by edit distance, if it is near
// enough to be a likely typo.
func closest(s string, candidates []string) string {
	s = strings.ToLower(s)
	best, bestD := "", 1<<30
	for _, c := range candidates {
		d := distance(s, strings.ToLower(c))
		if d < bestD {
			best, bestD = c, d
		}
	}
	if bestD > max(2, len(s)/3) {
		return ""
	}
	return best
}

// distance is the Levenshtein edit distance.
func distance(a, b string) int {
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}
