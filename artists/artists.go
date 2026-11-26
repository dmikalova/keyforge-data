// Package artists matches the artist line of card images to the known
// artists, and reads and writes the curated artist files.
//
// The data is in the override files (package overlay):
//
//   - overrides/_artists.json has the known artists and the aliases.
//   - overrides/<set>.json maps each collector number of the set to its
//     artist, in its "artists" section.
//
// The OCR is not here: the artistocr tool reads the images (macOS Vision),
// and this package only decides which known artist a set of readings names.
package artists

import (
	"cmp"
	"maps"
	"slices"

	"github.com/dmikalova/keyforge-data/overlay"
	"github.com/dmikalova/keyforge-data/query"
)

// Aliases maps a variant spelling of an artist to the canonical name: a
// native and an anglicized name, a short form, a studio's other name, or a
// misprint. Cards print some artists in more than one way, and the dataset
// uses one name for one artist.
type Aliases map[string]string

// Canonical returns the canonical name of an artist. The match of a variant
// ignores case, diacritics, and punctuation.
func (a Aliases) Canonical(name string) string {
	if c, ok := a[name]; ok {
		return c
	}
	n := query.Normalize(name)
	for variant, c := range a {
		if query.Normalize(variant) == n {
			return c
		}
	}
	return name
}

// LoadAliases reads the aliases from the overrides directory.
func LoadAliases(dir string) (Aliases, error) {
	f, err := overlay.Load(dir, overlay.ArtistsFile)
	if err != nil {
		return nil, err
	}
	a := Aliases{}
	maps.Copy(a, f.ArtistAliases)
	return a, nil
}

// CanonicalAll replaces each variant spelling in the artists of the
// printings with its canonical name, in place.
func (a Aliases) CanonicalAll(m map[string]map[string]string) {
	for _, nums := range m {
		for num, name := range nums {
			nums[num] = a.Canonical(name)
		}
	}
}

// Thresholds of Decide. On a hand-labeled sample of 101 printings, Decide
// accepted 76 with these values and all 76 were right when it matched the
// names only. With the examples of the other printings (1986 printings), it
// accepted 90, and all 90 were right. The three artists of the sample that
// are not in the known list led the second candidate by 0.02 or less, so
// MinMargin rejects them.
const (
	// MinScore is the lowest score of the best candidate that Decide accepts.
	MinScore = 0.65
	// MinMargin is the lowest difference between the best and the second
	// candidate that Decide accepts.
	MinMargin = 0.15
)

// Candidate is one known artist and how well the readings match it, from 0
// to 1.
type Candidate struct {
	Name  string  `json:"name"`
	Score float64 `json:"score"`
}

// Examples are the past OCR readings of the printings of each artist, in
// normalized form. The artist line of one artist blurs in the same ways on
// many cards, so a reading like a past reading of an artist is evidence for
// that artist. Each answer in the review adds examples.
type Examples map[string]map[string]bool

// exampleMinLen is the shortest reading that is an example. Shorter readings
// are noise that matches too many artists.
const exampleMinLen = 3

// exampleMaxLenDiff skips the example readings that are much longer or
// shorter than the reading. They cannot be similar, and the skip saves time.
const exampleMaxLenDiff = 5

// Add adds the readings of one printing of the artist.
func (e Examples) Add(artist string, readings []string) {
	for _, r := range readings {
		n := query.Normalize(r)
		if len(n) < exampleMinLen {
			continue
		}
		if e[artist] == nil {
			e[artist] = map[string]bool{}
		}
		e[artist][n] = true
	}
}

// Match scores each artist against the OCR readings of one image and
// returns the candidates, best first. The artists are the known artists and
// the artists of the examples.
//
// An artist has two scores, and Match uses the higher one:
//
//   - The name score: the mean similarity of the three readings that are
//     most like the name.
//   - The example score: for each reading, the similarity of the most
//     similar example of the artist, and then the mean of the three best.
//
// The mean of three means that a few bad readings do not lower the score.
//
// A variant spelling in aliases is a candidate too, and its score counts
// for the canonical name. So one artist with two spellings is one
// candidate, and the second spelling does not lower the margin.
func Match(readings, known []string, examples Examples, aliases Aliases) []Candidate {
	raw := match(readings, slices.Concat(known, slices.Collect(maps.Keys(aliases))), examples)
	if len(aliases) == 0 {
		return raw
	}
	out := make([]Candidate, 0, len(raw))
	seen := map[string]bool{}
	// raw is best first, so the first candidate of a name has its best score.
	for _, c := range raw {
		name := aliases.Canonical(c.Name)
		if !seen[name] {
			seen[name] = true
			out = append(out, Candidate{Name: name, Score: c.Score})
		}
	}
	return out
}

