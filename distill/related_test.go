package distill

import (
	"slices"
	"testing"
)

func TestFindTitles(t *testing.T) {
	titles := []string{"Mega Narp", "Sucker Punch", "Old Bruno", "Narp", "Punch", "Fear", "Squire"}
	cases := []struct {
		text, own string
		want      []string
	}{
		{"Play: Deal 2 damage to Old Bruno.", "Anger", []string{"Old Bruno"}},
		{"Mega Narp gains taunt. Narp is ready.", "Mega Narp", nil},
		{"If Narp is in play, gain 1 Æmber.", "Hug", []string{"Narp"}},
		{"Play: Ready Sucker Punch. Punch a creature.", "Sucker Punch", nil},
		{"Fear. Play: Return a creature.", "Hex", nil},
		{"Play a Fear card.", "Hex", []string{"Fear"}},
		{"Narpish things.", "Hex", nil},
		{"“Remember me.”\n—Squire Duma", "Duma", nil},
		{"Ready a Squire creature.", "Hex", []string{"Squire"}},
	}
	for _, c := range cases {
		if got := findTitles(c.text, titles, c.own); !slices.Equal(got, c.want) {
			t.Errorf("findTitles(%q) = %v, want %v", c.text, got, c.want)
		}
	}
}
