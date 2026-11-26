package mv

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
)

// Page is one Master Vault response, split into its decks and its linked
// objects. Each deck and each linked object keeps its bytes as received.
type Page struct {
	// Count is the total number of decks that the feed query matches. It is
	// zero for a single-deck response.
	Count int
	// Decks are the decks in the response.
	Decks []Deck
	// Linked holds the linked objects by kind ("cards", "houses" …).
	Linked map[string][]Object
}

// Deck is one deck as received, without the linked objects.
type Deck struct {
	// ID is the Master Vault deck ID.
	ID string
	// Expansion is the deck's expansion ID.
	Expansion int
	// CardIDs are the IDs of the deck's cards, with repeats.
	CardIDs []string
	// Raw is the deck object as received.
	Raw jsontext.Value
}

// Object is one linked object as received.
type Object struct {
	// ID is the object's Master Vault ID.
	ID string
	// Raw is the object as received.
	Raw jsontext.Value
}

// CardHead is the part of a card object that the crawler reads.
type CardHead struct {
	ID         string `json:"id"`
	Expansion  int    `json:"expansion"`
	CardNumber string `json:"card_number"`
	Title      string `json:"card_title"`
	House      string `json:"house"`
	IsMaverick bool   `json:"is_maverick"`
}

type rawPage struct {
	Count  int                         `json:"count"`
	Data   []jsontext.Value            `json:"data"`
	Linked map[string][]jsontext.Value `json:"_linked"`
}

type rawDeckResponse struct {
	Data   jsontext.Value              `json:"data"`
	Linked map[string][]jsontext.Value `json:"_linked"`
}

type deckHead struct {
	ID        string `json:"id"`
	Expansion int    `json:"expansion"`
	Links     struct {
		Cards []string `json:"cards"`
	} `json:"_links"`
}

type objectHead struct {
	ID string `json:"id"`
}

// ParsePage splits a feed response.
func ParsePage(body []byte) (*Page, error) {
	var rp rawPage
	if err := json.Unmarshal(body, &rp); err != nil {
		return nil, fmt.Errorf("mv: decode feed page: %w", err)
	}
	p := &Page{Count: rp.Count}
	for _, d := range rp.Data {
		deck, err := parseDeck(d)
		if err != nil {
			return nil, err
		}
		p.Decks = append(p.Decks, deck)
	}
	linked, err := parseLinked(rp.Linked)
	if err != nil {
		return nil, err
	}
	p.Linked = linked
	return p, nil
}

// ParseDeck splits a single-deck response.
func ParseDeck(body []byte) (*Page, error) {
	var rd rawDeckResponse
	if err := json.Unmarshal(body, &rd); err != nil {
		return nil, fmt.Errorf("mv: decode deck: %w", err)
	}
	deck, err := parseDeck(rd.Data)
	if err != nil {
		return nil, err
	}
	linked, err := parseLinked(rd.Linked)
	if err != nil {
		return nil, err
	}
	return &Page{Decks: []Deck{deck}, Linked: linked}, nil
}

func parseDeck(raw jsontext.Value) (Deck, error) {
	var h deckHead
	if err := json.Unmarshal(raw, &h); err != nil {
		return Deck{}, fmt.Errorf("mv: decode deck head: %w", err)
	}
	if h.ID == "" {
		return Deck{}, fmt.Errorf("mv: deck without an id: %.200s", raw)
	}
	return Deck{ID: h.ID, Expansion: h.Expansion, CardIDs: h.Links.Cards, Raw: raw}, nil
}

// ParseDeckObject reads one deck object, as a raw record stores it.
func ParseDeckObject(raw jsontext.Value) (Deck, error) {
	return parseDeck(raw)
}

func parseLinked(in map[string][]jsontext.Value) (map[string][]Object, error) {
	out := make(map[string][]Object, len(in))
	for kind, objs := range in {
		for _, raw := range objs {
			var h objectHead
			if err := json.Unmarshal(raw, &h); err != nil {
				return nil, fmt.Errorf("mv: decode linked %s: %w", kind, err)
			}
			out[kind] = append(out[kind], Object{ID: h.ID, Raw: raw})
		}
	}
	return out, nil
}

// Cards returns the head of each linked card object, by ID.
func (p *Page) Cards() (map[string]CardHead, error) {
	out := map[string]CardHead{}
	for _, o := range p.Linked["cards"] {
		var h CardHead
		if err := json.Unmarshal(o.Raw, &h); err != nil {
			return nil, fmt.Errorf("mv: decode card head: %w", err)
		}
		out[h.ID] = h
	}
	return out, nil
}
