package main

import (
	"bufio"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"io"
	"os"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/dmikalova/keyforge-data/card"
	"github.com/dmikalova/keyforge-data/distill"
)

// ruleKinds are the rule types of the Archon Arcana relationships_and_notes
// sheet that link cards. The other types (copies, house, houseswap, anomaly,
// special) are about one card.
var ruleKinds = []string{"fixed", "always", "sometimes", "others", "variant"}

var (
	// othersRe finds "at least two other Sins" in a rule note.
	othersRe = regexp.MustCompile(`at least (\w+) other `)
	// prefixRe finds the family in `Other "Dæmo-" variants`. The label drops
	// the hyphen: "Dæmo variants".
	prefixRe = regexp.MustCompile(`"([^"]+)" variants`)
)

// sheetRule is one row of the sheet: the page cards (empty for a group), the
// rule type, the linked cards, and the note.
type sheetRule struct {
	pages []string
	kind  string
	cards []string
	note  string
}

// readSheet reads the rows of the sheet TSV that link cards.
func readSheet(r io.Reader) ([]sheetRule, error) {
	var out []sheetRule
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		f := strings.Split(sc.Text(), "\t")
		if len(f) < 4 || (!slices.Contains(ruleKinds, f[1]) && !slices.Contains(noteKinds, f[1])) {
			continue
		}
		out = append(out, sheetRule{
			pages: bullets(f[0]),
			kind:  f[1],
			cards: bullets(f[2]),
			note:  strings.TrimSpace(f[3]),
		})
	}
	return out, sc.Err()
}

