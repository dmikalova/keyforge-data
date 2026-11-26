package crawl

import (
	"encoding/json/v2"
	"fmt"
	"slices"

	"github.com/dmikalova/keyforge-data/mv"
	"github.com/dmikalova/keyforge-data/raw"
	"github.com/dmikalova/keyforge-data/sets"
)

// Cover is the list of cover decks of one set.
type Cover struct {
	// Set is the set slug.
	Set string `json:"set"`
	// Expansion is the Master Vault expansion ID.
	Expansion int `json:"expansion"`
	// Crawl is the ID of the crawl that the cover comes from.
	Crawl string `json:"crawl"`
	// Cards is the number of target printings (TargetKey) that the decks contain.
	Cards int `json:"cards"`
	// Decks are the deck IDs, in the order that the greedy cover chose them.
	Decks []string `json:"decks"`
}

// DeckCards is one deck and its target card IDs.
type DeckCards struct {
	ID    string
	Cards []string
}

// Greedy returns a small list of decks that together have every card that
// any of the decks has. It takes the deck that adds the most cards each time,
// and the first such deck on a tie, so the result is stable.
func Greedy(decks []DeckCards) []string {
	left := map[string]bool{}
	for _, d := range decks {
		for _, c := range d.Cards {
			left[c] = true
		}
	}
	var out []string
	used := make([]bool, len(decks))
	for len(left) > 0 {
		best, bestN := -1, 0
		for i, d := range decks {
			if used[i] {
				continue
			}
			n := 0
			for _, c := range uniq(d.Cards) {
				if left[c] {
					n++
				}
			}
			if n > bestN {
				best, bestN = i, n
			}
		}
		if best < 0 {
			break
		}
		used[best] = true
		out = append(out, decks[best].ID)
		for _, c := range decks[best].Cards {
			delete(left, c)
		}
	}
	return out
}

func uniq(s []string) []string {
	out := slices.Clone(s)
	slices.Sort(out)
	return slices.Compact(out)
}

// ComputeCover finds the cover decks of a set from the feed decks of a crawl.
// It reads the card heads from the store's English card objects.
func ComputeCover(store *raw.Store, crawlID string, s sets.Set) (Cover, error) {
	heads, err := CardHeads(store, "en")
	if err != nil {
		return Cover{}, err
	}
	var decks []DeckCards
	ids := map[string]bool{}
	for rec, err := range store.Records(crawlID) {
		if err != nil {
			return Cover{}, err
		}
		d, err := deckOf(rec)
		if err != nil {
			return Cover{}, err
		}
		if d.Expansion != s.Expansion || ids[d.ID] {
			continue
		}
		ids[d.ID] = true
		dc := DeckCards{ID: d.ID}
		for _, c := range d.CardIDs {
			h, ok := heads[c]
			if !ok {
				return Cover{}, fmt.Errorf("deck %s: card %s is not in the store", d.ID, c)
			}
			if IsTarget(h) {
				dc.Cards = append(dc.Cards, TargetKey(h))
			}
		}
		decks = append(decks, dc)
	}
	all := map[string]bool{}
	for _, d := range decks {
		for _, c := range d.Cards {
			all[c] = true
		}
	}
	return Cover{
		Set:       s.Slug,
		Expansion: s.Expansion,
		Crawl:     crawlID,
		Cards:     len(all),
		Decks:     Greedy(decks),
	}, nil
}

func deckOf(rec raw.Record) (mv.Deck, error) {
	return mv.ParseDeckObject(rec.Deck)
}

// CardHeads reads the head of every stored card object in a language. When a
// card has more than one version, any version gives the same head fields.
func CardHeads(store *raw.Store, locale string) (map[string]mv.CardHead, error) {
	versions, err := store.Linked("cards", locale)
	if err != nil {
		return nil, err
	}
	out := make(map[string]mv.CardHead, len(versions))
	for id, vs := range versions {
		var h mv.CardHead
		if err := json.Unmarshal(vs[0].Raw, &h); err != nil {
			return nil, fmt.Errorf("card %s: %w", id, err)
		}
		out[id] = h
	}
	return out, nil
}
