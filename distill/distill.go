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
	"unicode"

	"github.com/dmikalova/keyforge-data/card"
	"github.com/dmikalova/keyforge-data/derive"
	"github.com/dmikalova/keyforge-data/overlay"
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
	// fromAnomalies is true for an object of the anomaly expansion (453). The
	// card takes the house of the deck, so it is houseless.
	fromAnomalies bool
	// playableHouse is the house of the card's printing in a playable set,
	// for an anomaly object of a card that a later set prints as a normal
	// card. The anomaly printing joins that card.
	playableHouse string
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
	// Artists are the artists of the printings, by set slug and collector
	// number (the "artists" section of overrides/<set>.json).
	Artists map[string]map[string]string
	// ArtistAliases maps another spelling of an artist to the canonical
	// name (overrides/_artists.json). It is optional.
	ArtistAliases map[string]string
	// DeckSets maps a card object ID to the expansions of the stored decks
	// that have it. Anomalies use it to find their set.
	DeckSets map[string][]int
	// Decks maps a card object ID to the IDs of the decks that have it. It
	// is optional. Problems use it to name the decks with the card.
	Decks map[string][]string
	// Now is the generated time of the dataset.
	Now time.Time
}

// Override is one override file (package overlay).
type Override = overlay.File

// CardFix corrects a Master Vault mistake.
type CardFix = overlay.CardFix

// Problem is a card that the rules do not recognize, or data that is
// missing. The user fixes a problem with an override or new data.
type Problem struct {
	Kind   string `json:"kind"`
	Set    string `json:"set,omitempty"`
	Number string `json:"number,omitempty"`
	Title  string `json:"title,omitempty"`
	Detail string `json:"detail,omitempty"`
	// Decks are the Master Vault decks that have the card of the problem.
	Decks []string `json:"decks,omitempty"`
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
	ProblemForeignText     = "flavor text not in English"
	ProblemAnomalyNoSet    = "anomaly without a set"
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
	// card: anomalies and the houseless kinds.
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
	evilTwinRarities(cards)
	d := r.dataset(cards)
	r.addDecks(groups)
	return d, r.problems
}

