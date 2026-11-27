package main

import (
	"encoding/csv"
	"encoding/json/v2"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/dmikalova/keyforge-data/artists"
	"github.com/dmikalova/keyforge-data/card"
	"github.com/dmikalova/keyforge-data/query"
)

// hintMinScore is the lowest score of the hint's artist among the OCR
// candidates that counts as agreement. It is lower than artists.MinScore:
// the hint is a second, independent source.
const hintMinScore = 0.5

// The OCR contradicts the hint when its best candidate is another artist
// with at least hintContradict, and a lead of hintLead over the hint. A
// review item has no sure OCR match (artists.Decide), so a weaker best
// candidate is noise: the reading is of other text, or too blurred.
const (
	hintContradict = 0.5
	hintLead       = 0.1
)

// untrustedHints are the hint names and titles that the comparison of
// 2026-11-26 found wrong (docs/sources.md): a studio for an artist, and the
// swapped artists of related cards. A printing with one of them goes to the
// review.
var untrustedHints = map[string]bool{
	"art tavern":   true,
	"teliga":       true,
	"huntingwitch": true,
}

// hintResult is the decision of the hints for one review item.
type hintResult int

const (
	hintReview   hintResult = iota // the image decides
	hintAgree                      // the OCR and the hint name the same artist
	hintOnlyHint                   // no OCR reading, and the card's other printings have the hint's artist
	hintNewCard                    // no OCR reading, and the card has no other printing with an artist
)

// decideHint decides one review item from its OCR candidates, the hint's
// artist for its title, and the artists of the card's other printings.
func decideHint(cands []artists.Candidate, readings int, hint string, others []string) hintResult {
	if hint == "" {
		return hintReview
	}
	hintScore := 0.0
	for _, c := range cands {
		if c.Name == hint {
			hintScore = max(hintScore, c.Score)
		}
	}
	if hintScore >= hintMinScore {
		return hintAgree
	}
	contradicted := readings > 0 && len(cands) > 0 && cands[0].Name != hint &&
		cands[0].Score >= hintContradict && cands[0].Score-hintScore >= hintLead
	switch {
	case contradicted:
	case len(others) == 1 && others[0] == hint:
		return hintOnlyHint
	case len(others) == 0:
		// The images are 300 by 420, too small to read the artist line by
		// eye. On 2026-11-26 the hint file was right for all printings
		// that the OCR could not contradict.
		return hintNewCard
	}
	return hintReview
}

// artistsHints settles the review queue with a hint file of title and
// artist (for example skyjedi/illustrators.csv of the Archon Arcana bots,
// docs/sources.md). It keeps the items that it cannot settle in the queue.
func artistsHints(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("artists-hints", flag.ContinueOnError)
	var af artistFlags
	af.register(fs)
	hintsPath := fs.String("hints", "tmp/hints/illustrators.csv", "CSV of card title and artist")
	dataset := fs.String("dataset", "tmp/cards.json", "distilled dataset, for the other printings")
	explain := fs.Bool("explain", false, "print why each item stays in the review")
	if err := fs.Parse(args); err != nil {
		return err
	}
	aliases, err := artists.LoadAliases(af.dir)
	if err != nil {
		return err
	}
	known, err := artists.LoadKnown(af.dir)
	if err != nil {
		return err
	}
	assigned, err := artists.Load(af.dir)
	if err != nil {
		return err
	}
	hints, err := loadHints(*hintsPath, aliases, known)
	if err != nil {
		return err
	}
	others, err := titleArtists(*dataset, assigned)
	if err != nil {
		return err
	}
	b, err := os.ReadFile(af.reviewFile())
	if err != nil {
		return err
	}
	var queue []reviewItem
	if err := json.Unmarshal(b, &queue); err != nil {
		return err
	}
	// The hint's artists are candidates too: a new set brings artists that
	// the known list does not have yet.
	candidates := slices.Clone(known)
	for _, name := range hints {
		if !slices.Contains(candidates, name) {
			candidates = append(candidates, name)
		}
	}
	var rest []reviewItem
	counts := map[hintResult]int{}
	for _, it := range queue {
		key := query.Normalize(it.Title)
		hint := hints[key]
		if untrustedHints[query.Normalize(hint)] || untrustedHints[key] {
			hint = ""
		}
		cands := artists.Match(it.Readings, candidates, nil, aliases)
		r := decideHint(cands, len(it.Readings), hint, others[key])
		counts[r]++
		if r == hintReview {
			rest = append(rest, it)
			if *explain {
				explainHint(stdout, it, hint, cands, others[key])
			}
			continue
		}
		setArtist(assigned, it.Set, it.Number, hint)
		if !slices.Contains(known, hint) {
			known = append(known, hint)
			_, _ = fmt.Fprintln(stdout, "new artist:", hint)
		}
	}
	if err := saveAll(af.dir, assigned); err != nil {
		return err
	}
	if err := artists.SaveKnown(af.dir, known); err != nil {
		return err
	}
	if err := writeJSON(af.reviewFile(), rest); err != nil {
		return err
	}
	_, err = fmt.Fprintf(
		stdout,
		"%d review items: %d OCR and hint agree, %d hint only, %d hint only for a new card, %d left to review\n",
		len(queue),
		counts[hintAgree],
		counts[hintOnlyHint],
		counts[hintNewCard],
		len(rest),
	)
	return err
}