// bullets splits "•A•B•" into its titles.
func bullets(s string) []string {
	var out []string
	for t := range strings.SplitSeq(s, "•") {
		if t = strings.TrimSpace(t); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// ruleLabel returns the label of the Related entry of a rule.
func ruleLabel(r sheetRule) string {
	switch r.kind {
	case "sometimes":
		return "Sometimes with"
	case "variant":
		if m := prefixRe.FindStringSubmatch(r.note); m != nil {
			return strings.Trim(m[1], "-") + " variants"
		}
		return "Variants"
	case "fixed":
		if len(r.pages) == 0 {
			return "Always together"
		}
	}
	switch {
	case strings.Contains(r.note, "one of") || strings.Contains(r.note, "at least 1 of"):
		return "Always with one of"
	case othersRe.MatchString(r.note):
		// The label does not give the number of copies or of other cards.
		if n := othersRe.FindStringSubmatch(r.note)[1]; n == "one" || n == "1" {
			return "Always with another"
		}
		return "Always with others"
	}
	return "Always with"
}

// titleResolver maps a sheet title to a dataset title.
type titleResolver struct {
	known map[string]bool
}

// resolve returns the dataset title of a sheet title. The sheet uses a
// straight apostrophe in some titles, and "(Evil Twin)" for an evil twin.
func (t titleResolver) resolve(s string) (string, bool) {
	base, twin := strings.CutSuffix(s, " (Evil Twin)")
	for _, c := range []string{base, strings.ReplaceAll(base, "'", "’")} {
		if t.known[c] {
			if twin {
				return c + " (Evil Twin)", true
			}
			return c, true
		}
	}
	return "", false
}

// importRules turns the sheet rows into deck rules. It returns the rules
// and the titles that no card has.
func importRules(rows []sheetRule, cards []card.Card) ([]distill.Rule, []string) {
	t := titleResolver{known: map[string]bool{}}
	for _, c := range cards {
		t.known[c.Title] = true
	}
	var missing []string
	titles := func(in []string) []string {
		var out []string
		for _, s := range in {
			if r, ok := t.resolve(s); ok {
				out = append(out, r)
			} else if !slices.Contains(missing, s) {
				missing = append(missing, s)
			}
		}
		return out
	}
	var out []distill.Rule
	for _, r := range rows {
		if !slices.Contains(ruleKinds, r.kind) {
			continue
		}
		rule := distill.Rule{
			Kind:  r.kind,
			Label: ruleLabel(r),
			Cards: titles(r.pages),
			With:  titles(r.cards),
		}
		group := len(r.pages) == 0
		if (group && len(rule.With) < 2) ||
			(!group && (len(rule.Cards) == 0 || len(rule.With) == 0)) ||
			megaPair(append(slices.Clone(rule.Cards), rule.With...)) {
			continue
		}
		out = append(out, rule)
	}
	slices.Sort(missing)
	return out, missing
}

// megaPair reports whether the titles are a card and its Mega creature,
// such as "Alaka" and "Mega Alaka". Distillation links them already.
func megaPair(titles []string) bool {
	if len(titles) != 2 {
		return false
	}
	a, b := titles[0], titles[1]
	return a == "Mega "+b || b == "Mega "+a
}

// threadsRules writes overrides/_deckrules.json from the sheet TSV.
func threadsRules(w io.Writer, in, file string, cards []card.Card) error {
	f, err := os.Open(in)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	rows, err := readSheet(f)
	if err != nil {
		return err
	}
	rules, missing := importRules(rows, cards)
	notes, noteMissing := importNotes(rows, cards)
	missing = append(missing, noteMissing...)
	slices.Sort(missing)
	missing = slices.Compact(missing)
	b, err := json.Marshal(
		distill.Override{Rules: rules, Notes: notes},
		jsontext.Multiline(true),
		jsontext.WithIndent("  "),
	)
	if err != nil {
		return err
	}
	if err := os.WriteFile(file, append(b, '\n'), 0o600); err != nil {
		return err
	}
	counts := map[string]int{}
	for _, r := range rules {
		counts[r.Kind]++
	}
	for _, k := range ruleKinds {
		_, _ = fmt.Fprintf(w, "%s: %d rules\n", k, counts[k])
	}
	_, _ = fmt.Fprintf(w, "notes: %d\n", len(notes))
	for _, m := range missing {
		_, _ = fmt.Fprintf(w, "no card has the title %q\n", m)
	}
	return nil
}

// noteKinds are the rule types of the sheet that are about one card.
var noteKinds = []string{"copies", "house", "houseswap", "anomaly", "special"}

var (
	// copiesRe finds the number in "at least 3 copies of this card".
	copiesRe = regexp.MustCompile(`at least (\d+) copies`)
	// houseRe finds the house in "always appears in a deck with house Dis".
	houseRe = regexp.MustCompile(`always appears in a deck with house (.+?)\.?$`)
	// wasRe finds the houses in "belonged to house Sanctum prior to being in
	// house Redemption".
	wasRe = regexp.MustCompile(`belonged to house (.+?) prior to being in house (.+?)\.?$`)
)

// noteText returns the short text of a note from the sheet note.
func noteText(kind, note string) string {
	note = strings.TrimRight(strings.TrimSpace(note), ".: ")
	switch {
	case kind == "copies" && copiesRe.MatchString(note):
		return "Always " + copiesRe.FindStringSubmatch(note)[1] + "+ copies"
	case houseRe.MatchString(note):
		return "Always in a deck with " + houseRe.FindStringSubmatch(note)[1]
	case wasRe.MatchString(note):
		m := wasRe.FindStringSubmatch(note)
		return "Was in " + m[1] + " before " + m[2]
	}
	note = strings.TrimRight(strings.TrimPrefix(note, "this card "), ".: ")
	note = strings.Replace(note, "always appears in a deck with ", "always with ", 1)
	if note == "" {
		return ""
	}
	r, size := utf8.DecodeRuneInString(note)
	return string(unicode.ToUpper(r)) + note[size:]
}

// importNotes turns the sheet rows about one card into notes. It returns
// the notes and the titles that no card has.
func importNotes(rows []sheetRule, cards []card.Card) ([]distill.NoteRule, []string) {
	t := titleResolver{known: map[string]bool{}}
	for _, c := range cards {
		t.known[c.Title] = true
	}
	var out []distill.NoteRule
	var missing []string
	for _, r := range rows {
		if !slices.Contains(noteKinds, r.kind) {
			continue
		}
		text := noteText(r.kind, r.note)
		for _, p := range r.pages {
			base, _, _ := strings.Cut(p, " (")
			title, ok := t.resolve(base)
			if !ok {
				if !slices.Contains(missing, p) {
					missing = append(missing, p)
				}
				continue
			}
			if text != "" {
				out = append(
					out,
					distill.NoteRule{
						Kind:  r.kind,
						Title: title + strings.TrimPrefix(p, base),
						Text:  text,
					},
				)
			}
		}
	}
	slices.Sort(missing)
	return out, missing
}