func match(readings, known []string, examples Examples) []Candidate {
	norm := make([]string, 0, len(readings))
	for _, r := range readings {
		if n := query.Normalize(r); n != "" {
			norm = append(norm, n)
		}
	}
	// The example score uses each distinct reading one time, so one reading
	// that many variants agree on does not fill the three best.
	distinct := slices.DeleteFunc(
		slices.Clone(norm),
		func(r string) bool { return len(r) < exampleMinLen },
	)
	slices.Sort(distinct)
	distinct = slices.Compact(distinct)
	names := slices.Concat(known, slices.Collect(maps.Keys(examples)))
	slices.Sort(names)
	names = slices.Compact(names)
	out := make([]Candidate, 0, len(names))
	if len(norm) == 0 {
		return out
	}
	for _, name := range names {
		k := query.Normalize(name)
		score := topMean(norm, func(r string) float64 { return similarity(r, k) })
		if ex := examples[name]; len(ex) > 0 && len(distinct) > 0 {
			score = max(
				score,
				topMean(distinct, func(r string) float64 { return bestExample(r, ex) }),
			)
		}
		out = append(out, Candidate{Name: name, Score: score})
	}
	slices.SortStableFunc(out, func(a, b Candidate) int { return cmp.Compare(b.Score, a.Score) })
	return out
}

// topMean returns the mean of the three highest values of f over the
// readings.
func topMean(readings []string, f func(string) float64) float64 {
	sims := make([]float64, 0, len(readings))
	for _, r := range readings {
		sims = append(sims, f(r))
	}
	slices.SortFunc(sims, func(a, b float64) int { return cmp.Compare(b, a) })
	top := sims[:min(3, len(sims))]
	sum := 0.0
	for _, s := range top {
		sum += s
	}
	return sum / float64(len(top))
}

func bestExample(r string, ex map[string]bool) float64 {
	if len(r) < exampleMinLen {
		return 0
	}
	best := 0.0
	for e := range ex {
		if d := len(r) - len(e); d > exampleMaxLenDiff || d < -exampleMaxLenDiff {
			continue
		}
		best = max(best, similarity(r, e))
	}
	return best
}

// Decide returns the best candidate if the match is sure: its score is at
// least MinScore and it leads the second candidate by at least MinMargin.
func Decide(c []Candidate) (string, bool) {
	if len(c) == 0 || c[0].Score < MinScore {
		return "", false
	}
	if len(c) > 1 && c[0].Score-c[1].Score < MinMargin {
		return "", false
	}
	return c[0].Name, true
}

// similarity is 1 minus the edit distance over the longer length.
func similarity(a, b string) float64 {
	n := max(len(a), len(b))
	if n == 0 {
		return 1
	}
	return 1 - float64(levenshtein(a, b))/float64(n)
}

func levenshtein(a, b string) int {
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}

// LoadKnown reads the known artists from the overrides directory.
func LoadKnown(dir string) ([]string, error) {
	f, err := overlay.Load(dir, overlay.ArtistsFile)
	return f.KnownArtists, err
}

// SaveKnown writes the known artists, sorted by folded name, without
// duplicates.
func SaveKnown(dir string, names []string) error {
	names = slices.Clone(names)
	slices.SortFunc(
		names,
		func(a, b string) int { return cmp.Compare(query.Fold(a), query.Fold(b)) },
	)
	names = slices.Compact(names)
	return overlay.Update(
		dir,
		overlay.ArtistsFile,
		func(f *overlay.File) { f.KnownArtists = names },
	)
}

// Load reads the artist of each printing, by set slug and collector number,
// from the set files of the overrides directory.
func Load(dir string) (map[string]map[string]string, error) {
	files, err := overlay.LoadAll(dir)
	if err != nil {
		return nil, err
	}
	out := map[string]map[string]string{}
	for name, f := range files {
		if overlay.IsSet(name) && len(f.Artists) > 0 {
			out[name] = f.Artists
		}
	}
	return out, nil
}

// Save writes the artists of one set to the set file of the overrides
// directory. It keeps the other sections of the file.
func Save(dir, set string, m map[string]string) error {
	return overlay.Update(dir, set, func(f *overlay.File) { f.Artists = m })
}
