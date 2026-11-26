package overlay

import (
	"slices"
	"testing"

	"github.com/dmikalova/keyforge-data/card"
)

func TestLint(t *testing.T) {
	cards := []card.Card{
		{Title: "Picaroon", Printings: []card.Printing{{Set: "massmutation", Number: "028"}}},
		{Title: "Urchin", Printings: []card.Printing{{Set: "callofthearchons", Number: "320"}}},
		{Title: "Faygin", Printings: []card.Printing{{Set: "callofthearchons", Number: "301"}}},
	}
	zero := 0
	files := map[string]File{
		ArtistsFile: {
			KnownArtists:  []string{"Brolken"},
			ArtistAliases: map[string]string{"Brolkn": "Brolken", "Nobody": "Nobody Else"},
		},
		"massmutation": {
			Cards:   map[string]CardFix{"028": {Armor: &zero}, "999": {Note: "no fix"}},
			Artists: map[string]string{"028": "Brolkn"},
		},
		"notaset": {},
		ThreadsFile: {
			Threads: []Thread{
				{Label: "Pair", Titles: []string{"Faygin", "Urchin"}},
				{Label: "Same", Titles: []string{"Urchin", "Faygin"}},
				{Label: "Lonely", Titles: []string{"Faygin", "Missing Card"}},
			},
			Notes: []NoteRule{
				{Kind: "copies", Title: "Urchin (Shadows)", Text: "Always 2+ copies"},
			},
		},
		RulesFile: {
			Rules: []Rule{
				{
					Kind:  "always",
					Label: "Always with",
					Cards: []string{"Faygin"},
					With:  []string{"Urchin", "Zealot"},
				},
			},
		},
	}
	var got []string
	for _, f := range Lint(files, cards) {
		got = append(got, f.String())
	}
	want := []string{
		`error: _artists.json: alias "Nobody" names "Nobody Else", which is not a known artist`,
		`warning: _deckrules.json: rule: no card has the title "Zealot"`,
		`error: _threads.json: threads "Pair" and "Same" have the same cards`,
		`error: _threads.json: thread "Lonely": no card has the title "Missing Card"`,
		`error: _threads.json: thread "Lonely" links fewer than 2 cards`,
		`error: massmutation.json: card fix 999: no printing has this number`,
		`error: massmutation.json: card fix 999 changes nothing`,
		`error: massmutation.json: artist of 028 is "Brolkn", an alias of "Brolken"`,
		`error: notaset.json: "notaset" is not a set slug`,
	}
	if !slices.Equal(got, want) {
		t.Errorf("Lint =\n%s\nwant\n%s", join(got), join(want))
	}
}

func join(s []string) string {
	out := ""
	for _, x := range s {
		out += "  " + x + "\n"
	}
	return out
}
