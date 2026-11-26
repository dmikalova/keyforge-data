package query

import (
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"
)

// shape writes the AST of a query without the source positions. Two queries
// with the same shape match the same items.
func shape(q *Query) string {
	var b strings.Builder
	if q.Root != nil {
		shapeNode(&b, q.Root)
	}
	for _, d := range q.Directives {
		fmt.Fprintf(&b, " dir(%s=%q)", d.Name, d.Value)
	}
	return b.String()
}

func shapeNode(b *strings.Builder, n Node) {
	switch n := n.(type) {
	case *And:
		b.WriteString("and(")
		shapeList(b, n.Children)
		b.WriteString(")")
	case *Or:
		fmt.Fprintf(b, "or[list=%v](", n.List)
		shapeList(b, n.Children)
		b.WriteString(")")
	case *Not:
		b.WriteString("not(")
		shapeNode(b, n.Child)
		b.WriteString(")")
	case *Term:
		v := n.Value
		fmt.Fprintf(b, "term(%s key=%q op=%s text=%q quoted=%v num=%d x=%v range=%v hi=%d)",
			n.Field, n.Key, n.Op, v.Text, v.Quoted, v.Num, v.X, v.Range, v.Hi)
	case *Word:
		fmt.Fprintf(b, "word(%q quoted=%v)", n.Text, n.Quoted)
	default:
		fmt.Fprintf(b, "unknown(%T)", n)
	}
}

func shapeList(b *strings.Builder, nodes []Node) {
	for i, c := range nodes {
		if i > 0 {
			b.WriteString(", ")
		}
		shapeNode(b, c)
	}
}

// checkRoundTrip checks that the canonical form of a query parses to the
// same AST, and that the canonical form is stable.
func checkRoundTrip(t *testing.T, r *Registry[card], src string) {
	t.Helper()
	q, err := r.Parse(src)
	if err != nil {
		return
	}
	s := q.String()
	again, err := r.Parse(s)
	if err != nil {
		t.Fatalf("Parse(%q) is ok, but its canonical form %q does not parse: %v", src, s, err)
	}
	if got, want := shape(again), shape(q); got != want {
		t.Fatalf("Parse(%q): the canonical form %q gives another AST:\n got %s\nwant %s",
			src, s, got, want)
	}
	if again.String() != s {
		t.Fatalf("the canonical form is not stable: %q then %q", s, again.String())
	}
}

// roundTripCases are queries for each part of the language.
var roundTripCases = []string{
	// Enum values: a prefix, any case, and a value with a space.
	"r:r", "h:br t:act", "H:DIS", "h:Star-Alliance", `h:"star alliance"`, "h:st,u",
	"h:STAR", "t:creature,action", "h=dis,logos", "h!=dis", "-h:dis,logos",
	// Comma lists in text, name, and keyed fields.
	"x:a,b", "n:a,b", `x:"a,b"`, `x:"a,b",c`, "k:elusive,taunt", "en:capture,draw",
	// Quotes.
	`"steal 1"`, `x:"steal 1"`, `n:"troll"`, `x:"troll"`, `"troll"`, `x:""`,
	// Negation, grouping, and precedence.
	"not a", "-a", "- a", "not not a", "--a", "-(a or b)", "not (a b)",
	"a or b c", "(a or b) c", "a (b or c) d", "a or (b c)", "((a))", "(a or (b or c))",
	"a and b or c and d", "a or b and c or d", "-a or -b", "-(a) -(b c)",
	// Number comparisons and ranges.
	"p:3", "p=3", "p!=3", "p<3", "p<=3", "p>3", "p>=3", "p:x", "p=X", "p:2..4",
	"p:0..0", "ar>=1", "#001", "#S01", "#>=100", "number:2..10",
	// Keyed fields.
	"k:assault>=2", "k:assault:2..4", "k:assault=0", "k:assault<2", "k:assault",
	"k:ass>=2", "k:elusive", "en>=2", "en:capture>=1", "en:capture",
	// Flags.
	"is:gigantic", "is:gig", "-is:eviltwin", "is:gigantic,eviltwin",
	// Directives.
	"sort:power", "troll sort:power seed:abc", "sort:name,power", "seed:123 a",
	// Text with punctuation and non-ASCII.
	"æmber", "Æmber", "x:Æmber", "fábio", `x:"Ol’ Paddy"`, "ol’", "x:non-star",
	"n:ant1-10ny", "x:\u00a0", "a\u00a0b", `x:"a\u00a0b"`, "x:it’s", "x:(", "x:)",
	"voilà", "x:Åmber", "n:déjà", "x:\r", "#\"\r\"", "numBer:0..00", "X:",
	"x:a.b", "x:1+1", "x:!", "x:=", "x::", "x:<", "x:a=b", "n:mega-x",
}

func TestRoundTrip(t *testing.T) {
	r := registry(t)
	for _, src := range roundTripCases {
		t.Run(src, func(t *testing.T) { checkRoundTrip(t, r, src) })
	}
}

// roundTripParts are the pieces that TestRoundTripGenerated joins into
// queries.
var roundTripParts = []string{
	"h:dis", "h:st", "r:r", "t:act", "p>=3", "p:x", "p:2..4", "k:assault>=2",
	"k:assault:2..4", "k:elusive", "en:capture", "is:gigantic", "#001", "troll",
	`"steal 1"`, `x:"a,b"`, "x:Æmber", "h:dis,logos", "k:elusive,taunt", "n:fábio",
	"sort:power", "seed:7",
}

// genQuery returns a random query from roundTripParts, with random
// operators and groups.
func genQuery(rng *rand.Rand, depth int) string {
	if depth == 0 || rng.IntN(3) == 0 {
		return roundTripParts[rng.IntN(len(roundTripParts))]
	}
	a, b := genQuery(rng, depth-1), genQuery(rng, depth-1)
	switch rng.IntN(5) {
	case 0:
		return a + " or " + b
	case 1:
		return "(" + a + ") " + b
	case 2:
		return "-" + a + " " + b
	case 3:
		return "not (" + a + " or " + b + ")"
	default:
		return a + " " + b
	}
}

func TestRoundTripGenerated(t *testing.T) {
	r := registry(t)
	rng := rand.New(rand.NewPCG(1, 2))
	for range 2000 {
		checkRoundTrip(t, r, genQuery(rng, 4))
	}
}

func FuzzRoundTrip(f *testing.F) {
	for _, s := range roundTripCases {
		f.Add(s)
	}
	r := registry(f)
	f.Fuzz(func(t *testing.T, src string) {
		checkRoundTrip(t, r, src)
	})
}

// TestLexKeepsLetters checks that a letter whose UTF-8 bytes include 0x85
// or 0xA0 does not end a word, and that a no-break space does.
func TestLexKeepsLetters(t *testing.T) {
	r := registry(t)
	cases := []struct{ in, want string }{
		{"voilà", "voilà"},
		{"x:Åmber", "text:Åmber"},
		{"n:déjà", "name:déjà"},
		{"a\u00a0b", "a b"},
		{`x:"a` + "\u00a0" + `b"`, `text:"a` + "\u00a0" + `b"`},
		{"x:\r", "text:"},
		{"#001\u2003troll", "number:001 troll"},
	}
	for _, c := range cases {
		q, err := r.Parse(c.in)
		if err != nil {
			t.Errorf("Parse(%q): %v", c.in, err)
			continue
		}
		if got := q.String(); got != c.want {
			t.Errorf("Parse(%q).String() = %q, want %q", c.in, got, c.want)
		}
	}
}
