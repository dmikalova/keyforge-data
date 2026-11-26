package distill

import (
	"cmp"
	"maps"
	"regexp"
	"slices"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/dmikalova/keyforge-data/card"
	"github.com/dmikalova/keyforge-data/overlay"
)

// Thread is a curated thread of related cards (package overlay).
type Thread = overlay.Thread

// Rule is a deck rule of related cards (package overlay).
type Rule = overlay.Rule

// NoteRule is a note about one card (package overlay).
type NoteRule = overlay.NoteRule

// The override files of the threads and the deck rules.
const (
	threadsKey = overlay.ThreadsFile
	rulesKey   = overlay.RulesFile
)

// evilTwinSuffix marks the evil twin in a rule title: "Prof. Garwynne (Evil
// Twin)".
const evilTwinSuffix = " (Evil Twin)"

// linker adds related threads to the cards.
type linker struct {
	cards   []card.Card
	index   map[string]int
	byTitle map[string]string
}

// linkCards fills the mentions and the related threads of the cards.
func linkCards(cards []card.Card, overrides map[string]Override) {
	l := &linker{cards: cards, index: map[string]int{}, byTitle: titleIndex(cards)}
	for i, c := range cards {
		l.index[c.ID] = i
	}
	mentionedBy := l.mentions()
	l.evilTwins()
	l.megas()
	l.threads(overrides[threadsKey].Threads)
	l.rules(overrides[rulesKey].Rules)
	l.notes(overrides[rulesKey].Notes, overrides[threadsKey].Notes)
	for _, id := range slices.Sorted(maps.Keys(mentionedBy)) {
		l.add(id, "Mentioned by", mentionedBy[id])
	}
}

// mentions fills the mentions of each card and returns, for each card, the
// cards that mention it.
func (l *linker) mentions() map[string][]string {
	titles := slices.Collect(maps.Keys(l.byTitle))
	// Longer titles first, so that "Mega Narp" wins over "Narp".
	slices.SortFunc(titles, func(a, b string) int {
		return cmp.Or(cmp.Compare(len(b), len(a)), strings.Compare(a, b))
	})
	mentionedBy := map[string][]string{}
	for i := range l.cards {
		c := &l.cards[i]
		texts := []string{c.Text}
		for _, p := range c.Printings {
			texts = append(texts, p.Flavor)
		}
		seen := map[string]bool{c.ID: true}
		for _, text := range texts {
			for _, f := range findTitles(text, titles, c.Title) {
				id := l.byTitle[f.Title]
				if seen[id] {
					continue
				}
				seen[id] = true
				c.Mentions = append(c.Mentions, card.Mention{ID: id, Title: f.Text})
				mentionedBy[id] = append(mentionedBy[id], c.ID)
			}
		}
	}
	return mentionedBy
}

// add adds a thread to a card. It drops the card itself from the others.
func (l *linker) add(id, label string, others []string) {
	i, ok := l.index[id]
	if !ok {
		return
	}
	others = slices.DeleteFunc(slices.Clone(others), func(o string) bool { return o == id })
	if len(others) == 0 {
		return
	}
	slices.Sort(others)
	l.cards[i].Related = append(
		l.cards[i].Related,
		card.Related{Label: label, Cards: slices.Compact(others)},
	)
}

// evilTwins links each evil twin and the card with the same name and house.
func (l *linker) evilTwins() {
	for _, c := range l.cards {
		if twin, ok := strings.CutSuffix(c.ID, "-eviltwin"); ok {
			l.add(c.ID, "Evil twin", []string{twin})
			l.add(twin, "Evil twin", []string{c.ID})
		}
	}
}

// megas links the Worlds Collide Mega creatures and their base cards: "Mega
// Narp" and "Narp".
func (l *linker) megas() {
	for _, c := range l.cards {
		base, ok := strings.CutPrefix(c.Title, "Mega ")
		if !ok {
			continue
		}
		if id, ok := l.byTitle[base]; ok {
			l.add(c.ID, "Mega creature", []string{id})
			l.add(id, "Mega creature", []string{c.ID})
		}
	}
}

