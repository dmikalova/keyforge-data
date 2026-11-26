// Package inspect reports what Master Vault sends for each set, before any
// distillation rule exists. The report is the input for writing the rules
// (docs/todo.md, phase 4): it shows how each set encodes gigantic halves,
// houseless creatures, evil twins, token creatures, reference cards, and X
// power.
package inspect

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"

	"github.com/dmikalova/keyforge-data/raw"
	"github.com/dmikalova/keyforge-data/sets"
)

// Card is the part of a Master Vault card object that the report reads.
type Card struct {
	ID           string         `json:"id"`
	Title        string         `json:"card_title"`
	House        string         `json:"house"`
	Type         string         `json:"card_type"`
	Text         string         `json:"card_text"`
	Traits       string         `json:"traits"`
	Amber        jsontext.Value `json:"amber"`
	Power        jsontext.Value `json:"power"`
	Armor        jsontext.Value `json:"armor"`
	Rarity       string         `json:"rarity"`
	Flavor       *string        `json:"flavor_text"`
	Number       string         `json:"card_number"`
	Expansion    int            `json:"expansion"`
	IsMaverick   bool           `json:"is_maverick"`
	IsAnomaly    bool           `json:"is_anomaly"`
	IsEnhanced   bool           `json:"is_enhanced"`
	IsNonDeck    bool           `json:"is_non_deck"`
	HouseVariant jsontext.Value `json:"house_variant"`
	FrontImage   string         `json:"front_image"`
}

// realHouses are the houses that a deck can have.
var realHouses = map[string]bool{
	"Brobnar": true, "Dis": true, "Ekwidon": true, "Geistoid": true, "Logos": true,
	"Mars": true, "Ouboros": true, "Redemption": true, "Sanctum": true, "Saurian": true,
	"Shadows": true, "Skyborn": true, "Star Alliance": true, "Unfathomable": true,
	"Untamed": true, "Keyraken": true, "Elders": true, "Ironyx Rebels": true,
}

// creatureTypes are the Master Vault types that have power.
func creatureType(t string) bool {
	t = strings.ToLower(t)
	return strings.Contains(t, "creature") && !strings.Contains(t, "art")
}

// setData is everything the report knows about one set.
type setData struct {
	set   sets.Set
	cards map[string]Card // by ID, non-maverick cards of this expansion
	// outOfHouse counts, by card ID, the decks where the card is not in one
	// of the deck's houses.
	outOfHouse map[string]int
	decks      int
}

// Write writes a Markdown report of the stored English card objects and the
// decks of the crawls.
func Write(w io.Writer, store *raw.Store, crawlIDs []string) error {
	cards, err := loadCards(store)
	if err != nil {
		return err
	}
	data := map[int]*setData{}
	for _, s := range sets.All {
		data[s.Expansion] = &setData{set: s, cards: map[string]Card{}, outOfHouse: map[string]int{}}
	}
	var unknownExp []Card
	for _, c := range cards {
		if c.IsMaverick {
			continue
		}
		d, ok := data[c.Expansion]
		if !ok {
			unknownExp = append(unknownExp, c)
			continue
		}
		d.cards[c.ID] = c
	}
	for _, id := range crawlIDs {
		if err := scanDecks(store, id, cards, data); err != nil {
			return err
		}
	}
	p := &printer{w: w}
	p.f("# Master Vault data report\n\n")
	p.f("Crawls: %s. Card objects (English): %d.\n\n", strings.Join(crawlIDs, ", "), len(cards))
	p.f("This report shows the raw data. It does not apply any rule.\n")
	if len(unknownExp) > 0 {
		p.f("\n## Cards with an unknown expansion\n\n")
		for _, c := range unknownExp {
			p.f("- %d %s %q (%s)\n", c.Expansion, c.Number, c.Title, c.Type)
		}
	}
	for _, s := range sets.All {
		writeSet(p, data[s.Expansion])
	}
	return p.err
}

func loadCards(store *raw.Store) (map[string]Card, error) {
	versions, err := store.Linked("cards", "en")
	if err != nil {
		return nil, err
	}
	out := make(map[string]Card, len(versions))
	for id, vs := range versions {
		// Use the newest version.
		v := slices.MaxFunc(
			vs,
			func(a, b raw.LinkedVersion) int { return a.ModTime.Compare(b.ModTime) },
		)
		var c Card
		if err := json.Unmarshal(v.Raw, &c); err != nil {
			return nil, fmt.Errorf("card %s: %w", id, err)
		}
		out[id] = c
	}
	return out, nil
}

type deckLinks struct {
	Expansion int `json:"expansion"`
	Links     struct {
		Houses []string `json:"houses"`
		Cards  []string `json:"cards"`
	} `json:"_links"`
}

func scanDecks(
	store *raw.Store,
	crawlID string,
	cards map[string]Card,
	data map[int]*setData,
) error {
	for rec, err := range store.Records(crawlID) {
		if err != nil {
			return err
		}
		var d deckLinks
		if err := json.Unmarshal(rec.Deck, &d); err != nil {
			return err
		}
		sd, ok := data[d.Expansion]
		if !ok {
			continue
		}
		sd.decks++
		for _, id := range d.Links.Cards {
			c, ok := cards[id]
			if !ok || c.IsMaverick || slices.Contains(d.Links.Houses, c.House) {
				continue
			}
			sd.outOfHouse[id]++
		}
	}
	return nil
}

