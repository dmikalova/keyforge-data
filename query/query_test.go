package query

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

// card is a small test item.
type card struct {
	name     string
	aliases  []string
	text     string
	house    string
	ctype    string
	power    Num
	armor    int
	number   string
	keywords map[string]int
	enhance  map[string]int
	flags    []string
}

func registry(t testing.TB) *Registry[card] {
	t.Helper()
	strs := func(f func(card) string) func(card) []string {
		return func(c card) []string { return []string{f(c)} }
	}
	r, err := NewRegistry(
		[]Field[card]{
			{
				Name: "name", Alias: "n", Kind: KindFuzzy, Bare: true,
				Strings: func(c card) []string { return append([]string{c.name}, c.aliases...) },
			},
			{
				Name: "text", Alias: "x", Kind: KindText, Bare: true,
				Strings: strs(func(c card) string { return c.text }),
			},
			{
				Name: "house", Alias: "h", Kind: KindEnum,
				Values:  []string{"Brobnar", "Dis", "Logos", "Star Alliance", "Untamed"},
				Strings: strs(func(c card) string { return c.house }),
			},
			{
				Name: "type", Alias: "t", Kind: KindEnum,
				Values:  []string{"action", "artifact", "creature", "upgrade"},
				Strings: strs(func(c card) string { return c.ctype }),
			},
			{
				Name: "power", Alias: "p", Kind: KindNumber,
				Numbers: func(c card) []Num {
					if c.ctype != "creature" {
						return nil
					}
					return []Num{c.power}
				},
			},
			{
				Name: "armor", Alias: "ar", Kind: KindNumber,
				Numbers: func(c card) []Num { return []Num{{N: c.armor}} },
			},
			{
				Name: "number", Kind: KindNumber,
				Numbers: func(c card) []Num {
					n := Num{S: c.number}
					if isDigits(c.number) {
						n.N = atoi(c.number)
					}
					return []Num{n}
				},
			},
			{
				Name: "keyword", Alias: "k", Kind: KindKeyed,
				Values: []string{"elusive", "assault", "skirmish", "taunt"},
				Keyed:  func(c card) map[string]int { return c.keywords },
			},
			{
				Name: "enhance", Alias: "en", Kind: KindKeyed,
				Values: []string{"aember", "capture", "damage", "draw"},
				Keyed:  func(c card) map[string]int { return c.enhance },
			},
			{
				Name: "sort", Kind: KindDirective, Values: []string{"name", "power", "random"},
			},
			{Name: "seed", Kind: KindDirective},
		},
		[]Flag[card]{
			{
				Name: "gigantic",
				Test: func(c card) bool { return slices.Contains(c.flags, "gigantic") },
			},
			{
				Name: "eviltwin",
				Test: func(c card) bool { return slices.Contains(c.flags, "eviltwin") },
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func atoi(s string) int {
	n := 0
	for _, c := range s {
		n = n*10 + int(c-'0')
	}
	return n
}

var cards = []card{
	{
		name: "Troll", text: "Troll deals no damage.", house: "Brobnar", ctype: "creature",
		power: Num{N: 8}, number: "001", keywords: map[string]int{},
	},
	{
		name: "Urchin", text: "Elusive. Play: Steal 1 Æmber.", house: "Shadows", ctype: "creature",
		power: Num{N: 1}, number: "002", keywords: map[string]int{"elusive": 0},
	},
	{
		name: "Krump", text: "Assault 2. After an enemy creature is destroyed...",
		house: "Brobnar", ctype: "creature", power: Num{N: 6}, armor: 1, number: "003",
		keywords: map[string]int{"assault": 2},
	},
	{
		name: "Mimic Gel", text: "Mimic Gel's power is X.", house: "Logos", ctype: "creature",
		power: Num{X: true}, number: "004", keywords: map[string]int{},
	},
	{
		name: "Anger", text: "Play: Ready and fight with a friendly creature.",
		house: "Brobnar", ctype: "action", number: "005",
		enhance: map[string]int{"capture": 1, "draw": 1},
	},
	{
		name: "ANT1-10NY", aliases: []string{"anthony"}, text: "Taunt.", house: "Star Alliance",
		ctype: "creature", power: Num{N: 3}, number: "S01", keywords: map[string]int{"taunt": 0},
		flags: []string{"gigantic"},
	},
}

func names(t *testing.T, r *Registry[card], src string) []string {
	t.Helper()
	q, err := r.Parse(src)
	if err != nil {
		t.Fatalf("Parse(%q): %v", src, err)
	}
	match := r.Match(q)
	var out []string
	for _, c := range cards {
		if match(c) {
			out = append(out, c.name)
		}
	}
	return out
}

func TestParseFormatsCanonically(t *testing.T) {
	r := registry(t)
	cases := []struct{ in, want string }{
		{"", ""},
		{"troll", "troll"},
		{"h:dis t:creature", "house:Dis type:creature"},
		{"h:dis and t:creature", "house:Dis type:creature"},
		{"H:DIS OR h:logos", "house:Dis or house:Logos"},
		{"(h:dis or h:logos) p>=3", "(house:Dis or house:Logos) power>=3"},
		{"h:dis or (h:logos p>=3)", "house:Dis or house:Logos power>=3"},
		{"-h:dis", "-house:Dis"},
		{"not h:dis", "-house:Dis"},
		{"not (h:dis or h:logos)", "-(house:Dis or house:Logos)"},
		{`x:"steal 1"`, `text:"steal 1"`},
		{`"steal 1"`, `"steal 1"`},
		{"p:2..4", "power:2..4"},
		{"p:x", "power:x"},
		{"#001", "number:001"},
		{"#>=100", "number>=100"},
		{"#S01", "number:S01"},
		{"k:assault>=2", "keyword:assault>=2"},
		{"k:elusive", "keyword:elusive"},
		{"en:capture", "enhance:capture"},
		{"en>=2", "enhance>=2"},
		{"is:gigantic", "is:gigantic"},
		{"h:star-alliance", "house:\"Star Alliance\""},
		{"sort:power troll seed:abc", "troll sort:power seed:abc"},
		{"((troll))", "troll"},
		{"a (b c) d", "a b c d"},
		{"a or (b or c)", "a or b or c"},
		{"ant1-10ny", "ant1-10ny"},
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
		again, err := r.Parse(q.String())
		if err != nil {
			t.Errorf("reparse of %q: %v", q.String(), err)
			continue
		}
		if again.String() != q.String() {
			t.Errorf("reparse of %q gives %q", q.String(), again.String())
		}
	}
}

func TestParseErrors(t *testing.T) {
	r := registry(t)
	cases := []struct{ in, msg, suggest string }{
		{"hosue:dis", "unknown field", "house"},
		{"h:diss", "not a house value", "Dis"},
		{"is:gigantik", "not a is value", "gigantic"},
		{"p>=abc", "not a number", ""},
		{"p>x", "power does not take", ""},
		{"h>dis", "house does not take", ""},
		{"(troll", "no matching", ""},
		{"troll)", "unexpected", ""},
		{`x:"open`, "no closing quote", ""},
		{"troll or", "expected a term", ""},
		{"and troll", "needs a term", ""},
		{"-sort:power", "cannot be inside", ""},
		{"troll or sort:power", "cannot be inside", ""},
		{"k:assault>=x2", "not a number", ""},
		{"p:4..", "not a range", ""},
	}
	for _, c := range cases {
		_, err := r.Parse(c.in)
		var qe *Error
		if !errors.As(err, &qe) {
			t.Errorf("Parse(%q) error = %v, want a *Error", c.in, err)
			continue
		}
		if !strings.Contains(qe.Msg, c.msg) {
			t.Errorf("Parse(%q) message = %q, want it to contain %q", c.in, qe.Msg, c.msg)
		}
		if qe.Suggest != c.suggest {
			t.Errorf("Parse(%q) suggest = %q, want %q", c.in, qe.Suggest, c.suggest)
		}
	}
}

func TestErrorPointsAtTheToken(t *testing.T) {
	_, err := registry(t).Parse("troll hosue:dis")
	var qe *Error
	if !errors.As(err, &qe) || qe.Pos.Start != 6 || qe.Pos.End != 11 {
		t.Fatalf("error = %#v, want the span of hosue", err)
	}
}

func TestMatch(t *testing.T) {
	r := registry(t)
	cases := []struct {
		in   string
		want []string
	}{
		{"", []string{"Troll", "Urchin", "Krump", "Mimic Gel", "Anger", "ANT1-10NY"}},
		{"h:brobnar t:creature", []string{"Troll", "Krump"}},
		{"h:brobnar -t:creature", []string{"Anger"}},
		{"h:brobnar or h:logos", []string{"Troll", "Krump", "Mimic Gel", "Anger"}},
		{"h:brobnar (p>=7 or k:assault)", []string{"Troll", "Krump"}},
		{"p>=6", []string{"Troll", "Krump"}},
		{"p:1..3", []string{"Urchin", "ANT1-10NY"}},
		{"p:x", []string{"Mimic Gel"}},
		{"t:creature p!=x", []string{"Troll", "Urchin", "Krump", "ANT1-10NY"}},
		{"p<100", []string{"Troll", "Urchin", "Krump", "ANT1-10NY"}},
		{"ar>=1", []string{"Krump"}},
		{"#003", []string{"Krump"}},
		{"#>=4", []string{"Mimic Gel", "Anger"}},
		{"#<2", []string{"Troll"}},
		{"#:1..3", []string{"Troll", "Urchin", "Krump"}},
		{"#s01", []string{"ANT1-10NY"}},
		{"k:assault>=2", []string{"Krump"}},
		{"k:assault>=3", nil},
		{"k:elusive", []string{"Urchin"}},
		{"en:capture", []string{"Anger"}},
		{"en>=2", []string{"Anger"}},
		{"x:steal", []string{"Urchin"}},
		{"x:aember", []string{"Urchin"}},
		{`x:"ready and fight"`, []string{"Anger"}},
		{"x!=taunt.", []string{"Troll", "Urchin", "Krump", "Mimic Gel", "Anger"}},
		{"steal", []string{"Urchin"}},
		{"troll", []string{"Troll"}},
		{"n:mmc", []string{"Mimic Gel"}},
		{"n:anthony", []string{"ANT1-10NY"}},
		{"n:ant110", []string{"ANT1-10NY"}},
		{`n:"mimic gel"`, []string{"Mimic Gel"}},
		{"n=troll", []string{"Troll"}},
		{"h:staralliance", []string{"ANT1-10NY"}},
		{"is:gigantic", []string{"ANT1-10NY"}},
		{"-is:gigantic h:star-alliance", nil},
		{"sort:name h:logos", []string{"Mimic Gel"}},
		{"not not troll", []string{"Troll"}},
	}
	for _, c := range cases {
		if got := names(t, r, c.in); !slices.Equal(got, c.want) {
			t.Errorf("%q matches %v, want %v", c.in, got, c.want)
		}
	}
}

func TestDirectives(t *testing.T) {
	q, err := registry(t).Parse("sort:name troll sort:power seed:42")
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := q.Directive("sort"); v != "power" {
		t.Errorf("sort = %q, want the last one, power", v)
	}
	if v, _ := q.Directive("seed"); v != "42" {
		t.Errorf("seed = %q", v)
	}
	if _, ok := q.Directive("order"); ok {
		t.Error("order is set")
	}
}

func TestFuzzyRank(t *testing.T) {
	r := registry(t)
	q, err := r.Parse("n:mg")
	if err != nil {
		t.Fatal(err)
	}
	mimic, ok := r.FuzzyRank(q, cards[3])
	if !ok {
		t.Fatal("n:mg is a fuzzy-only query")
	}
	krump, _ := r.FuzzyRank(q, cards[2])
	if mimic <= krump {
		t.Errorf("Mimic Gel scores %d, Krump %d; want word starts to win", mimic, krump)
	}
	q, _ = r.Parse("n:mg h:logos")
	if _, ok := r.FuzzyRank(q, cards[3]); ok {
		t.Error("a query with a house term is not fuzzy-only")
	}
}

func TestFuzzyScore(t *testing.T) {
	cases := []struct {
		p, s string
		ok   bool
	}{
		{"brtm", "Brothers in Battle Team", true},
		{"tmrb", "Brothers in Battle Team", false},
		{"aember", "Æmber Imp", true},
		{"", "anything", true},
		{"ae imp", "Æmber Imp", true},
		{"zz", "Troll", false},
	}
	for _, c := range cases {
		if _, ok := FuzzyScore(c.p, c.s); ok != c.ok {
			t.Errorf("FuzzyScore(%q, %q) ok = %t, want %t", c.p, c.s, ok, c.ok)
		}
	}
	runs, _ := FuzzyScore("troll", "Troll")
	spread, _ := FuzzyScore("troll", "The Rot Of All Lands")
	if runs <= spread {
		t.Errorf("a run scores %d, a spread match %d; want the run higher", runs, spread)
	}
}

func TestFold(t *testing.T) {
	cases := map[string]string{
		"Æmber":      "aember",
		"Pandulf’s":  "pandulf's",
		"Señor":      "senor",
		"Ø-zone—now": "o-zone-now",
	}
	for in, want := range cases {
		if got := Fold(in); got != want {
			t.Errorf("Fold(%q) = %q, want %q", in, got, want)
		}
	}
	if got := Normalize("Star Alliance!"); got != "staralliance" {
		t.Errorf("Normalize = %q", got)
	}
}

func TestNewRegistryRejectsDuplicateNames(t *testing.T) {
	_, err := NewRegistry([]Field[card]{
		{Name: "a", Kind: KindText, Strings: func(card) []string { return nil }},
		{Name: "b", Alias: "a", Kind: KindText, Strings: func(card) []string { return nil }},
	}, nil)
	if err == nil {
		t.Fatal("duplicate name accepted")
	}
	_, err = NewRegistry([]Field[card]{{Name: "a", Kind: KindNumber}}, nil)
	if err == nil {
		t.Fatal("field without accessor accepted")
	}
}

func FuzzParse(f *testing.F) {
	for _, s := range []string{
		"h:dis (p>=3 or k:assault>=2) -is:gigantic",
		`x:"steal 1" sort:power`,
		"#S01 or n:anthony",
		"((a or b) c) not d",
		`"`, "(", ")", "-", "a:b:c", "p:1..2..3", "k:>=", "#",
	} {
		f.Add(s)
	}
	r := registry(f)
	f.Fuzz(func(t *testing.T, src string) {
		q, err := r.Parse(src)
		if err != nil {
			return
		}
		match := r.Match(q)
		for _, c := range cards {
			match(c)
		}
		again, err := r.Parse(q.String())
		if err != nil {
			t.Fatalf("Parse(%q) ok but its String %q does not parse: %v", src, q.String(), err)
		}
		if again.String() != q.String() {
			t.Fatalf("String is not stable: %q then %q", q.String(), again.String())
		}
	})
}