// threads adds the curated threads.
func (l *linker) threads(ths []Thread) {
	for _, th := range ths {
		var ids []string
		for _, t := range th.Titles {
			if id, ok := l.byTitle[t]; ok {
				ids = append(ids, id)
			}
		}
		for _, id := range ids {
			l.add(id, th.Label, ids)
		}
	}
}

// rules adds the deck rules.
func (l *linker) rules(rs []Rule) {
	for _, r := range rs {
		with := l.ids(r.With)
		if len(r.Cards) == 0 {
			for _, id := range with {
				l.addNew(id, r.Label, with)
			}
			continue
		}
		for _, id := range l.ids(r.Cards) {
			l.addNew(id, r.Label, with)
		}
	}
}

// ids returns the card IDs of the titles that it finds.
func (l *linker) ids(titles []string) []string {
	var out []string
	for _, t := range titles {
		if id, ok := l.resolve(t); ok {
			out = append(out, id)
		}
	}
	return out
}

// resolve returns the card ID of a rule title. "Title (Evil Twin)" is the
// evil twin of the card.
func (l *linker) resolve(title string) (string, bool) {
	base, twin := strings.CutSuffix(title, evilTwinSuffix)
	id, ok := l.byTitle[base]
	if !ok || !twin {
		return id, ok
	}
	_, ok = l.index[id+"-eviltwin"]
	return id + "-eviltwin", ok
}

// addNew adds a thread to a card, unless a thread of the card already has
// all the others.
func (l *linker) addNew(id, label string, others []string) {
	i, ok := l.index[id]
	if !ok {
		return
	}
	others = slices.DeleteFunc(slices.Clone(others), func(o string) bool { return o == id })
	for _, r := range l.cards[i].Related {
		if !slices.ContainsFunc(
			others,
			func(o string) bool { return !slices.Contains(r.Cards, o) },
		) {
			return
		}
	}
	l.add(id, label, others)
}

// titleIndex maps each title to one card ID. A title with more than one card
// (an evil twin, or the same name in two houses) maps to the first card that
// is not an evil twin, in ID order.
func titleIndex(cards []card.Card) map[string]string {
	out := map[string]string{}
	for _, c := range cards {
		old, ok := out[c.Title]
		switch {
		case !ok,
			strings.HasSuffix(old, "-eviltwin") && !strings.HasSuffix(c.ID, "-eviltwin"),
			!strings.HasSuffix(c.ID, "-eviltwin") && c.ID < old && !strings.HasSuffix(old, "-eviltwin"):
			out[c.Title] = c.ID
		}
	}
	return out
}

// findTitles returns the titles that the text names, longest first, without
// overlaps and without the card's own title. A match must be a whole name:
// the characters next to it must not be letters or digits. A title of one
// word must not start a sentence, because most one-word titles are also
// common words ("Fear", "Punch").
func findTitles(text string, titles []string, own string) []found {
	if text == "" {
		return nil
	}
	used := make([]bool, len(text))
	// Block the card's own name first, so that a title inside it ("Punch"
	// in "Sucker Punch") does not match.
	for from := 0; own != ""; {
		i := strings.Index(text[from:], own)
		if i < 0 {
			break
		}
		i += from
		for k := i; k < i+len(own); k++ {
			used[k] = true
		}
		from = i + len(own)
	}
	var out []found
	for _, t := range titles {
		if t == own || len(t) < 3 {
			continue
		}
		for from := 0; ; {
			i, end := indexName(text, from, t)
			if i < 0 {
				break
			}
			from = end
			if slices.Contains(used[i:end], true) || !wordEdge(text, i, end) {
				continue
			}
			if !strings.Contains(t, " ") && (sentenceStart(text, i) || inLongerName(text, i, end)) {
				continue
			}
			for k := i; k < end; k++ {
				used[k] = true
			}
			out = append(out, found{Title: t, Text: text[i:end]})
			break
		}
	}
	return out
}

