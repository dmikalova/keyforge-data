// Package distill makes the published dataset from the raw store. It is a
// pure function of its input: the English card objects, the list of card
// images in the public bucket, and the overrides. It does not fetch.
//
// These are first-pass rules. They come from the data report of the sets
// that the crawl has read. A card that the rules do not recognize becomes a
// Problem, and distillation does not fail.
package distill

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/dmikalova/keyforge-data/card"
	"github.com/dmikalova/keyforge-data/derive"
	"github.com/dmikalova/keyforge-data/query"
	"github.com/dmikalova/keyforge-data/sets"
)

// mvCard is the part of a Master Vault card object that distillation reads.
type mvCard struct {
	ID         string         `json:"id"`
	Title      string         `json:"card_title"`
	House      string         `json:"house"`
	Type       string         `json:"card_type"`
	Text       string         `json:"card_text"`
	Traits     *string        `json:"traits"`
	Amber      int            `json:"amber"`
	Power      jsontext.Value `json:"power"`
	Armor      jsontext.Value `json:"armor"`
	Rarity     string         `json:"rarity"`
	Flavor     *string        `json:"flavor_text"`
	Number     string         `json:"card_number"`
	Expansion  int            `json:"expansion"`
	IsMaverick bool           `json:"is_maverick"`
	IsAnomaly  bool           `json:"is_anomaly"`
	IsEnhanced bool           `json:"is_enhanced"`
	FrontImage string         `json:"front_image"`
}

// Input is everything that distillation reads.
type Input struct {
	// Cards are the English card objects as Master Vault sent them.
	Cards [][]byte
	// Images are the object names under images/cards/ in the public bucket,
	// for example "massmutation/125.webp?v=1764000000000000". The version
	// query is optional.
	Images []string
	// Overrides are the curated corrections, by set slug.
	Overrides map[string]Override
	// Now is the generated time of the dataset.
	Now time.Time
}

// Override is the curated data for one set (overrides/<set>.json).
type Override struct {
	// Kinds maps a Master Vault house of houseless cards to its kind label,
	// for example "Skybeast" to "skybeast".
	Kinds map[string]string `json:"kinds,omitempty"`
	// Aliases maps a card title to extra names for name search.
	Aliases map[string][]string `json:"aliases,omitempty"`
	// Cards maps a card number to corrections for that printing's card.
	Cards map[string]CardFix `json:"cards,omitempty"`
	// Threads are curated threads of related cards. Only the file
	// overrides/_threads.json has them.
	Threads []Thread `json:"threads,omitempty"`
}

// CardFix corrects a Master Vault mistake.
type CardFix struct {
	Power *card.Power `json:"power,omitempty"`
	Armor *int        `json:"armor,omitempty"`
	Note  string      `json:"note,omitempty"`
}

// Problem is a card that the rules do not recognize, or data that is
// missing. The user fixes a problem with an override or new data.
type Problem struct {
	Kind   string `json:"kind"`
	Set    string `json:"set,omitempty"`
	Number string `json:"number,omitempty"`
	Title  string `json:"title,omitempty"`
	Detail string `json:"detail,omitempty"`
}

// Problem kinds.
const (
	ProblemUnknownSet      = "unknown set"
	ProblemNoPower         = "creature without power"
	ProblemMissingImage    = "missing image"
	ProblemUnlabeledKind   = "unlabeled houseless kind"
	ProblemDuplicateCard   = "same name and house twice in a set"
	ProblemUnknownType     = "unknown card type"
	ProblemBadObject       = "unreadable card object"
	ProblemGiganticNoBase  = "gigantic without a base half"
	ProblemOnlyMavericks   = "printing with only mavericks"
	ProblemMissingGigantic = "missing gigantic image"
)

// referenceHouses are the Master Vault pseudo-houses of reference cards.
var referenceHouses = map[string]bool{"archonpower": true, "prophecy": true, "thetide": true}

