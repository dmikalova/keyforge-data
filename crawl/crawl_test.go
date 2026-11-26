package crawl

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dmikalova/keyforge-data/mv"
	"github.com/dmikalova/keyforge-data/raw"
	"github.com/dmikalova/keyforge-data/sets"
)

func TestGreedyFindsSmallCover(t *testing.T) {
	decks := []DeckCards{
		{ID: "a", Cards: []string{"1", "2"}},
		{ID: "b", Cards: []string{"1", "2", "3", "4"}},
		{ID: "c", Cards: []string{"5"}},
		{ID: "d", Cards: []string{"4", "5"}},
	}
	got := Greedy(decks)
	if want := []string{"b", "c"}; !slices.Equal(got, want) {
		t.Fatalf("Greedy = %v, want %v", got, want)
	}
}

func TestGreedyCountsRepeatsOnce(t *testing.T) {
	decks := []DeckCards{
		{ID: "a", Cards: []string{"1", "1", "1"}},
		{ID: "b", Cards: []string{"1", "2"}},
	}
	if got, want := Greedy(decks), []string{"b"}; !slices.Equal(got, want) {
		t.Fatalf("Greedy = %v, want %v", got, want)
	}
}

func TestHasGap(t *testing.T) {
	cases := []struct {
		in   []string
		want bool
	}{
		{nil, false},
		{[]string{"001", "002", "003"}, false},
		{[]string{"001", "003"}, true},
		{[]string{"001", "002", "S01"}, false},
	}
	for _, c := range cases {
		if got := hasGap(c.in); got != c.want {
			t.Errorf("hasGap(%v) = %t, want %t", c.in, got, c.want)
		}
	}
}

func TestKeepNewSkipsMavericksSeenCardsAndCopies(t *testing.T) {
	heads := map[string]mv.CardHead{
		"c1": {ID: "c1", Expansion: 341, CardNumber: "001"},
		"c2": {ID: "c2", Expansion: 341, CardNumber: "002"},
		"m1": {ID: "m1", Expansion: 341, CardNumber: "003", IsMaverick: true},
		"l1": {ID: "l1", Expansion: 435, CardNumber: "010"},
		// e1 is an enhanced copy of c2: a new object of the same printing.
		"e1": {ID: "e1", Expansion: 341, CardNumber: "002"},
		// a1 is an anomaly. Anomalies come in the decks of every set.
		"a1": {ID: "a1", Expansion: 453, CardNumber: "A11"},
	}
	seen := map[string]bool{TargetKey(heads["c1"]): true}
	numbers := map[string]bool{}
	decks := []mv.Deck{
		{ID: "d1", CardIDs: []string{"c1", "m1"}},
		{ID: "d2", CardIDs: []string{"c1", "c2", "c2"}},
		{ID: "d3", CardIDs: []string{"l1"}},
		{ID: "d4", CardIDs: []string{"e1"}},
		{ID: "d5", CardIDs: []string{"a1"}},
	}
	kept, added := keepNew(decks, heads, seen, numbers, 341)
	var ids []string
	for _, d := range kept {
		ids = append(ids, d.ID)
	}
	// d3 has only a card from another set, so it is not kept.
	if want := []string{"d2", "d5"}; !slices.Equal(ids, want) {
		t.Errorf("kept = %v, want %v", ids, want)
	}
	if added != 2 {
		t.Errorf("added = %d, want 2", added)
	}
	if !numbers["002"] || numbers["010"] {
		t.Errorf("numbers = %v, want only the set's own numbers", numbers)
	}
}

// fakeMV serves a feed of decks for one expansion and single decks by ID.
type fakeMV struct {
	mu       sync.Mutex
	decks    [][]string // card IDs of each deck, oldest first
	requests []string
}

func card(id string, n int) string {
	return fmt.Sprintf(
		`{"id":%q,"card_title":"Card %s","expansion":341,"card_number":"%03d","is_maverick":false}`,
		id, id, n,
	)
}

func deckJSON(i int, cards []string) string {
	q := make([]string, len(cards))
	for j, c := range cards {
		q[j] = strconv.Quote(c)
	}
	return fmt.Sprintf(
		`{"id":"deck-%d","name":"Deck %d","expansion":341,"_links":{"cards":[%s]}}`,
		i, i, strings.Join(q, ","),
	)
}

func linkedJSON(cards []string) string {
	var objs []string
	for _, c := range cards {
		n, _ := strconv.Atoi(strings.TrimPrefix(c, "c"))
		objs = append(objs, card(c, n))
	}
	return `{"cards":[` + strings.Join(objs, ",") + `]}`
}

func (f *fakeMV) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.requests = append(f.requests, r.URL.String()+" "+r.Header.Get("Accept-Language"))
	f.mu.Unlock()
	if id, ok := strings.CutPrefix(r.URL.Path, "/api/decks/deck-"); ok {
		i, _ := strconv.Atoi(strings.TrimSuffix(id, "/"))
		_, _ = fmt.Fprintf(
			w,
			`{"data":%s,"_linked":%s}`,
			deckJSON(i, f.decks[i]),
			linkedJSON(f.decks[i]),
		)
		return
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	start := (page - 1) * mv.PageSize
	if start >= len(f.decks) {
		http.NotFound(w, r)
		return
	}
	end := min(start+mv.PageSize, len(f.decks))
	var data []string
	var cards []string
	for i := start; i < end; i++ {
		data = append(data, deckJSON(i, f.decks[i]))
		for _, c := range f.decks[i] {
			if !slices.Contains(cards, c) {
				cards = append(cards, c)
			}
		}
	}
	_, _ = fmt.Fprintf(w, `{"count":%d,"data":[%s],"_linked":%s}`,
		len(f.decks), strings.Join(data, ","), linkedJSON(cards))
}

