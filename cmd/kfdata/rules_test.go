package main

import (
	"strings"
	"testing"

	"github.com/dmikalova/keyforge-data/card"
)

func TestRuleLabel(t *testing.T) {
	cases := []struct {
		r    sheetRule
		want string
	}{
		{
			sheetRule{
				kind:  "always",
				pages: []string{"A"},
				note:  "this card always appears in a deck with:",
			},
			"Always with",
		},
		{
			sheetRule{
				kind:  "always",
				pages: []string{"A"},
				note:  "this card always appears in a deck with at least 2 copies of:",
			},
			"Always with 2+",
		},
		{sheetRule{kind: "sometimes", pages: []string{"A"}}, "Sometimes with"},
		{
			sheetRule{kind: "fixed", note: "this card always appears in a deck with:"},
			"Always together",
		},
		{
			sheetRule{
				kind:  "fixed",
				pages: []string{"A"},
				note:  "this card always appears in a deck with one of the gigantic creatures:",
			},
			"Always with one of",
		},
		{
			sheetRule{
				kind: "others",
				note: "this card always appears in a deck with at least two other Sins:",
			},
			"Always with 2+ others",
		},
		{
			sheetRule{
				kind: "others",
				note: "this card always appears in a deck with at least one other Shard:",
			},
			"Always with another",
		},
		{
			sheetRule{kind: "variant", note: `Other "Dæmo-" variants of this card are:`},
			"Dæmo- variants",
		},
		{sheetRule{kind: "variant", note: "Other variants of this card are:"}, "Variants"},
	}
	for _, c := range cases {
		if got := ruleLabel(c.r); got != c.want {
			t.Errorf("ruleLabel(%+v) = %q, want %q", c.r, got, c.want)
		}
	}
}

func TestImportRules(t *testing.T) {
	tsv := "meta\ttitle\t\t\n" +
		"Pages\tType\tCards\tText\n" +
		"•Faygin•\talways\t•Urchin•\tthis card always appears in a deck with at least 2 copies of:\n" +
		"•Theory or Conjecture•\tsometimes\t•Prof. Garwynne•Prof. Garwynne (Evil Twin)•\tthis card sometimes appears:\n" +
		"\tfixed\t•Scylla•Charybdis•\tthis card always appears in a deck with:\n" +
		"•Angry Mob•\tcopies\t\tThere are always at least 2 copies of this card in a deck.\n"
	rows, err := readSheet(strings.NewReader(tsv))
	if err != nil {
		t.Fatal(err)
	}
	cards := []card.Card{
		{
			Title: "Faygin",
		},
		{Title: "Urchin"},
		{Title: "Theory or Conjecture"},
		{Title: "Prof. Garwynne"},
		{Title: "Scylla"},
	}
	rules, missing := importRules(rows, cards)
	if len(rules) != 2 {
		t.Fatalf("rules = %+v, want 2 (the group with one known card is dropped)", rules)
	}
	if got := rules[1].With; len(got) != 2 || got[1] != "Prof. Garwynne (Evil Twin)" {
		t.Errorf("sometimes with = %q", got)
	}
	if len(missing) != 1 || missing[0] != "Charybdis" {
		t.Errorf("missing = %q, want Charybdis", missing)
	}
}