// realHouses are the houses that a deck can have, by normalized name.
var realHouses = map[string]bool{
	"brobnar": true, "dis": true, "ekwidon": true, "elders": true, "geistoid": true,
	"ironyxrebels": true, "keyraken": true, "logos": true, "mars": true, "ouboros": true,
	"redemption": true, "sanctum": true, "saurian": true, "shadows": true, "skyborn": true,
	"staralliance": true, "unfathomable": true, "untamed": true,
}

// Flags that distillation sets.
const (
	FlagReference     = "reference"
	FlagTokenCreature = "tokencreature"
	FlagGigantic      = "gigantic"
	FlagEvilTwin      = "eviltwin"
	// FlagHouseless marks every card without a house that is not a reference
	// card: anomalies, multi-house cards, and the houseless kinds.
	FlagHouseless = "houseless"
)

// printingKey is one printing: a number in a set with one title.
type printingKey struct {
	exp    int
	number string
	title  string
}

// Run makes the dataset and the list of problems.
func Run(in Input) (card.Dataset, []Problem) {
	r := &run{in: in, images: indexImages(in.Images)}
	objs := r.parse(in.Cards)
	groups := map[printingKey][]mvCard{}
	for _, c := range objs {
		if _, ok := sets.ByExpansion(c.Expansion); !ok {
			r.problem(Problem{
				Kind: ProblemUnknownSet, Number: c.Number, Title: c.Title,
				Detail: fmt.Sprintf("expansion %d", c.Expansion),
			})
			continue
		}
		k := printingKey{exp: c.Expansion, number: c.Number, title: c.Title}
		groups[k] = append(groups[k], c)
	}
	cards := map[string]*card.Card{}
	keys := slices.SortedFunc(maps.Keys(groups), func(a, b printingKey) int {
		return cmp.Or(
			cmp.Compare(setIndex(a.exp), setIndex(b.exp)),
			strings.Compare(a.number, b.number),
			strings.Compare(a.title, b.title),
		)
	})
	for _, k := range keys {
		r.add(cards, k, groups[k])
	}
	return r.dataset(cards), r.problems
}

type run struct {
	in       Input
	images   map[string][]string
	problems []Problem
}

func (r *run) problem(p Problem) { r.problems = append(r.problems, p) }

