// Package crawl reads Master Vault into the raw store (ADR 0002).
//
// FeedSets pages the deck feed of each set and keeps only the decks that add
// a card the crawl has not seen. ComputeCover finds the smallest list of the
// kept decks that has every card. Refresh fetches a list of decks in each
// language.
package crawl

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"

	"github.com/dmikalova/keyforge-data/mv"
	"github.com/dmikalova/keyforge-data/raw"
	"github.com/dmikalova/keyforge-data/sets"
)

// ModeFeedSets is the manifest mode of a FeedSets crawl.
const ModeFeedSets = "feed-sets"

// StopRule decides when the scan of one set stops.
type StopRule struct {
	// QuietPages is the number of pages in a row with no new card after
	// which the scan stops, if the set has no gap in its card numbers.
	QuietPages int
	// MaxQuietPages is the number of pages in a row with no new card after
	// which the scan stops, gap or not.
	MaxQuietPages int
}

// DefaultStop is the stop rule for a cover scan.
var DefaultStop = StopRule{QuietPages: 30, MaxQuietPages: 200}

// SetState is the resume state of one set in a FeedSets crawl.
type SetState struct {
	// Page is the last page that the crawl stored.
	Page int `json:"page"`
	// Quiet is the number of pages in a row with no new card.
	Quiet int `json:"quiet"`
	// Count is the feed's deck count for the set.
	Count int `json:"count"`
	// Kept is the number of decks kept.
	Kept int `json:"kept"`
	// Seen are the target printings seen (TargetKey), sorted.
	Seen []string `json:"seen"`
	// Numbers are the card numbers of the set's own target cards.
	Numbers []string `json:"numbers"`
	// Done is true when the scan of the set stopped.
	Done bool `json:"done"`
	// Reason says why the scan stopped.
	Reason string `json:"reason,omitempty"`
}

// FeedSetsState is the resume state of a FeedSets crawl, by set slug.
type FeedSetsState map[string]*SetState

// IsTarget reports whether the cover must contain a card object. A maverick
// is a copy of a card in another house, so it is not a target. Every other
// card object is a target, so the data shows what Master Vault sends.
func IsTarget(c mv.CardHead) bool {
	return !c.IsMaverick
}

// TargetKey is the printing of a target card object: the expansion, the
// number, the title, and the house. Master Vault makes a new card object
// for each enhanced copy, so object IDs never stop being new in a set with
// Enhance. The scan and the cover count printings, not objects.
func TargetKey(c mv.CardHead) string {
	return fmt.Sprintf("%d/%s/%s/%s", c.Expansion, c.CardNumber, c.Title, c.House)
}

// FeedSets scans the deck feed of each set, oldest decks first. It stores
// every linked object of each page, and it keeps a deck only if the deck has
// a target card that the crawl has not seen. It continues a stopped crawl
// from the crawl's state.
func FeedSets(
	ctx context.Context,
	log *slog.Logger,
	client *mv.Client,
	c *raw.Crawl,
	targets []sets.Set,
	stop StopRule,
) error {
	state := FeedSetsState{}
	if err := c.State(&state); err != nil {
		return err
	}
	for _, s := range targets {
		st := state[s.Slug]
		if st == nil {
			st = &SetState{}
			state[s.Slug] = st
		}
		if st.Done {
			log.Info("set already scanned", "set", s.Slug, "kept", st.Kept, "cards", len(st.Seen))
			continue
		}
		if err := scanSet(ctx, log, client, c, s, st, state, stop); err != nil {
			return fmt.Errorf("crawl %s: %w", s.Slug, err)
		}
	}
	return c.Checkpoint(state)
}

