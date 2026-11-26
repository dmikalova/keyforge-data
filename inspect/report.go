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
	"strconv"
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
	// foreign holds, by expansion, the IDs of non-maverick cards from another
	// expansion in the decks of this set.
	foreign map[int]map[string]bool
	decks   int
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
		data[s.Expansion] = &setData{
			set:        s,
			cards:      map[string]Card{},
			outOfHouse: map[string]int{},
			foreign:    map[int]map[string]bool{},
		}
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
			if ok && !c.IsMaverick && c.Expansion != d.Expansion {
				if sd.foreign[c.Expansion] == nil {
					sd.foreign[c.Expansion] = map[string]bool{}
				}
				sd.foreign[c.Expansion][id] = true
			}
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
	if len(d.foreign) > 0 {
		var parts []string
		for _, exp := range slices.Sorted(maps.Keys(d.foreign)) {
			name := strconv.Itoa(exp)
			if s, ok := sets.ByExpansion(exp); ok {
				name = s.Code
			}
			parts = append(parts, fmt.Sprintf("%s %d", name, len(d.foreign[exp])))
		}
		p.f("- Card objects from other expansions in these decks: %s\n", strings.Join(parts, ", "))
	}

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
		return !creatureType(c.Type) && isNumber(c.Power) && numText(c.Power) != "0"
	}))
	section(p, "Card numbers with more than one title, type, or house", dupNumbers(cs))

	var out []Card
	for id, n := range d.outOfHouse {
		if c, ok := d.cards[id]; ok && n > 0 {
			out = append(out, c)
		}
	}
	slices.SortFunc(out, func(a, b Card) int { return strings.Compare(a.Number, b.Number) })
	section(p, "Cards outside the deck's houses (not mavericks)", out)
}

// row is one line of a section table: the card objects that differ only in
// their ID and their enhanced state.
type row struct {
	card     Card
	objects  int
	enhanced int
}

// rows collapses card objects that differ only in their ID and their
// enhanced state. Master Vault makes a new card object for each enhanced
// copy, so a set with Enhance has many objects for one printing.
func rows(cs []Card) []row {
	var out []row
	index := map[string]int{}
	for _, c := range cs {
		k := strings.Join([]string{
			c.Number, c.Title, c.Type, c.House, c.Rarity, numText(c.Power), numText(c.Armor),
			fmt.Sprint(c.IsAnomaly, c.IsNonDeck, isSet(c.HouseVariant)),
		}, "\x00")
		i, ok := index[k]
		if !ok {
			i = len(out)
			index[k] = i
			out = append(out, row{card: c})
		}
		out[i].objects++
		if c.IsEnhanced {
			out[i].enhanced++
		}
	}
	return out
}

func section(p *printer, title string, cs []Card) {
	if len(cs) == 0 {
		return
	}
	rs := rows(cs)
	p.f("\n### %s (%d)\n\n", title, len(rs))
	p.f("| # | Title | Type | House | Rarity | Power | Armor | Flags | Objects |\n")
	p.f("| --- | --- | --- | --- | --- | --- | --- | --- | --- |\n")
	for _, r := range rs {
		c := r.card
		var fl []string
		for name, on := range map[string]bool{
			"anomaly": c.IsAnomaly, "non-deck": c.IsNonDeck, "variant": isSet(c.HouseVariant),
		} {
			if on {
				fl = append(fl, name)
			}
		}
		slices.Sort(fl)
		objects := strconv.Itoa(r.objects)
		if r.enhanced > 0 {
			objects = fmt.Sprintf("%d (%d enhanced)", r.objects, r.enhanced)
		}
		p.f("| %s | %s | %s | %s | %s | %s | %s | %s | %s |\n",
			c.Number, cell(c.Title), c.Type, c.House, c.Rarity,
			cell(numText(c.Power)), cell(numText(c.Armor)), strings.Join(fl, " "), objects)
	}
}

// dupNumbers returns the cards whose number has more than one identity: a
// different title, type, or house.
func dupNumbers(cs []Card) []Card {
	by := map[string]map[string]bool{}
	for _, c := range cs {
		if by[c.Number] == nil {
			by[c.Number] = map[string]bool{}
		}
		by[c.Number][c.Title+"\x00"+c.Type+"\x00"+c.House] = true
	}
	return filter(cs, func(c Card) bool { return len(by[c.Number]) > 1 })
}

// numText returns a power or armor value without JSON quotes, or "null".
func numText(v jsontext.Value) string {
	s := strings.Trim(strings.TrimSpace(string(v)), `"`)
	if s == "" {
		return "null"
	}
	return s
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