// add merges one printing into its card.
func (r *run) add(cards map[string]*card.Card, k printingKey, objs []mvCard) {
	set, _ := sets.ByExpansion(k.exp)
	ov := r.in.Overrides[set.Slug]
	base, flags, ok := r.pick(set, objs)
	if !ok {
		return
	}
	house, kindFlags := r.house(set, base, objs, ov)
	flags = append(flags, kindFlags...)
	group := houselessGroup(house, base, objs, kindFlags)
	typ := strings.ToLower(base.Type)
	if strings.Contains(typ, "token") {
		typ = "creature"
		flags = append(flags, FlagTokenCreature)
	}
	// Master Vault gives an evil twin the type "Evil Twin Creature". The
	// card is a creature with the flag eviltwin, and a separate card from
	// the normal card with the same name and house.
	if rest, ok := strings.CutPrefix(typ, "evil twin "); ok {
		typ = rest
		flags = append(flags, FlagEvilTwin)
	}
	switch {
	case slices.Contains([]string{"action", "artifact", "creature", "upgrade"}, typ),
		slices.Contains(flags, FlagReference):
	default:
		r.problem(
			Problem{
				Kind:   ProblemUnknownType,
				Set:    set.Slug,
				Number: k.number,
				Title:  k.title,
				Detail: base.Type,
			},
		)
	}

	id := query.Normalize(k.title) + "-" + cmp.Or(house, "none")
	if slices.Contains(flags, FlagEvilTwin) {
		id += "-eviltwin"
	}
	c, seen := cards[id]
	if seen &&
		slices.ContainsFunc(c.Printings, func(p card.Printing) bool { return p.Set == set.Slug }) {
		// The same name and house at a second number in one set: an evil
		// twin or a Master Vault mistake. Keep both as separate cards.
		r.problem(
			Problem{Kind: ProblemDuplicateCard, Set: set.Slug, Number: k.number, Title: k.title},
		)
		id += "-" + set.Slug + "-" + k.number
		c, seen = nil, false
	}
	if !seen {
		c = &card.Card{ID: id}
		cards[id] = c
	}
	// The newest printing gives the card fields.
	c.Title = k.title
	c.Folded = query.Fold(k.title)
	c.House = house
	c.Group = group
	c.Type = typ
	c.Traits = traits(base.Traits)
	c.Text = derive.Display(base.Text)
	c.Aember = base.Amber
	c.Keywords = nilIfEmpty(derive.CardKeywords(base.Text))
	c.Enhance = derive.Enhance(base.Text)
	c.Abilities = derive.CardAbilities(base.Text)
	c.Flags = sortedUnique(append(c.Flags, flags...))
	c.Aliases = sortedUnique(append(c.Aliases, aliases(k.title, ov)...))
	c.Power, c.Armor = nil, 0
	if typ == "creature" {
		c.Power, c.Armor = r.stats(set, k, base, ov)
	}

	p := card.Printing{
		Set:     set.Slug,
		Number:  k.number,
		Rarity:  rarity(base),
		Anomaly: base.IsAnomaly,
		Image:   r.printingImage(set, k, base, objs, slices.Contains(flags, FlagGigantic)),
	}
	if base.Flavor != nil {
		p.Flavor = strings.TrimSpace(*base.Flavor)
	}
	for _, o := range objs {
		p.MVIDs = append(p.MVIDs, o.ID)
	}
	slices.Sort(p.MVIDs)
	c.Printings = append(c.Printings, p)
}

// pick returns the object that gives the card fields, and the flags that
// the objects show. It prefers a gigantic base half, then an object that is
// not enhanced.
func (r *run) pick(set sets.Set, objs []mvCard) (mvCard, []string, bool) {
	var flags []string
	gigantic := slices.ContainsFunc(objs, func(o mvCard) bool {
		return strings.Contains(strings.ToLower(o.Type), "gigantic")
	})
	cands := objs
	if gigantic {
		flags = append(flags, FlagGigantic)
		cands = filter(
			objs,
			func(o mvCard) bool { return strings.Contains(strings.ToLower(o.Type), "base") },
		)
		if len(cands) == 0 {
			r.problem(
				Problem{
					Kind:   ProblemGiganticNoBase,
					Set:    set.Slug,
					Number: objs[0].Number,
					Title:  objs[0].Title,
				},
			)
			return mvCard{}, nil, false
		}
	}
	base := slices.MinFunc(cands, func(a, b mvCard) int {
		return cmp.Or(compareBool(a.IsEnhanced, b.IsEnhanced), strings.Compare(a.ID, b.ID))
	})
	if gigantic {
		base.Type = "Creature"
	}
	return base, flags, true
}

// house returns the card's house and the flags that the house gives.
func (r *run) house(set sets.Set, base mvCard, objs []mvCard, ov Override) (string, []string) {
	houses := map[string]bool{}
	for _, o := range objs {
		houses[query.Normalize(o.House)] = true
	}
	h := query.Normalize(base.House)
	switch {
	case len(houses) > 1:
		// Master Vault prints the card in the house of the deck: an anomaly,
		// or a card such as Dark Æmber Vault. The card is houseless.
		return "", []string{FlagHouseless}
	case referenceHouses[h]:
		return "", []string{FlagReference}
	case realHouses[h]:
		return h, nil
	}
	if label, ok := ov.Kinds[base.House]; ok {
		return "", []string{query.Normalize(label), FlagHouseless}
	}
	r.problem(Problem{
		Kind: ProblemUnlabeledKind, Set: set.Slug, Number: base.Number, Title: base.Title,
		Detail: "Master Vault house " + strconv.Quote(base.House),
	})
	return "", []string{h, FlagHouseless}
}