type run struct {
	in       Input
	images   map[string][]string
	problems []Problem
	// labels are the names for people that the Master Vault data gives, by
	// kind ("houses", "groups", "types") and value.
	labels map[string]map[string]string
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
	group := houselessGroup(house, base, kindFlags)
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
	// A reference card (an archon power, a prophecy, the tide) is not one of
	// the four card types. Its kind is in the card group.
	if slices.Contains(flags, FlagReference) {
		typ = "reference"
	}
	if strings.EqualFold(base.Type, typ) {
		r.label("types", typ, base.Type)
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
		c.Overlay = overlaid(ov.Cards[k.number])
	}

	p := card.Printing{
		Set:     set.Slug,
		Number:  k.number,
		Rarity:  rarity(base),
		Anomaly: base.IsAnomaly,
		Image:   r.printingImage(set, k, base, objs, slices.Contains(flags, FlagGigantic)),
		Artist:  r.in.Artists[set.Slug][k.number],
		// dataset keeps the text only where it differs from the card text.
		Text: derive.Display(base.Text),
	}
	if base.Flavor != nil {
		p.Flavor = derive.Flavor(*base.Flavor)
		if foreign(p.Flavor) {
			// Master Vault has text in another language in the English data.
			r.problem(
				Problem{
					Kind:   ProblemForeignText,
					Set:    set.Slug,
					Number: k.number,
					Title:  k.title,
					Detail: "Master Vault's English data has this text, which the printed card does not have. The site drops it: " + p.Flavor,
				},
			)
			p.Flavor = ""
		}
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
	// A number prefix in the overrides names a kind of houseless cards, such
	// as the Skybeasts (S01) of Æmber Skies. Master Vault prints them in the
	// house of the deck, so a card seen in one deck only has a house too.
	for prefix, label := range ov.KindPrefixes {
		if strings.HasPrefix(base.Number, prefix) {
			r.label("groups", query.Normalize(label), label)
			return "", []string{query.Normalize(label), FlagHouseless}
		}
	}
	switch {
	case base.playableHouse != "":
		return base.playableHouse, nil
	case base.fromAnomalies:
		return "", []string{FlagHouseless}
	case len(houses) > 1:
		// Master Vault prints the card in the house of the deck: an anomaly,
		// or a card such as Dark Æmber Vault. The card is houseless.
		return "", []string{FlagHouseless}
	case referenceHouses[h]:
		r.label("groups", h, base.House)
		return "", []string{FlagReference}
	case realHouses[h]:
		r.label("houses", h, base.House)
		return h, nil
	}
	if label, ok := ov.Kinds[base.House]; ok {
		r.label("groups", query.Normalize(label), base.House)
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
	for i := range out {
		for j := range out[i].Printings {
			if p := &out[i].Printings[j]; sameText(p.Text, out[i].Text) {
				p.Text = ""
			}
		}
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
	d := card.Dataset{
		Generated: r.in.Now.UTC(), Sets: ss, Cards: out,
		ArtistAliases: usedAliases(r.in.ArtistAliases, out),
		Labels:        r.datasetLabels(),
	}
	b, err := json.Marshal(struct {
		Sets    []sets.Set
		Cards   []card.Card
		Aliases map[string]string
		Labels  map[string]map[string]string
	}{ss, out, d.ArtistAliases, d.Labels}, json.Deterministic(true))
	if err == nil {
		sum := sha256.Sum256(b)
		d.Version = hex.EncodeToString(sum[:6])
	}
	return d
}

// usedAliases returns the aliases whose canonical name is the artist of a
// printing, or nil.
func usedAliases(aliases map[string]string, cards []card.Card) map[string]string {
	used := map[string]bool{}
	for _, c := range cards {
		for _, p := range c.Printings {
			used[p.Artist] = true
		}
	}
	var out map[string]string
	for variant, name := range aliases {
		if used[name] && variant != name {
			if out == nil {
				out = map[string]string{}
			}
			out[variant] = name
		}
	}
	return out
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
	case c.fromAnomalies:
		// Master Vault gives most anomalies the rarity Special. The printed
		// card has the anomaly rarity icon.
		return "Anomaly"
	case c.Rarity == "Token":
		// A token creature is not in a deck. Its rarity names the kind.
		return "Token Creature"
	case c.Rarity == "FIXED" && c.IsAnomaly:
		return "Anomaly"
	case slices.Contains(specialRarities, c.Rarity):
		return "Special"
	}
	return c.Rarity
}

// specialRarities are the Master Vault rarities of the sets before Winds of
// Exchange that the later sets call Special: FIXED for the cards that come
// in a fixed group, Variant for the variant families, and The Tide. An evil
// twin's rarity "Evil Twin" becomes the rarity of its normal form
// (evilTwinRarities).
var specialRarities = []string{"FIXED", "Variant", "The Tide"}

// evilTwinRarity is the Master Vault rarity of every evil twin.
const evilTwinRarity = "Evil Twin"

// evilTwinRarities gives each evil twin printing the rarity of its normal
// form: the normal card's printing in the same set, or else the normal
// card's newest printing. The flag eviltwin marks the evil twin.
func evilTwinRarities(cards map[string]*card.Card) {
	for id, c := range cards {
		base, ok := strings.CutSuffix(id, "-eviltwin")
		if !ok {
			continue
		}
		normal := cards[base]
		for i := range c.Printings {
			p := &c.Printings[i]
			if p.Rarity != evilTwinRarity {
				continue
			}
			p.Rarity = "Special"
			if normal == nil || len(normal.Printings) == 0 {
				continue
			}
			p.Rarity = normal.Printings[len(normal.Printings)-1].Rarity
			for _, np := range normal.Printings {
				if np.Set == p.Set {
					p.Rarity = np.Rarity
				}
			}
		}
	}
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

// anomalyExpansion is the Master Vault expansion of the anomalies. Players
// do not see it: an anomaly comes in the decks of other sets, and later sets
// can reprint it as a normal card.
const anomalyExpansion = 453

// parse reads the card objects, drops the mavericks, and moves the
// anomalies of expansion 453. An anomaly becomes a printing of the oldest
// set whose stored decks have it (Input.DeckSets). An anomaly that a
// playable set also prints, such as Valoocanth, keeps that anomaly printing
// on the card of the playable printing. Nothing is coded for each anomaly.
// A printing that has only maverick objects is kept, in the house of the
// card's other printings (see maverickOnly).
func (r *run) parse(raws [][]byte) []mvCard {
	var objs, mavericks []mvCard
	playable := map[string]string{}
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
			playable[query.Normalize(c.Title)] = query.Normalize(c.House)
		}
		objs = append(objs, c)
	}
	objs = append(objs, r.maverickOnly(objs, mavericks)...)
	reported := map[string]bool{}
	anomalySets := r.anomalySets(objs)
	out := objs[:0]
	for _, c := range objs {
		if c.Expansion == anomalyExpansion {
			house, isPlayable := playable[query.Normalize(c.Title)]
			exp, ok := anomalySets[c.Number]
			if isPlayable {
				// The playable printing makes the card. Without a stored
				// deck, the anomaly printing has no set and is left out.
				if !ok {
					continue
				}
				c.playableHouse = house
			}
			if !ok {
				if !reported[c.Number] {
					reported[c.Number] = true
					r.problem(Problem{
						Kind: ProblemAnomalyNoSet, Number: c.Number, Title: c.Title,
						Detail: "no printing in a playable set, and no stored deck has it",
					})
				}
				continue
			}
			c.Expansion = exp
			c.IsAnomaly = true
			c.fromAnomalies = true
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
// reference kind (archonpower, prophecy, thetide), "anomaly", or the houseless
// kind label. A card that Master Vault prints in the house of the deck, such
// as Dark Æmber Vault, is houseless.
func houselessGroup(house string, base mvCard, kindFlags []string) string {
	if house != "" {
		return ""
	}
	if slices.Contains(kindFlags, FlagReference) {
		return query.Normalize(base.House)
	}
	if base.IsAnomaly {
		return "anomaly"
	}
	for _, f := range kindFlags {
		if f != FlagHouseless {
			return f
		}
	}
	return "houseless"
}

// foreign reports whether text has letters from a script that English card
// text does not use, for example Hangul.
func foreign(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) && !unicode.In(r, unicode.Latin, unicode.Greek) {
			return true
		}
	}
	return false
}

// addDecks adds to each problem the decks that have a card object of the
// problem's printing.
func (r *run) addDecks(groups map[printingKey][]mvCard) {
	if len(r.in.Decks) == 0 {
		return
	}
	for i, p := range r.problems {
		s, ok := sets.BySlug(p.Set)
		if !ok {
			continue
		}
		var decks []string
		for _, o := range groups[printingKey{exp: s.Expansion, number: p.Number, title: p.Title}] {
			decks = append(decks, r.in.Decks[o.ID]...)
		}
		slices.Sort(decks)
		r.problems[i].Decks = slices.Compact(decks)
	}
}

// anomalySets returns, for each anomaly number, the oldest playable set
// whose stored decks have one of its objects.
func (r *run) anomalySets(objs []mvCard) map[string]int {
	out := map[string]int{}
	for _, c := range objs {
		if c.Expansion != anomalyExpansion {
			continue
		}
		for _, exp := range r.in.DeckSets[c.ID] {
			if _, ok := sets.ByExpansion(exp); !ok || exp == anomalyExpansion {
				continue
			}
			if old, ok := out[c.Number]; !ok || setIndex(exp) < setIndex(old) {
				out[c.Number] = exp
			}
		}
	}
	return out
}

// overlaid returns the card fields that a card fix sets: "power", "armor".
func overlaid(fix CardFix) []string {
	var out []string
	if fix.Power != nil {
		out = append(out, "power")
	}
	if fix.Armor != nil {
		out = append(out, "armor")
	}
	return out
}

// label keeps the first name for people that Master Vault gives a value.
func (r *run) label(kind, value, name string) {
	if r.labels == nil {
		r.labels = map[string]map[string]string{}
	}
	if r.labels[kind] == nil {
		r.labels[kind] = map[string]string{}
	}
	if _, ok := r.labels[kind][value]; !ok && value != "" && name != "" {
		r.labels[kind][value] = name
	}
}

// datasetLabels returns the labels of the Master Vault data with the labels
// of overrides/_labels.json over them.
func (r *run) datasetLabels() map[string]map[string]string {
	out := map[string]map[string]string{}
	for kind, m := range r.labels {
		out[kind] = maps.Clone(m)
	}
	for kind, m := range r.in.Overrides[overlay.LabelsFile].Labels {
		if out[kind] == nil {
			out[kind] = map[string]string{}
		}
		maps.Copy(out[kind], m)
	}
	return out
}

// sameText reports whether two card texts differ only in white space: a
// line break in one printing and a space in another.
func sameText(a, b string) bool {
	return strings.Join(strings.Fields(a), " ") == strings.Join(strings.Fields(b), " ")
}
