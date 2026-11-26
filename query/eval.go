package query

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// Num is one number value of an item. A card's power can be X.
type Num struct {
	N int
	// X is true for the value X.
	X bool
	// S is the number as printed, for a value that is not only digits, for
	// example the card number "S01".
	S string
}

// Field is a field of items of type T, with its accessor. Set the accessor
// that matches the kind:
//
//   - KindText, KindFuzzy, KindEnum: Strings.
//   - KindNumber: Numbers.
//   - KindKeyed: Keyed.
type Field[T any] struct {
	FieldInfo
	Strings func(T) []string
	Numbers func(T) []Num
	Keyed   func(T) map[string]int
}

// Flag is one value of the "is:" field.
type Flag[T any] struct {
	Name string
	Test func(T) bool
}

// Registry is a Schema with accessors, so it can evaluate a query on items
// in memory.
type Registry[T any] struct {
	fields  []FieldInfo
	byName  map[string]*Field[T]
	flags   map[string]func(T) bool
	bare    []*Field[T]
	reserve map[string]bool
}

// NewRegistry returns a registry of the fields and flags. The "is" field is
// added when flags are given. A name or alias used twice is an error.
func NewRegistry[T any](fields []Field[T], flags []Flag[T]) (*Registry[T], error) {
	r := &Registry[T]{
		byName:  map[string]*Field[T]{},
		flags:   map[string]func(T) bool{},
		reserve: map[string]bool{},
	}
	if len(flags) > 0 {
		names := make([]string, len(flags))
		for i, f := range flags {
			names[i] = f.Name
			r.flags[Normalize(f.Name)] = f.Test
		}
		fields = append(fields, Field[T]{
			Name: FlagField, Kind: KindFlag, Values: names})
	}
	for i := range fields {
		f := &fields[i]
		if err := checkAccessor(f); err != nil {
			return nil, err
		}
		for _, n := range []string{f.Name, f.Alias} {
			if n == "" {
				continue
			}
			n = strings.ToLower(n)
			if r.reserve[n] {
				return nil, fmt.Errorf("query: the name %q is used twice", n)
			}
			r.reserve[n] = true
			r.byName[n] = f
		}
		r.fields = append(r.fields, f.FieldInfo)
		if f.Bare {
			r.bare = append(r.bare, f)
		}
	}
	return r, nil
}

func checkAccessor[T any](f *Field[T]) error {
	ok := true
	switch f.Kind {
	case KindText, KindFuzzy, KindEnum:
		ok = f.Strings != nil
	case KindNumber:
		ok = f.Numbers != nil
	case KindKeyed:
		ok = f.Keyed != nil
	case KindFlag, KindDirective:
	}
	if !ok {
		return fmt.Errorf("query: field %q has no accessor for its kind", f.Name)
	}
	if f.Bare && f.Strings == nil {
		return fmt.Errorf("query: bare field %q needs Strings", f.Name)
	}
	return nil
}

// Field returns the field with the name or alias.
func (r *Registry[T]) Field(name string) (FieldInfo, bool) {
	if name == "#" {
		name = NumberField
	}
	f, ok := r.byName[strings.ToLower(name)]
	if !ok {
		return FieldInfo{}, false
	}
	return f.FieldInfo, true
}

// Fields returns every field.
func (r *Registry[T]) Fields() []FieldInfo {
	return slices.Clone(r.fields)
}

// Parse parses a query with the registry's fields.
func (r *Registry[T]) Parse(src string) (*Query, error) {
	return Parse(src, r)
}

// Match returns a function that reports whether an item matches the query.
// A query with no filter matches every item.
func (r *Registry[T]) Match(q *Query) func(T) bool {
	if q.Root == nil {
		return func(T) bool { return true }
	}
	return r.compile(q.Root)
}

func (r *Registry[T]) compile(n Node) func(T) bool {
	switch n := n.(type) {
	case *And:
		fs := r.compileAll(n.Children)
		return func(item T) bool {
			for _, f := range fs {
				if !f(item) {
					return false
				}
			}
			return true
		}
	case *Or:
		fs := r.compileAll(n.Children)
		return func(item T) bool {
			for _, f := range fs {
				if f(item) {
					return true
				}
			}
			return false
		}
	case *Not:
		f := r.compile(n.Child)
		return func(item T) bool { return !f(item) }
	case *Word:
		return r.word(n)
	case *Term:
		return r.term(n)
	default:
		panic(fmt.Sprintf("query: unknown node %T", n))
	}
}

func (r *Registry[T]) compileAll(nodes []Node) []func(T) bool {
	out := make([]func(T) bool, len(nodes))
	for i, c := range nodes {
		out[i] = r.compile(c)
	}
	return out
}

// word matches a bare word as a substring of any bare field.
func (r *Registry[T]) word(w *Word) func(T) bool {
	want := Fold(w.Text)
	return func(item T) bool {
		for _, f := range r.bare {
			for _, s := range f.Strings(item) {
				if strings.Contains(Fold(s), want) {
					return true
				}
			}
		}
		return false
	}
}

