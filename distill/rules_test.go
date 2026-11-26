package distill

import (
	"slices"
	"testing"

	"github.com/dmikalova/keyforge-data/card"
)

func TestRules(t *testing.T) {
	cards := []card.Card{
		{ID: "faygin-shadows", Title: "Faygin"},
		{ID: "urchin-shadows", Title: "Urchin"},
		{ID: "profgarwynne-logos", Title: "Prof. Garwynne"},
		{ID: "profgarwynne-logos-eviltwin", Title: "Prof. Garwynne"},
		{ID: "theoryorconjecture-logos", Title: "Theory or Conjecture"},
		{ID: "pride-dis", Title: "Pride"},
		{ID: "envy-dis", Title: "Envy"},
	}
	overrides := map[string]Override{
		threadsKey: {Threads: []Thread{{Label: "The sins", Titles: []string{"Pride", "Envy"}}}},
		rulesKey: {Rules: []Rule{
			{
				Kind:  "always",
				Label: "Always with 2+",
				Cards: []string{"Faygin"},
				With:  []string{"Urchin"},
			},
			{
				Kind:  "sometimes",
				Label: "Sometimes with",
				Cards: []string{"Theory or Conjecture"},
				With:  []string{"Prof. Garwynne", "Prof. Garwynne (Evil Twin)"},
			},
			// The curated thread has these cards, so the group adds nothing.
			{Kind: "others", Label: "Always with another", With: []string{"Pride", "Envy"}},
		}},
	}
	linkCards(cards, overrides)
	related := func(id string) []string {
		var out []string
		for _, c := range cards {
			if c.ID != id {
				continue
			}
			for _, r := range c.Related {
				for _, o := range r.Cards {
					out = append(out, r.Label+": "+o)
				}
			}
		}
		return out
	}
	for id, want := range map[string][]string{
		"faygin-shadows": {"Always with 2+: urchin-shadows"},
		"urchin-shadows": nil,
		"theoryorconjecture-logos": {
			"Sometimes with: profgarwynne-logos",
			"Sometimes with: profgarwynne-logos-eviltwin",
		},
		"pride-dis": {"The sins: envy-dis"},
	} {
		if got := related(id); !slices.Equal(got, want) {
			t.Errorf("%s related = %q, want %q", id, got, want)
		}
	}
}

func TestNotes(t *testing.T) {
	cards := []card.Card{
		{ID: "plaguerat-dis", Title: "Plague Rat", House: "dis"},
		{ID: "armageddoncloak-sanctum", Title: "Armageddon Cloak", House: "sanctum"},
		{ID: "exchangeofficer-staralliance", Title: "Exchange Officer", House: "staralliance"},
		{ID: "orbofwonder-sanctum", Title: "Orb of Wonder", House: "sanctum"},
	}
	overrides := map[string]Override{
		rulesKey: {Notes: []NoteRule{
			{Kind: "copies", Title: "Plague Rat", Text: "Always 3+ copies"},
			{
				Kind:  "houseswap",
				Title: "Armageddon Cloak",
				Text:  "Appears in house Redemption in Tokens of Change",
			},
			// The dataset has no Redemption card, so the note is dropped.
			{
				Kind:  "houseswap",
				Title: "Armageddon Cloak (Redemption)",
				Text:  "Was in Sanctum before Redemption",
			},
			{
				Kind:  "anomaly",
				Title: "Orb of Wonder (Anomaly)",
				Text:  "Appears in house Sanctum in Mass Mutation",
			},
			{Kind: "special", Title: "Exchange Officer", Text: "A long sheet note"},
		}},
		threadsKey: {Notes: []NoteRule{
			{Kind: "special", Title: "Exchange Officer", Text: "Always a Maverick"},
		}},
	}
	linkCards(cards, overrides)
	want := map[string][]string{
		"plaguerat-dis":                {"Always 3+ copies"},
		"armageddoncloak-sanctum":      {"Appears in house Redemption in Tokens of Change"},
		"exchangeofficer-staralliance": {"Always a Maverick"},
		"orbofwonder-sanctum":          {"Appears in house Sanctum in Mass Mutation"},
	}
	for _, c := range cards {
		var got []string
		for _, n := range c.Notes {
			got = append(got, n.Text)
		}
		if !slices.Equal(got, want[c.ID]) {
			t.Errorf("%s notes = %q, want %q", c.ID, got, want[c.ID])
		}
	}
}
