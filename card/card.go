// Package card is the card model of the dataset: a Card has Printings, and a
// Printing has Localizations. The vocabulary is in CONTEXT.md.
//
// Distillation makes these values from the raw store. The rules that decide
// gigantic halves, houseless kinds, evil twins, and token creatures are not
// written yet: they come from the data report (docs/todo.md, phase 4).
package card

import (
	"bytes"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/dmikalova/keyforge-data/sets"
)

// Card is one playable card identity: a name and a house. An evil twin is a
// separate card.
type Card struct {
	// ID is the stable key: the folded name, the house, and a suffix for an
	// evil twin, as a slug.
	ID string `json:"id"`
	// Title is the English title as printed, with Æ.
	Title string `json:"title"`
	// Folded is the title in lowercase ASCII.
	Folded string `json:"folded"`
	// Aliases are extra folded names that name search matches.
	Aliases []string `json:"aliases,omitempty"`
	// House is the house slug, or empty for a card without a house.
	House string `json:"house,omitempty"`
	// Type is the card type as Master Vault gives it, in lowercase.
	Type string `json:"type"`
	// Traits are the traits in lowercase.
	Traits []string `json:"traits,omitempty"`
	// Text is the English card text with icons spelled as words ("Gain 1 Æmber").
	Text string `json:"text,omitempty"`
	// Power is the power of a creature.
	Power *Power `json:"power,omitempty"`
	// Armor is the armor of a creature.
	Armor int `json:"armor,omitempty"`
	// Aember is the printed bonus aember.
	Aember int `json:"aember,omitempty"`
	// Keywords are the keywords that start the text, with their numbers.
	Keywords map[string]int `json:"keywords,omitempty"`
	// Abilities are the ability triggers of the card text, for example "play"
	// and "reap". "After Fight" counts as "fight".
	Abilities []string `json:"abilities,omitempty"`
	// Enhance counts the icons that the card's Enhance keywords give.
	Enhance map[string]int `json:"enhance,omitempty"`
	// Flags are the "is:" flags: reference, tokencreature, gigantic,
	// eviltwin, and the houseless kind labels.
	Flags []string `json:"flags,omitempty"`
	// Mentions are the other cards that the text or the flavor text of a
	// printing names. The site links each title in the text to the card.
	Mentions []Mention `json:"mentions,omitempty"`
	// Related are the threads of related cards that the card is in: an evil
	// twin pair, a named family such as the Four Horsemen, or the cards that
	// mention this card.
	Related []Related `json:"related,omitempty"`
	// Group is the sort group of a card without a house, for example
	// "anomaly" or "skybeast". The default sort puts the cards without a
	// house in the order of HouselessGroups.
	Group string `json:"group,omitempty"`
	// VariantGroup links the members of one variant family.
	VariantGroup string `json:"variant_group,omitempty"`
	// Printings are the card's printings, oldest set first.
	Printings []Printing `json:"printings"`
}

// Printing is one appearance of a card in one set.
type Printing struct {
	// Set is the set slug.
	Set string `json:"set"`
	// Number is the collector number as printed, for example "001" or "S01".
	Number string `json:"number"`
	// Rarity is the printed rarity as Master Vault gives it.
	Rarity string `json:"rarity"`
	// Anomaly is true for a printing with the anomaly frame.
	Anomaly bool `json:"anomaly,omitempty"`
	// Image is the image path under the public bucket's images/cards/.
	Image string `json:"image,omitempty"`
	// MVIDs are the Master Vault card object IDs of the printing.
	MVIDs []string `json:"mv_ids"`
	// Localizations are the printing's texts by language. English is in the
	// card and is not repeated here.
	Localizations map[string]Localization `json:"localizations,omitempty"`
	// Flavor is the English flavor text.
	Flavor string `json:"flavor,omitempty"`
}

// Localization is the text of a printing in one language.
type Localization struct {
	Title  string `json:"title"`
	Text   string `json:"text,omitempty"`
	Flavor string `json:"flavor,omitempty"`
	Image  string `json:"image,omitempty"`
}

// Power is a number or X.
type Power struct {
	N int
	X bool
}

// String returns the power as printed.
func (p Power) String() string {
	if p.X {
		return "X"
	}
	return strconv.Itoa(p.N)
}

// MarshalText writes the power as a number or "X".
func (p Power) MarshalText() ([]byte, error) {
	return []byte(p.String()), nil
}

// UnmarshalText reads a number or "X".
func (p *Power) UnmarshalText(b []byte) error {
	s := string(bytes.TrimSpace(b))
	if strings.EqualFold(s, "x") {
		*p = Power{X: true}
		return nil
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return err
	}
	*p = Power{N: n}
	return nil
}

// HasFlag reports whether the card has the flag.
func (c *Card) HasFlag(flag string) bool {
	return slices.Contains(c.Flags, flag)
}

// Dataset is the published card data. The site reads it from the public
// bucket and reloads it when it changes, so a new card or set needs no
// deploy.
type Dataset struct {
	// Version changes each time the content changes. It is a hash of the
	// sets and the cards.
	Version string `json:"version"`
	// Generated is when distillation made the dataset.
	Generated time.Time `json:"generated"`
	// Sets are the sets that have cards, oldest first.
	Sets []sets.Set `json:"sets"`
	// Cards are the cards in the default sort order.
	Cards []Card `json:"cards"`
}

// ImageBase is the public URL that Printing.Image and Localization.Image are
// relative to.
const ImageBase = "https://storage.googleapis.com/mklv-keyforge-cards-public/images/cards/"

// DatasetPath is the object path of the dataset in the public bucket.
const DatasetPath = "data/cards.json"

// Mention is a card that another card names in its text or flavor text.
type Mention struct {
	// ID is the card that is named.
	ID string `json:"id"`
	// Title is the name as it is in the text.
	Title string `json:"title"`
}

// Related is one thread of related cards.
type Related struct {
	// Label names the thread for people, for example "Evil twin" or "The
	// Four Horsemen".
	Label string `json:"label"`
	// Cards are the IDs of the other cards in the thread, without this card.
	Cards []string `json:"cards"`
}

// HouselessGroups is the default sort order of the cards without a house,
// after the cards with a house. Reference cards are last.
var HouselessGroups = []string{
	"anomaly", "multihouse", "skybeast", "revenant", "mercenary", "houseless",
	"archonpower", "prophecy", "thetide", "reference",
}
