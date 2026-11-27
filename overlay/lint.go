package overlay

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/dmikalova/keyforge-data/card"
	"github.com/dmikalova/keyforge-data/sets"
)

// Finding is one problem that Lint finds in the override files.
type Finding struct {
	File string
	// Error is true for a mistake in a file that people edit. A finding in
	// the generated _deckrules.json is a warning: it names a card of a set
	// that the crawl does not have yet.
	Error bool
	Msg   string
}

func (f Finding) String() string {
	level := "warning"
	if f.Error {
		level = "error"
	}
	return fmt.Sprintf("%s: %s.json: %s", level, f.File, f.Msg)
}

// linter holds the dataset facts that the checks use.
type linter struct {
	titles  map[string]bool
	numbers map[string]bool // "<set>/<number>"
	out     []Finding
}

func (l *linter) add(file string, isErr bool, format string, args ...any) {
	l.out = append(l.out, Finding{File: file, Error: isErr, Msg: fmt.Sprintf(format, args...)})
}

// Lint checks the override files against a dataset: unknown sets, titles,
// and collector numbers, duplicates, and entries that do nothing.
func Lint(files map[string]File, cards []card.Card) []Finding {
	l := &linter{titles: map[string]bool{}, numbers: map[string]bool{}}
	for _, c := range cards {
		l.titles[c.Title] = true
		for _, p := range c.Printings {
			l.numbers[p.Set+"/"+p.Number] = true
		}
	}
	known := map[string]bool{}
	for _, n := range files[ArtistsFile].KnownArtists {
		if known[n] {
			l.add(ArtistsFile, true, "known artist %q is listed twice", n)
		}
		known[n] = true
	}
	l.artistAliases(files[ArtistsFile].ArtistAliases, known)
	for _, name := range slices.Sorted(maps.Keys(files)) {
		f := files[name]
		if IsSet(name) {
			l.set(name, f, known, files[ArtistsFile].ArtistAliases)
		}
		l.threads(name, f.Threads)
		l.rules(name, f.Rules)
		l.notes(name, f.Notes)
	}
	return l.out
}

func (l *linter) artistAliases(aliases map[string]string, known map[string]bool) {
	for _, alias := range slices.Sorted(maps.Keys(aliases)) {
		name := aliases[alias]
		if !known[name] {
			l.add(ArtistsFile, true, "alias %q names %q, which is not a known artist", alias, name)
		}
		if _, ok := aliases[name]; ok {
			l.add(ArtistsFile, true, "alias %q names %q, which is an alias too", alias, name)
		}
	}
}

func (l *linter) set(name string, f File, known map[string]bool, aliases map[string]string) {
	if _, ok := sets.BySlug(name); !ok {
		l.add(name, true, "%q is not a set slug", name)
		return
	}
	for _, num := range slices.Sorted(maps.Keys(f.Cards)) {
		if !l.numbers[name+"/"+num] {
			l.add(name, true, "card fix %s: no printing has this number", num)
		}
		if fix := f.Cards[num]; fix.Power == nil && fix.Armor == nil {
			l.add(name, true, "card fix %s changes nothing", num)
		}
	}
	for _, num := range slices.Sorted(maps.Keys(f.Artists)) {
		a := f.Artists[num]
		if !l.numbers[name+"/"+num] {
			l.add(name, false, "artist of %s: no printing has this number", num)
		}
		if !known[a] {
			if c, ok := aliases[a]; ok {
				l.add(name, true, "artist of %s is %q, an alias of %q", num, a, c)
			} else {
				l.add(name, true, "artist of %s is %q, which is not a known artist", num, a)
			}
		}
	}
	for _, num := range f.GreyFrame {
		if !l.numbers[name+"/"+num] {
			l.add(name, false, "grey frame %s: no printing has this number", num)
		}
	}
	for _, t := range slices.Sorted(maps.Keys(f.Aliases)) {
		if !l.titles[t] {
			l.add(name, true, "aliases of %q: no card has this title", t)
		}
	}
}

// title reports whether a rule or note title names a card: the title, or
// the title with " (Evil Twin)", " (Anomaly)", or a house in parentheses.
func (l *linter) title(t string) bool {
	if l.titles[t] {
		return true
	}
	base, _, ok := strings.Cut(t, " (")
	return ok && l.titles[base]
}

func (l *linter) threads(file string, ths []Thread) {
	labels := map[string]bool{}
	members := map[string]string{}
	for _, th := range ths {
		if labels[th.Label] {
			l.add(file, true, "thread %q is in the file twice", th.Label)
		}
		labels[th.Label] = true
		seen := map[string]bool{}
		n := 0
		for _, t := range th.Titles {
			if seen[t] {
				l.add(file, true, "thread %q has %q twice", th.Label, t)
			}
			seen[t] = true
			if l.title(t) {
				n++
			} else {
				l.add(file, true, "thread %q: no card has the title %q", th.Label, t)
			}
		}
		if n < 2 {
			l.add(file, true, "thread %q links fewer than 2 cards", th.Label)
		}
		key := strings.Join(slices.Sorted(maps.Keys(seen)), "|")
		if other, ok := members[key]; ok {
			l.add(file, true, "threads %q and %q have the same cards", other, th.Label)
		}
		members[key] = th.Label
	}
}

func (l *linter) rules(file string, rs []Rule) {
	missing := map[string]bool{}
	for _, r := range rs {
		for _, t := range append(slices.Clone(r.Cards), r.With...) {
			if !l.title(t) {
				missing[t] = true
			}
		}
	}
	for _, t := range slices.Sorted(maps.Keys(missing)) {
		l.add(file, file != RulesFile, "rule: no card has the title %q", t)
	}
}

func (l *linter) notes(file string, ns []NoteRule) {
	seen := map[NoteRule]bool{}
	for _, n := range ns {
		if seen[n] {
			l.add(file, true, "note %q of %q is in the file twice", n.Text, n.Title)
		}
		seen[n] = true
		if !l.title(n.Title) {
			l.add(file, file != RulesFile, "note: no card has the title %q", n.Title)
		}
		if strings.TrimSpace(n.Text) == "" {
			l.add(file, true, "note of %q has no text", n.Title)
		}
	}
}