func wordEdge(text string, i, end int) bool {
	if i > 0 {
		r, _ := utf8.DecodeLastRuneInString(text[:i])
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return false
		}
	}
	if end < len(text) {
		r, _ := utf8.DecodeRuneInString(text[end:])
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

// sentenceStart reports whether position i starts a line, a sentence, or a
// quote.
func sentenceStart(text string, i int) bool {
	s := strings.TrimRight(text[:i], " ")
	if s == "" {
		return true
	}
	r, _ := utf8.DecodeLastRuneInString(s)
	return strings.ContainsRune(".!?:\n“\"(", r)
}

// inLongerName reports whether the match is part of a longer proper name: a
// capitalized word is next to it, as in "Squire Duma" for the card Squire.
func inLongerName(text string, i, end int) bool {
	next := strings.TrimLeft(text[end:], "  ")
	if r, _ := utf8.DecodeRuneInString(next); unicode.IsUpper(r) && len(next) < len(text[end:]) {
		return true
	}
	prev := strings.TrimRight(text[:i], "  ")
	if len(prev) == len(text[:i]) {
		return false
	}
	// The word before: a capital at the start of a sentence ("If Narp") is not
	// part of a name.
	start := strings.LastIndexAny(prev, " \n\u00a0") + 1
	r, _ := utf8.DecodeRuneInString(prev[start:])
	return unicode.IsUpper(r) && !sentenceStart(prev, start)
}

// found is a title that a text names, and the name as it is in the text.
type found struct {
	Title string
	Text  string
}

// indexName finds title in text from a byte offset, and returns the start
// and the end of the match, or -1. A space in the title also matches a
// non-breaking space, because Master Vault keeps names on one line with one
// ("Ol’ Paddy").
func indexName(text string, from int, title string) (int, int) {
	if !strings.Contains(title, " ") {
		i := strings.Index(text[from:], title)
		if i < 0 {
			return -1, -1
		}
		return from + i, from + i + len(title)
	}
	re := nameRe(title)
	loc := re.FindStringIndex(text[from:])
	if loc == nil {
		return -1, -1
	}
	return from + loc[0], from + loc[1]
}

// nameRes caches the patterns of the titles with spaces.
var nameRes sync.Map

func nameRe(title string) *regexp.Regexp {
	if re, ok := nameRes.Load(title); ok {
		return re.(*regexp.Regexp)
	}
	re := regexp.MustCompile(strings.ReplaceAll(regexp.QuoteMeta(title), " ", "[  ]"))
	nameRes.Store(title, re)
	return re
}

// notes adds the notes to the cards. A curated note replaces the imported
// notes of the same card and kind.
func (l *linker) notes(imported, curated []NoteRule) {
	type key struct{ id, kind string }
	replaced := map[key]bool{}
	for _, n := range curated {
		if id, ok := l.noteCard(n.Title); ok {
			replaced[key{id, n.Kind}] = true
		}
	}
	add := func(n NoteRule) {
		id, ok := l.noteCard(n.Title)
		if !ok {
			return
		}
		c := &l.cards[l.index[id]]
		note := card.Note{Kind: n.Kind, Text: n.Text}
		if !slices.Contains(c.Notes, note) {
			c.Notes = append(c.Notes, note)
		}
	}
	for _, n := range imported {
		if id, ok := l.noteCard(n.Title); ok && !replaced[key{id, n.Kind}] {
			add(n)
		}
	}
	for _, n := range curated {
		add(n)
	}
}

// noteCard returns the card ID of a note title. "Title (House)" is the card
// of that house if the dataset has it, else the card with the title.
// "Title (Anomaly)" is the card with the title.
func (l *linker) noteCard(title string) (string, bool) {
	if id, ok := l.resolve(title); ok {
		return id, true
	}
	base, house, ok := strings.Cut(title, " (")
	if !ok {
		return "", false
	}
	house = strings.TrimSuffix(house, ")")
	id := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, house)
	if id == "anomaly" {
		id, ok := l.byTitle[base]
		return id, ok
	}
	// A house card that the dataset does not have yet, for example a
	// Redemption card of a set that the crawl has not read, gets no note.
	for _, c := range l.cards {
		if c.Title == base && c.House == id {
			return c.ID, true
		}
	}
	return "", false
}
