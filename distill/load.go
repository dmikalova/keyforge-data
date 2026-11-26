package distill

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/dmikalova/keyforge-data/overlay"
	"github.com/dmikalova/keyforge-data/raw"
)

// The loaders below read the inputs of Run. They are not pure: they read
// the raw store, the overrides directory, and the public bucket.

// LoadCards returns the newest stored version of each English card object.
func LoadCards(store *raw.Store) ([][]byte, error) {
	versions, err := store.Linked("cards", "en")
	if err != nil {
		return nil, err
	}
	out := make([][]byte, 0, len(versions))
	for _, vs := range versions {
		v := slices.MaxFunc(
			vs,
			func(a, b raw.LinkedVersion) int { return a.ModTime.Compare(b.ModTime) },
		)
		out = append(out, v.Raw)
	}
	return out, nil
}

// LoadOverrides reads overrides/<set>.json. A missing directory is not an
// error.
func LoadOverrides(dir string) (map[string]Override, error) {
	return overlay.LoadAll(dir)
}

// PublicBucket is the bucket that holds the card images and the dataset.
const PublicBucket = "mklv-keyforge-cards-public"

// ListImages returns the object names under images/cards/ in the public
// bucket, without that prefix. Each name ends with "?v=<generation>". The
// generation changes when an object is replaced, so the image URL changes
// and browsers and caches load the new image. The bucket caches each URL for
// a year. The bucket is public, so the list needs no
// credentials.
func ListImages(ctx context.Context, client *http.Client) ([]string, error) {
	const prefix = "images/cards/"
	var out []string
	token := ""
	for {
		q := url.Values{
			"prefix": {prefix},
			"fields": {"items(name,generation),nextPageToken"},
		}
		if token != "" {
			q.Set("pageToken", token)
		}
		u := "https://storage.googleapis.com/storage/v1/b/" + PublicBucket + "/o?" + q.Encode()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		b, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("list images: %s", resp.Status)
		}
		var page struct {
			Items []struct {
				Name       string `json:"name"`
				Generation string `json:"generation"`
			} `json:"items"`
			NextPageToken string `json:"nextPageToken"`
		}
		if err := json.Unmarshal(b, &page); err != nil {
			return nil, err
		}
		for _, it := range page.Items {
			name := strings.TrimPrefix(it.Name, prefix)
			if it.Generation != "" {
				name += "?v=" + it.Generation
			}
			out = append(out, name)
		}
		if token = page.NextPageToken; token == "" {
			return out, nil
		}
	}
}

// WriteProblems writes the problems as Markdown, grouped by kind.
func WriteProblems(w io.Writer, version string, problems []Problem) error {
	var b strings.Builder
	fmt.Fprintf(&b, "# Dataset problems\n\n")
	fmt.Fprintf(&b, "Dataset version %s. Problems: %d.\n\n", version, len(problems))
	b.WriteString("Distillation does not recognize these cards, or data is missing. ")
	b.WriteString("Fix a problem with an override in `overrides/<set>.json` or with new data.\n")
	kind := ""
	for _, p := range problems {
		if p.Kind != kind {
			kind = p.Kind
			fmt.Fprintf(&b, "\n## %s (%d)\n\n", kind, countKind(problems, kind))
			b.WriteString("| Set | # | Title | Detail | Decks |\n| --- | --- | --- | --- | --- |\n")
		}
		b.WriteString(row(p.Set, p.Number, cell(p.Title), cell(p.Detail), deckLinks(p.Decks)))
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func countKind(ps []Problem, kind string) int {
	n := 0
	for _, p := range ps {
		if p.Kind == kind {
			n++
		}
	}
	return n
}

func cell(s string) string {
	return strings.ReplaceAll(s, "|", `\|`)
}

// row writes a compact Markdown table row: an empty cell is "| |".
func row(cells ...string) string {
	var b strings.Builder
	b.WriteString("|")
	for _, c := range cells {
		if c != "" {
			b.WriteString(" " + c)
		}
		b.WriteString(" |")
	}
	return b.String() + "\n"
}

// LoadDecks maps each card object ID to the IDs of the stored decks that
// have it, and to the expansions of those decks, from every crawl in the raw
// store.
func LoadDecks(store *raw.Store) (map[string][]string, map[string][]int, error) {
	crawls, err := store.Crawls()
	if err != nil {
		return nil, nil, err
	}
	out := map[string][]string{}
	deckSets := map[string][]int{}
	seen := map[string]bool{}
	for _, id := range crawls {
		for rec, err := range store.Records(id) {
			if err != nil {
				return nil, nil, err
			}
			var d struct {
				ID        string `json:"id"`
				Expansion int    `json:"expansion"`
				Links     struct {
					Cards []string `json:"cards"`
				} `json:"_links"`
			}
			if err := json.Unmarshal(rec.Deck, &d); err != nil {
				return nil, nil, err
			}
			if seen[d.ID] {
				continue
			}
			seen[d.ID] = true
			for _, c := range slices.Compact(slices.Sorted(slices.Values(d.Links.Cards))) {
				out[c] = append(out[c], d.ID)
				if !slices.Contains(deckSets[c], d.Expansion) {
					deckSets[c] = append(deckSets[c], d.Expansion)
				}
			}
		}
	}
	return out, deckSets, nil
}

// deckLinks returns Markdown links to up to three Master Vault decks, and
// the count of the other decks.
func deckLinks(decks []string) string {
	var parts []string
	for _, d := range decks[:min(3, len(decks))] {
		parts = append(parts, "[deck](https://www.keyforgegame.com/deck-details/"+d+")")
	}
	if n := len(decks) - 3; n > 0 {
		parts = append(parts, fmt.Sprintf("+%d", n))
	}
	return strings.Join(parts, " ")
}