func writeSet(p *printer, d *setData) {
	p.f("\n## %s (%s, expansion %d)\n\n", d.set.Name, d.set.Code, d.set.Expansion)
	if len(d.cards) == 0 {
		p.f("No card objects.\n")
		return
	}
	cs := slices.SortedFunc(maps.Values(d.cards), func(a, b Card) int {
		return strings.Compare(a.Number+a.Title+a.ID, b.Number+b.Title+b.ID)
	})
	p.f("Card objects: %d. Decks read: %d.\n\n", len(cs), d.decks)
	p.f("- Types: %s\n", counts(cs, func(c Card) string { return c.Type }))
	p.f("- Rarities: %s\n", counts(cs, func(c Card) string { return c.Rarity }))
	p.f("- Houses: %s\n", counts(cs, func(c Card) string { return c.House }))
	flags := func(name string, f func(Card) bool) {
		if n := len(filter(cs, f)); n > 0 {
			p.f("- %s: %d\n", name, n)
		}
	}
	flags("is_anomaly", func(c Card) bool { return c.IsAnomaly })
	flags("is_enhanced", func(c Card) bool { return c.IsEnhanced })
	flags("is_non_deck", func(c Card) bool { return c.IsNonDeck })
	flags("house_variant set", func(c Card) bool { return isSet(c.HouseVariant) })
	flags("flavor text", func(c Card) bool { return c.Flavor != nil && *c.Flavor != "" })

	section(
		p,
		"Houses that are not deck houses",
		filter(cs, func(c Card) bool { return !realHouses[c.House] }),
	)
	section(p, "Gigantic halves", filter(cs, func(c Card) bool {
		return strings.Contains(strings.ToLower(c.Type), "gigantic")
	}))
	section(p, "Evil twins", filter(cs, func(c Card) bool {
		return strings.Contains(strings.ToLower(c.Type+c.Rarity), "twin")
	}))
	section(p, "Tokens", filter(cs, func(c Card) bool {
		return strings.Contains(strings.ToLower(c.Type+c.Rarity), "token")
	}))
	section(p, "Creature types with empty or non-numeric power", filter(cs, func(c Card) bool {
		return creatureType(c.Type) && !isNumber(c.Power)
	}))
	section(p, "Non-creature types with a power value", filter(cs, func(c Card) bool {
		return !creatureType(c.Type) && isNumber(c.Power) && string(c.Power) != "0"
	}))
	section(p, "Card numbers with more than one card object", dupNumbers(cs))

	var out []Card
	for id, n := range d.outOfHouse {
		if c, ok := d.cards[id]; ok && n > 0 {
			out = append(out, c)
		}
	}
	slices.SortFunc(out, func(a, b Card) int { return strings.Compare(a.Number, b.Number) })
	section(p, "Cards outside the deck's houses (not mavericks)", out)
}

func section(p *printer, title string, cs []Card) {
	if len(cs) == 0 {
		return
	}
	p.f("\n### %s (%d)\n\n", title, len(cs))
	p.f("| # | Title | Type | House | Rarity | Power | Armor | Flags |\n")
	p.f("|---|---|---|---|---|---|---|---|\n")
	for _, c := range cs {
		var fl []string
		for name, on := range map[string]bool{
			"anomaly": c.IsAnomaly, "enhanced": c.IsEnhanced, "non-deck": c.IsNonDeck,
			"variant": isSet(c.HouseVariant),
		} {
			if on {
				fl = append(fl, name)
			}
		}
		slices.Sort(fl)
		p.f("| %s | %s | %s | %s | %s | %s | %s | %s |\n",
			c.Number, cell(c.Title), c.Type, c.House, c.Rarity,
			cell(string(c.Power)), cell(string(c.Armor)), strings.Join(fl, " "))
	}
}

func dupNumbers(cs []Card) []Card {
	by := map[string][]Card{}
	for _, c := range cs {
		by[c.Number] = append(by[c.Number], c)
	}
	var out []Card
	for _, c := range cs {
		if len(by[c.Number]) > 1 {
			out = append(out, c)
		}
	}
	return out
}

func counts(cs []Card, key func(Card) string) string {
	m := map[string]int{}
	for _, c := range cs {
		m[key(c)]++
	}
	keys := slices.Sorted(maps.Keys(m))
	parts := make([]string, len(keys))
	for i, k := range keys {
		label := k
		if label == "" {
			label = "(empty)"
		}
		parts[i] = fmt.Sprintf("%s %d", label, m[k])
	}
	return strings.Join(parts, ", ")
}

func filter(cs []Card, f func(Card) bool) []Card {
	var out []Card
	for _, c := range cs {
		if f(c) {
			out = append(out, c)
		}
	}
	return out
}

func isSet(v jsontext.Value) bool {
	s := strings.TrimSpace(string(v))
	return s != "" && s != "null" && s != "false" && s != `""`
}

func isNumber(v jsontext.Value) bool {
	s := strings.Trim(strings.TrimSpace(string(v)), `"`)
	if s == "" || s == "null" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func cell(s string) string {
	return strings.ReplaceAll(s, "|", `\|`)
}

type printer struct {
	w   io.Writer
	err error
}

func (p *printer) f(format string, args ...any) {
	if p.err != nil {
		return
	}
	_, p.err = fmt.Fprintf(p.w, format, args...)
}
