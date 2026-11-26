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
	"strconv"
	"strings"

	"github.com/dmikalova/keyforge-data/card"
	"github.com/dmikalova/keyforge-data/distill"
)

// ruleKinds are the rule types of the Archon Arcana relationships_and_notes
// sheet that link cards. The other types (copies, house, houseswap, anomaly,
// special) are about one card.
var ruleKinds = []string{"fixed", "always", "sometimes", "others", "variant"}

var (
	// copiesRe finds "at least 2 copies of" in a rule note.
	copiesRe = regexp.MustCompile(`at least (\d+) copies`)
	// othersRe finds "at least two other Sins" in a rule note.
	othersRe = regexp.MustCompile(`at least (\w+) other `)
	// prefixRe finds the family in `Other "Dæmo-" variants`.
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
		if len(f) < 4 || !slices.Contains(ruleKinds, f[1]) {
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
			return m[1] + " variants"
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
		n := othersRe.FindStringSubmatch(r.note)[1]
		if n == "one" || n == "1" {
			return "Always with another"
		}
		return "Always with " + number(n) + "+ others"
	}
	if m := copiesRe.FindStringSubmatch(r.note); m != nil {
		return "Always with " + m[1] + "+"
	}
	return "Always with"
}

// number returns the digits of a small number word.
func number(s string) string {
	words := []string{"zero", "one", "two", "three", "four", "five"}
	if i := slices.Index(words, s); i >= 0 {
		return strconv.Itoa(i)
	}
	return s
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
		rule := distill.Rule{
			Kind:  r.kind,
			Label: ruleLabel(r),
			Cards: titles(r.pages),
			With:  titles(r.cards),
		}
		group := len(r.pages) == 0
		if (group && len(rule.With) < 2) ||
			(!group && (len(rule.Cards) == 0 || len(rule.With) == 0)) {
			continue
		}
		out = append(out, rule)
	}
	slices.Sort(missing)
	return out, missing
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
	b, err := json.Marshal(
		distill.Override{Rules: rules},
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
	for _, m := range missing {
		_, _ = fmt.Fprintf(w, "no card has the title %q\n", m)
	}
	return nil
}
