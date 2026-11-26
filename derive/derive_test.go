package derive

import (
	"maps"
	"slices"
	"testing"
)

func TestCardKeywords(t *testing.T) {
	cases := []struct {
		text string
		want map[string]int
	}{
		{"", map[string]int{}},
		{"Elusive.", map[string]int{"elusive": 0}},
		{"Elusive. Skirmish.", map[string]int{"elusive": 0, "skirmish": 0}},
		{"Taunt.\u000bPlay: Gain 1 Æmber.", map[string]int{"taunt": 0}},
		{"Assault 3. Hazardous 2.", map[string]int{"assault": 3, "hazardous": 2}},
		{
			"Skirmish. (When you use this creature to fight, it is dealt no damage in return.)\u000bReap: Draw a card.",
			map[string]int{"skirmish": 0},
		},
		{"Deploy, elusive.", map[string]int{"deploy": 0, "elusive": 0}},
		{"Splash-attack 3.", map[string]int{"splash-attack": 3}},
		{
			"Play: A friendly creature gains elusive for the remainder of the turn.",
			map[string]int{},
		},
		{"Alpha. Play: Steal 1 Æmber. Taunt.", map[string]int{"alpha": 0}},
		{"Assault X.", map[string]int{}},
	}
	for _, c := range cases {
		if got := CardKeywords(c.text); !maps.Equal(got, c.want) {
			t.Errorf("CardKeywords(%q) = %v, want %v", c.text, got, c.want)
		}
	}
}

func TestEnhance(t *testing.T) {
	cases := []struct {
		text string
		want map[string]int
	}{
		{"Elusive.", nil},
		{"Enhance \uF360\uF360\uF361.", map[string]int{"aember": 2, "damage": 1}},
		{"Enhance \uF36F\uF560\uF36E.", map[string]int{"capture": 1, "draw": 1}},
		{"Enhance <A><R>.", map[string]int{"aember": 1, "draw": 1}},
		{"Enhance AAD.", map[string]int{"aember": 2, "damage": 1}},
		{"Enhance APT.", map[string]int{"aember": 1, "capture": 1}},
		{"Enhance \uF37A\uF37D.", map[string]int{"dis": 1, "logos": 1}},
		{"Enhance \uF360.\u000bEnhance \uF361.", map[string]int{"aember": 1, "damage": 1}},
		{"Play: Enhance cards in your deck.", nil},
	}
	for _, c := range cases {
		if got := Enhance(c.text); !maps.Equal(got, c.want) {
			t.Errorf("Enhance(%q) = %v, want %v", c.text, got, c.want)
		}
	}
}

func TestCardAbilities(t *testing.T) {
	cases := []struct {
		text string
		want []string
	}{
		{"Play: Gain 1 Æmber.", []string{"play"}},
		{"Elusive.\u000bFight/Reap: Draw a card.", []string{"fight", "reap"}},
		{"Play/After Fight/After Reap: Steal 1.", []string{"play", "fight", "reap"}},
		{"Before Fight: Deal 2 damage.", []string{"beforefight"}},
		{"Taunt. Destroyed: Return it to hand.", []string{"destroyed"}},
		{"Leaves Play: Purge it.", []string{"leavesplay"}},
		{"This creature gains, “Fight/Reap: Draw a card.”", []string{"fight", "reap"}},
		{"Each creature gains elusive.", nil},
	}
	for _, c := range cases {
		if got := CardAbilities(c.text); !slices.Equal(got, c.want) {
			t.Errorf("CardAbilities(%q) = %v, want %v", c.text, got, c.want)
		}
	}
}

func TestDisplay(t *testing.T) {
	for in, want := range map[string]string{
		"Play: Gain 1A.":                             "Play: Gain 1 Æmber.",
		"Reap: Capture 1<A>.":                        "Reap: Capture 1 Æmber.",
		"Deal 2D to a creature.":                     "Deal 2 damage to a creature.",
		"Deal 3<D> to each enemy creature.\r":        "Deal 3 damage to each enemy creature.",
		"If your opponent has more <A> than you":     "If your opponent has more Æmber than you",
		"Enhance .":                                "Enhance Æmber damage.",
		"Taunt.\u000bReap: Gain 1A.":                 "Taunt.\nReap: Gain 1 Æmber.",
		"Archive 2 cards. Ready A. Vinda.":           "Archive 2 cards. Ready A. Vinda.",
		"\uF566 After you raise the tide, ready it.": "{tide} After you raise the tide, ready it.",
	} {
		if got := Display(in); got != want {
			t.Errorf("Display(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestClean(t *testing.T) {
	for in, want := range map[string]string{
		"“They said they’d bring it right back.” <softreturn>—Squire<nonbreak>Alys": "“They said they’d bring it right back.”\n—Squire\u00a0Alys",
		"Play: Return 2\u202fenemy creatures.\r\n":                                  "Play: Return 2 enemy creatures.",
		"non\u2011Star Alliance":                                                    "non-Star Alliance",
		"Gain 1<nonbreak><A>.":                                                      "Gain 1\u00a0<A>.",
	} {
		if got := Clean(in); got != want {
			t.Errorf("Clean(%q) = %q, want %q", in, got, want)
		}
	}
	if got := Display("Gain 1<nonbreak><A>."); got != "Gain 1 Æmber." {
		t.Errorf("Display with a non-breaking space = %q", got)
	}
}

func TestFlavor(t *testing.T) {
	want := "“Remember me.”\n—Squire Duma"
	for _, in := range []string{
		"“Remember me.” - Squire Duma",
		"“Remember me.” – Squire Duma",
		"“Remember me.” –Squire Duma",
		"“Remember me.”—Squire Duma",
		"“Remember me.” <softreturn>—Squire Duma",
	} {
		if got := Flavor(in); got != want {
			t.Errorf("Flavor(%q) = %q, want %q", in, got, want)
		}
	}
	if got := Flavor("A well-known fact."); got != "A well-known fact." {
		t.Errorf("Flavor changed a hyphen: %q", got)
	}
}