func testClient(url string) *mv.Client {
	c := mv.New(slog.New(slog.NewTextHandler(io.Discard, nil)))
	c.BaseURL = url + "/api/decks/"
	c.Pacer.Pace = time.Millisecond
	return c
}

func TestFeedSetsCoverAndRefresh(t *testing.T) {
	f := &fakeMV{}
	// Deck 0 has c1-c3; deck 5 adds c4; deck 12 adds c5. The others repeat.
	for i := range 25 {
		cards := []string{"c1", "c2", "c3"}
		switch i {
		case 5:
			cards = []string{"c1", "c4"}
		case 12:
			cards = []string{"c2", "c5"}
		}
		f.decks = append(f.decks, cards)
	}
	srv := httptest.NewServer(f)
	defer srv.Close()

	store, err := raw.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	cotA, _ := sets.BySlug("callofthearchons")
	ctx := context.Background()

	c, err := store.OpenCrawl("t1", ModeFeedSets)
	if err != nil {
		t.Fatal(err)
	}
	stop := StopRule{QuietPages: 5, MaxQuietPages: 10}
	if err := FeedSets(ctx, log, testClient(srv.URL), c, []sets.Set{cotA}, stop); err != nil {
		t.Fatal(err)
	}
	if err := c.Close(nil, true); err != nil {
		t.Fatal(err)
	}

	cv, err := ComputeCover(store, "t1", cotA)
	if err != nil {
		t.Fatal(err)
	}
	if cv.Cards != 5 {
		t.Errorf("cover cards = %d, want 5", cv.Cards)
	}
	if want := []string{"deck-0", "deck-5", "deck-12"}; !slices.Equal(cv.Decks, want) {
		t.Errorf("cover decks = %v, want %v", cv.Decks, want)
	}

	r, err := store.OpenCrawl("r1", ModeRefresh)
	if err != nil {
		t.Fatal(err)
	}
	if err := Refresh(
		ctx,
		log,
		testClient(srv.URL),
		r,
		cv.Decks,
		[]string{"en", "de"},
	); err != nil {
		t.Fatal(err)
	}
	n := 0
	for rec, err := range store.Records("r1") {
		if err != nil {
			t.Fatal(err)
		}
		n++
		if rec.Source != "deck" {
			t.Errorf("source = %q, want deck", rec.Source)
		}
	}
	if n != 6 {
		t.Errorf("refresh records = %d, want 6", n)
	}
	de, err := store.Linked("cards", "de")
	if err != nil {
		t.Fatal(err)
	}
	if len(de) != 5 {
		t.Errorf("de cards = %d, want 5", len(de))
	}
	if !slices.ContainsFunc(
		f.requests,
		func(s string) bool { return strings.HasSuffix(s, " de-de") },
	) {
		t.Error("no request sent Accept-Language de-de")
	}
}

func TestFeedSetsResumes(t *testing.T) {
	f := &fakeMV{}
	for i := range 30 {
		f.decks = append(f.decks, []string{"c" + strconv.Itoa(i+1)})
	}
	srv := httptest.NewServer(f)
	defer srv.Close()
	store, err := raw.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	cotA, _ := sets.BySlug("callofthearchons")

	// Stop the first run after two pages.
	ctx, cancel := context.WithCancel(context.Background())
	client := testClient(srv.URL)
	c, err := store.OpenCrawl("t", ModeFeedSets)
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			f.mu.Lock()
			n := len(f.requests)
			f.mu.Unlock()
			if n >= 2 {
				cancel()
				return
			}
			time.Sleep(time.Millisecond)
		}
	}()
	err = FeedSets(ctx, log, client, c, []sets.Set{cotA}, DefaultStop)
	if err == nil {
		t.Fatal("first run did not stop")
	}
	if err := c.Close(nil, false); err != nil {
		t.Fatal(err)
	}

	c, err = store.OpenCrawl("t", ModeFeedSets)
	if err != nil {
		t.Fatal(err)
	}
	if err := FeedSets(
		context.Background(),
		log,
		client,
		c,
		[]sets.Set{cotA},
		DefaultStop,
	); err != nil {
		t.Fatal(err)
	}
	if err := c.Close(nil, true); err != nil {
		t.Fatal(err)
	}
	ids := map[string]int{}
	for rec, err := range store.Records("t") {
		if err != nil {
			t.Fatal(err)
		}
		d, err := deckOf(rec)
		if err != nil {
			t.Fatal(err)
		}
		ids[d.ID]++
	}
	if len(ids) != 30 {
		t.Errorf("decks = %d, want 30", len(ids))
	}
	for id, n := range ids {
		if n != 1 {
			t.Errorf("deck %s stored %d times", id, n)
		}
	}
}