func scanSet(
	ctx context.Context,
	log *slog.Logger,
	client *mv.Client,
	c *raw.Crawl,
	s sets.Set,
	st *SetState,
	all FeedSetsState,
	stop StopRule,
) error {
	seen := map[string]bool{}
	for _, id := range st.Seen {
		seen[id] = true
	}
	numbers := map[string]bool{}
	for _, n := range st.Numbers {
		numbers[n] = true
	}
	log.Info("scan set", "set", s.Slug, "expansion", s.Expansion, "from_page", st.Page+1)
	for {
		page := st.Page + 1
		p, err := client.Feed(ctx, mv.FeedQuery{Expansion: s.Expansion, Page: page})
		if errors.Is(err, mv.ErrPastLimit) || errors.Is(err, mv.ErrNotFound) {
			st.Done, st.Reason = true, "end of feed: "+err.Error()
			break
		}
		if err != nil {
			return err
		}
		heads, err := p.Cards()
		if err != nil {
			return err
		}
		kept, added := keepNew(p.Decks, heads, seen, numbers, s.Expansion)
		if err := c.WriteDecks("en", "feed", kept); err != nil {
			return err
		}
		if _, err := c.PutLinked("en", p.Linked); err != nil {
			return err
		}
		st.Page, st.Count = page, p.Count
		st.Kept += len(kept)
		if added > 0 {
			st.Quiet = 0
		} else {
			st.Quiet++
		}
		st.Seen = sortedKeys(seen)
		st.Numbers = sortedKeys(numbers)
		log.Info("page",
			"set", s.Slug, "page", page, "decks", p.Count, "kept", st.Kept,
			"cards", len(seen), "new", added, "quiet", st.Quiet,
		)
		switch {
		case len(p.Decks) < mv.PageSize || page*mv.PageSize >= p.Count:
			st.Done, st.Reason = true, "out of decks"
		case st.Quiet >= stop.MaxQuietPages:
			st.Done, st.Reason = true, fmt.Sprintf("%d quiet pages", st.Quiet)
		case st.Quiet >= stop.QuietPages && !hasGap(st.Numbers):
			st.Done, st.Reason = true, fmt.Sprintf("%d quiet pages and no gap", st.Quiet)
		}
		if err := c.Checkpoint(all); err != nil {
			return err
		}
		if st.Done {
			break
		}
	}
	log.Info("set done",
		"set", s.Slug, "reason", st.Reason, "pages", st.Page,
		"kept", st.Kept, "cards", len(seen), "gap", hasGap(st.Numbers),
	)
	return c.Checkpoint(all)
}

// keepNew returns the decks that have a target card not in seen, and adds
// their target cards to seen. It counts the new cards. It adds the number of
// each new card of the set's own expansion to numbers.
func keepNew(
	decks []mv.Deck,
	heads map[string]mv.CardHead,
	seen, numbers map[string]bool,
	expansion int,
) ([]mv.Deck, int) {
	var kept []mv.Deck
	added := 0
	for _, d := range decks {
		var fresh, freshIDs []string
		for _, id := range d.CardIDs {
			h, ok := heads[id]
			if !ok || !IsTarget(h) || seen[TargetKey(h)] || slices.Contains(fresh, TargetKey(h)) {
				continue
			}
			fresh = append(fresh, TargetKey(h))
			freshIDs = append(freshIDs, id)
		}
		if len(fresh) == 0 {
			continue
		}
		kept = append(kept, d)
		for i, id := range freshIDs {
			seen[fresh[i]] = true
			if h := heads[id]; h.Expansion == expansion {
				numbers[h.CardNumber] = true
			}
		}
		added += len(fresh)
	}
	return kept, added
}

// hasGap reports whether the numeric card numbers skip a value below their
// maximum. Lettered numbers (S01, A21) are ignored.
func hasGap(numbers []string) bool {
	present := map[int]bool{}
	top := 0
	for _, s := range numbers {
		n, err := strconv.Atoi(strings.TrimLeft(s, "0"))
		if err != nil {
			continue
		}
		present[n] = true
		top = max(top, n)
	}
	for n := 1; n < top; n++ {
		if !present[n] {
			return true
		}
	}
	return false
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}