// stats returns the power and armor of a creature.
func (r *run) stats(set sets.Set, k printingKey, base mvCard, ov Override) (*card.Power, int) {
	fix := ov.Cards[k.number]
	armor := 0
	if n, ok := number(base.Armor); ok {
		armor = n
	}
	if fix.Armor != nil {
		armor = *fix.Armor
	}
	if fix.Power != nil {
		return fix.Power, armor
	}
	s := strings.Trim(strings.TrimSpace(string(base.Power)), `"`)
	if strings.EqualFold(s, "x") {
		return &card.Power{X: true}, armor
	}
	if n, ok := number(base.Power); ok {
		return &card.Power{N: n}, armor
	}
	r.problem(Problem{
		Kind: ProblemNoPower, Set: set.Slug, Number: k.number, Title: k.title,
		Detail: "power " + cmp.Or(s, "null"),
	})
	return nil, armor
}

// image returns the image path of a printing under images/cards/, or empty.
// The path is <set>/<number>.webp, or <set>/<number>-<name>.webp when two
// cards in a set share a number.
func (r *run) image(set sets.Set, k printingKey, front string) string {
	names := r.images[set.Slug+"/"+k.number]
	want := set.Slug + "/" + k.number + "-" + Slug(k.title) + "."
	for _, n := range names {
		if strings.HasPrefix(n, want) {
			return n
		}
	}
	for _, n := range names {
		if strings.HasPrefix(n, set.Slug+"/"+k.number+".") {
			return n
		}
	}
	r.problem(Problem{
		Kind: ProblemMissingImage, Set: set.Slug, Number: k.number, Title: k.title, Detail: front,
	})
	return ""
}

// dataset sorts the cards and makes the envelope.
func (r *run) dataset(cards map[string]*card.Card) card.Dataset {
	out := make([]card.Card, 0, len(cards))
	used := map[string]bool{}
	for _, c := range cards {
		for _, p := range c.Printings {
			used[p.Set] = true
		}
		out = append(out, *c)
	}
	linkCards(out, r.in.Overrides)
	slices.SortFunc(out, compareCards)
	var ss []sets.Set
	for _, s := range sets.All {
		if used[s.Slug] {
			ss = append(ss, s)
		}
	}
	slices.SortFunc(r.problems, func(a, b Problem) int {
		return cmp.Or(
			strings.Compare(a.Kind, b.Kind), strings.Compare(a.Set, b.Set),
			strings.Compare(a.Number, b.Number), strings.Compare(a.Title, b.Title),
		)
	})
	d := card.Dataset{Generated: r.in.Now.UTC(), Sets: ss, Cards: out}
	b, err := json.Marshal(struct {
		Sets  []sets.Set
		Cards []card.Card
	}{ss, out}, json.Deterministic(true))
	if err == nil {
		sum := sha256.Sum256(b)
		d.Version = hex.EncodeToString(sum[:6])
	}
	return d
}

// compareCards is the default order: house, then name. Cards without a
// house sort last, in the order of card.HouselessGroups.
func compareCards(a, b card.Card) int {
	rank := func(c card.Card) int {
		if c.House != "" {
			return -1
		}
		if i := slices.Index(card.HouselessGroups, c.Group); i >= 0 {
			return i
		}
		return len(card.HouselessGroups)
	}
	return cmp.Or(
		cmp.Compare(rank(a), rank(b)),
		strings.Compare(a.House, b.House),
		strings.Compare(a.Folded, b.Folded),
		strings.Compare(a.ID, b.ID),
	)
}

func indexImages(names []string) map[string][]string {
	out := map[string][]string{}
	for _, n := range names {
		dir, file, ok := strings.Cut(n, "/")
		if !ok {
			continue
		}
		num, _, _ := strings.Cut(file, ".")
		num, _, _ = strings.Cut(num, "-")
		out[dir+"/"+num] = append(out[dir+"/"+num], n)
	}
	return out
}

