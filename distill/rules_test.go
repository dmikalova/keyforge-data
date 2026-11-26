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