// loadHints reads a CSV with a header and the columns title and artist, by
// normalized title. A title with two artists has no hint. A name that is a
// known name in another spelling or encoding, for example "Leo Sá" with a
// combining accent, becomes the known name.
func loadHints(path string, aliases artists.Aliases, known []string) (map[string]string, error) {
	byNorm := map[string]string{}
	for _, k := range known {
		byNorm[query.Normalize(k)] = k
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, errors.New("hints: empty file")
	}
	out := map[string]string{}
	two := map[string]bool{}
	for _, row := range rows[1:] {
		if len(row) < 2 || row[1] == "" {
			continue
		}
		// The hint file has names with two spaces, for example
		// "Raimundo  Souza".
		key := query.Normalize(row[0])
		name := aliases.Canonical(strings.Join(strings.Fields(row[1]), " "))
		if k, ok := byNorm[query.Normalize(name)]; ok {
			name = k
		}
		if old, ok := out[key]; ok && old != name {
			two[key] = true
		}
		out[key] = name
	}
	for key := range two {
		delete(out, key)
	}
	return out, nil
}

// titleArtists returns the distinct artists of the printings of each card
// title, by normalized title, from the assigned artists.
func titleArtists(path string, assigned map[string]map[string]string) (map[string][]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var d card.Dataset
	if err := json.Unmarshal(b, &d); err != nil {
		return nil, err
	}
	out := map[string][]string{}
	for _, c := range d.Cards {
		key := query.Normalize(c.Title)
		for _, p := range c.Printings {
			if a := assigned[p.Set][p.Number]; a != "" && !slices.Contains(out[key], a) {
				out[key] = append(out[key], a)
			}
		}
	}
	return out, nil
}

// explainHint prints why a review item stays in the review: the hint, the
// best OCR candidates, and the artists of the card's other printings.
func explainHint(
	w io.Writer,
	it reviewItem,
	hint string,
	cands []artists.Candidate,
	others []string,
) {
	var top strings.Builder
	for i, c := range cands {
		if i == 2 {
			break
		}
		fmt.Fprintf(&top, " %s=%.2f", c.Name, c.Score)
	}
	hintScore := 0.0
	for _, c := range cands {
		if c.Name == hint {
			hintScore = c.Score
		}
	}
	_, _ = fmt.Fprintf(w, "%s %s\t%s\thint=%q (%.2f)\tocr:%s\tothers=%v\n",
		it.Set, it.Number, it.Title, hint, hintScore, top.String(), others)
}