func (r *Registry[T]) term(t *Term) func(T) bool {
	if t.Field == FlagField {
		test := r.flags[Normalize(t.Value.Text)]
		if t.Op == OpNe {
			return func(item T) bool { return !test(item) }
		}
		return test
	}
	f := r.byName[t.Field]
	switch f.Kind {
	case KindText:
		return textTerm(f.Strings, t, Fold, false)
	case KindEnum:
		return textTerm(f.Strings, t, Normalize, true)
	case KindFuzzy:
		if t.Op != OpHas || t.Value.Quoted {
			return textTerm(f.Strings, t, Fold, false)
		}
		return func(item T) bool {
			for _, s := range f.Strings(item) {
				if _, ok := FuzzyScore(t.Value.Text, s); ok {
					return true
				}
			}
			return false
		}
	case KindNumber:
		return func(item T) bool { return numberTerm(f.Numbers(item), t) }
	case KindKeyed:
		return func(item T) bool { return keyedTerm(f.Keyed(item), t) }
	default:
		panic(fmt.Sprintf("query: field %q of kind %d in a term", t.Field, f.Kind))
	}
}

// textTerm matches strings in the form that norm makes. For text, ":" and
// "!=" test a substring, and "=" tests the whole string. For an enum
// (enum set), every operator tests the whole value.
func textTerm[T any](
	get func(T) []string,
	t *Term,
	norm func(string) string,
	enum bool,
) func(T) bool {
	want := norm(t.Value.Text)
	exact := enum || t.Op == OpEq
	return func(item T) bool {
		found := false
		for _, s := range get(item) {
			s = norm(s)
			if exact && s == want || !exact && strings.Contains(s, want) {
				found = true
				break
			}
		}
		if t.Op == OpNe {
			return !found
		}
		return found
	}
}

func numberTerm(nums []Num, t *Term) bool {
	v := t.Value
	hasNum := func(test func(Num) bool) bool {
		return slices.ContainsFunc(nums, test)
	}
	switch {
	case v.X:
		has := hasNum(func(n Num) bool { return n.X })
		return has != (t.Op == OpNe)
	case v.Range:
		return hasNum(func(n Num) bool { return numeric(n) && n.N >= v.Num && n.N <= v.Hi })
	case !isDigits(v.Text):
		// A lettered number such as S01.
		has := hasNum(func(n Num) bool { return Normalize(n.S) == Normalize(v.Text) })
		return has != (t.Op == OpNe)
	case t.Op == OpNe:
		return !hasNum(func(n Num) bool { return numeric(n) && n.N == v.Num })
	default:
		return hasNum(func(n Num) bool { return numeric(n) && compare(n.N, t.Op, v.Num) })
	}
}

func keyedTerm(m map[string]int, t *Term) bool {
	if t.Key == "" {
		total := 0
		for _, n := range m {
			total += n
		}
		return numberTerm([]Num{{N: total}}, t)
	}
	n, ok := lookupKey(m, t.Key)
	if !ok {
		return false
	}
	if t.Value.Text == "" {
		return true // the key alone
	}
	return numberTerm([]Num{{N: n}}, t)
}

func lookupKey(m map[string]int, key string) (int, bool) {
	if n, ok := m[key]; ok {
		return n, true
	}
	want := Normalize(key)
	for k, n := range m {
		if Normalize(k) == want {
			return n, true
		}
	}
	return 0, false
}

func compare(a int, op Op, b int) bool {
	switch op {
	case OpHas, OpEq:
		return a == b
	case OpNe:
		return a != b
	case OpLt:
		return a < b
	case OpLe:
		return a <= b
	case OpGt:
		return a > b
	case OpGe:
		return a >= b
	}
	return false
}

func isDigits(s string) bool {
	_, err := strconv.Atoi(s)
	return err == nil
}

// FuzzyRank returns the total fuzzy score of the query's fuzzy terms for an
// item, and whether the query has only fuzzy terms (joined by AND, not
// negated) and words. A caller ranks by the score when the second result is
// true and no sort directive is set.
func (r *Registry[T]) FuzzyRank(q *Query, item T) (int, bool) {
	terms, only := fuzzyTerms(q.Root, r)
	if !only || len(terms) == 0 {
		return 0, false
	}
	total := 0
	for _, t := range terms {
		best := 0
		for _, s := range r.byName[t.Field].Strings(item) {
			if sc, ok := FuzzyScore(t.Value.Text, s); ok && sc > best {
				best = sc
			}
		}
		total += best
	}
	return total, true
}

func fuzzyTerms[T any](n Node, r *Registry[T]) ([]*Term, bool) {
	switch n := n.(type) {
	case nil:
		return nil, true
	case *Term:
		if f := r.byName[n.Field]; f.Kind == KindFuzzy && n.Op == OpHas {
			return []*Term{n}, true
		}
		return nil, false
	case *And:
		var out []*Term
		for _, c := range n.Children {
			ts, ok := fuzzyTerms(c, r)
			if !ok {
				return nil, false
			}
			out = append(out, ts...)
		}
		return out, true
	default:
		return nil, false
	}
}

// numeric reports whether n is a plain number: not X and not a lettered
// number such as S01.
func numeric(n Num) bool {
	return !n.X && (n.S == "" || isDigits(n.S))
}