// Slug returns the folded title with runs of other characters as one
// hyphen: "It’s Coming..." becomes "its-coming".
func Slug(s string) string {
	s = strings.NewReplacer("'", "", "’", "").Replace(query.Fold(s))
	var b strings.Builder
	dash := false
	for _, r := range s {
		if r < 128 && (r >= 'a' && r <= 'z' || r >= '0' && r <= '9') {
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			b.WriteRune(r)
			dash = false
			continue
		}
		dash = true
	}
	return b.String()
}

// leet maps digits to the letters that they stand for in robot names.
var leet = strings.NewReplacer("0", "o", "1", "i", "3", "e", "4", "a", "5", "s", "7", "t")

// aliases returns the generic alias of a title with digits and the curated
// aliases of the overrides.
func aliases(title string, ov Override) []string {
	var out []string
	folded := query.Fold(title)
	if strings.ContainsAny(folded, "0123456789") {
		if a := leet.Replace(folded); a != folded {
			out = append(out, a)
		}
	}
	for _, a := range ov.Aliases[title] {
		out = append(out, query.Fold(a))
	}
	return out
}

func traits(s *string) []string {
	if s == nil {
		return nil
	}
	var out []string
	for t := range strings.SplitSeq(*s, "•") {
		if t = strings.ToLower(strings.TrimSpace(t)); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// rarity returns the printed rarity. Master Vault calls the rarity of the
// fixed cards of a deck "FIXED": the Horsemen, the Mega creatures, Velum,
// the anomalies. The cards print the special rarity, and the anomalies
// print their own anomaly rarity.
func rarity(c mvCard) string {
	switch {
	case c.Rarity != "FIXED":
		return c.Rarity
	case c.IsAnomaly:
		return "Anomaly"
	}
	return "Special"
}

func number(v jsontext.Value) (int, bool) {
	s := strings.Trim(strings.TrimSpace(string(v)), `"`)
	n, err := strconv.Atoi(s)
	return n, err == nil
}

func setIndex(exp int) int {
	return slices.IndexFunc(sets.All, func(s sets.Set) bool { return s.Expansion == exp })
}

func compareBool(a, b bool) int {
	switch {
	case a == b:
		return 0
	case a:
		return 1
	}
	return -1
}

func filter[T any](xs []T, f func(T) bool) []T {
	var out []T
	for _, x := range xs {
		if f(x) {
			out = append(out, x)
		}
	}
	return out
}

func sortedUnique(xs []string) []string {
	if len(xs) == 0 {
		return nil
	}
	slices.Sort(xs)
	return slices.Compact(xs)
}

func nilIfEmpty(m map[string]int) map[string]int {
	if len(m) == 0 {
		return nil
	}
	return m
}

// Master Vault keeps the anomalies of Worlds Collide in a separate expansion
// (453). Players do not see that expansion: the anomalies come in Worlds
// Collide decks. The data report shows that every expansion 453 object is
// in a Worlds Collide deck.
const (
	anomalyExpansion  = 453
	anomalyDeckSetExp = 452
)

// parse reads the card objects, drops the mavericks, and moves the
// anomalies of expansion 453. An anomaly with a printing in a playable set
// uses that printing, so its expansion 453 objects are dropped. Other
// anomalies become printings of the set whose decks have them.
// A printing that has only maverick objects is kept, in the house of the
// card's other printings (see maverickOnly).
func (r *run) parse(raws [][]byte) []mvCard {
	var objs, mavericks []mvCard
	playable := map[string]bool{}
	for _, b := range raws {
		var c mvCard
		if err := json.Unmarshal(b, &c); err != nil {
			r.problem(Problem{Kind: ProblemBadObject, Detail: err.Error()})
			continue
		}
		if c.IsMaverick {
			mavericks = append(mavericks, c)
			continue
		}
		if c.Expansion != anomalyExpansion {
			playable[query.Normalize(c.Title)] = true
		}
		objs = append(objs, c)
	}
	objs = append(objs, r.maverickOnly(objs, mavericks)...)
	out := objs[:0]
	for _, c := range objs {
		if c.Expansion == anomalyExpansion {
			if playable[query.Normalize(c.Title)] {
				continue
			}
			c.Expansion = anomalyDeckSetExp
			c.IsAnomaly = true
		}
		out = append(out, c)
	}
	return out
}

// maverickOnly returns the printings that Master Vault sends only as
// mavericks, moved to the card's own house. Some cards are always mavericks
// (Exchange Officer in Vault Masters 2023), so no deck has the printing in
// its own house. The card's other printings give the house. Other mavericks
// are copies of a printing that the gallery already has, and are dropped.
func (r *run) maverickOnly(objs, mavericks []mvCard) []mvCard {
	type key struct {
		exp           int
		number, title string
	}
	have := map[key]bool{}
	houses := map[string]map[string]bool{}
	for _, c := range objs {
		have[key{c.Expansion, c.Number, c.Title}] = true
		t := query.Normalize(c.Title)
		if houses[t] == nil {
			houses[t] = map[string]bool{}
		}
		houses[t][c.House] = true
	}
	var out []mvCard
	missing := map[key]bool{}
	for _, c := range mavericks {
		k := key{c.Expansion, c.Number, c.Title}
		if have[k] {
			continue
		}
		hs := houses[query.Normalize(c.Title)]
		if len(hs) != 1 {
			if !missing[k] {
				missing[k] = true
				set, _ := sets.ByExpansion(c.Expansion)
				r.problem(Problem{
					Kind: ProblemOnlyMavericks, Set: set.Slug, Number: c.Number, Title: c.Title,
					Detail: fmt.Sprintf("%d houses in other printings", len(hs)),
				})
			}
			continue
		}
		for h := range hs {
			c.House = h
		}
		c.IsMaverick = false
		out = append(out, c)
	}
	return out
}

// printingImage returns the image path of a printing. A gigantic creature
// uses the composite image <set>/<number>-gigantic.webp. The composite has
// a new path, because the bucket caches each image path for a year.
func (r *run) printingImage(
	set sets.Set,
	k printingKey,
	base mvCard,
	objs []mvCard,
	gigantic bool,
) string {
	if !gigantic {
		return r.image(set, k, base.FrontImage)
	}
	want := set.Slug + "/" + k.number + "-gigantic."
	for _, n := range r.images[set.Slug+"/"+k.number] {
		if strings.HasPrefix(n, want) {
			return n
		}
	}
	art := ""
	for _, o := range objs {
		if strings.Contains(strings.ToLower(o.Type), "art") && (art == "" || !o.IsEnhanced) {
			art = o.FrontImage
		}
	}
	r.problem(Problem{
		Kind: ProblemMissingGigantic, Set: set.Slug, Number: k.number, Title: k.title,
		Detail: base.FrontImage + " " + art,
	})
	return ""
}

// houselessGroup returns the sort group of a card without a house: the
// reference kind (archonpower, prophecy, thetide), "anomaly", "multihouse"
// for a card that Master Vault prints in the house of the deck, or the
// houseless kind label.
func houselessGroup(house string, base mvCard, objs []mvCard, kindFlags []string) string {
	if house != "" {
		return ""
	}
	if slices.Contains(kindFlags, FlagReference) {
		return query.Normalize(base.House)
	}
	if base.IsAnomaly {
		return "anomaly"
	}
	houses := map[string]bool{}
	for _, o := range objs {
		houses[o.House] = true
	}
	if len(houses) > 1 {
		return "multihouse"
	}
	for _, f := range kindFlags {
		if f != FlagHouseless {
			return f
		}
	}
	return "houseless"
}
