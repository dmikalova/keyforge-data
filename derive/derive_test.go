package derive

import (
	"maps"
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
