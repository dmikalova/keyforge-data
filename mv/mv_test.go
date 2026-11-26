package mv

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

const feedPage = `{
  "count": 2,
  "data": [
    {"id": "d1", "expansion": 341, "_links": {"cards": ["c1", "c2", "c2"]}},
    {"id": "d2", "expansion": 341, "_links": {"cards": ["c1"]}}
  ],
  "_linked": {
    "cards": [
      {"id": "c1", "expansion": 341, "card_number": "001", "is_maverick": false},
      {"id": "c2", "expansion": 341, "card_number": "002", "is_maverick": true}
    ],
    "houses": [{"id": "Dis", "name": "Dis"}]
  }
}`

func TestParsePageKeepsRawObjects(t *testing.T) {
	p, err := ParsePage([]byte(feedPage))
	if err != nil {
		t.Fatal(err)
	}
	if p.Count != 2 || len(p.Decks) != 2 {
		t.Fatalf("count %d, decks %d", p.Count, len(p.Decks))
	}
	if got := p.Decks[0].CardIDs; len(got) != 3 {
		t.Errorf("deck card IDs = %v", got)
	}
	if len(p.Linked["houses"]) != 1 || p.Linked["houses"][0].ID != "Dis" {
		t.Errorf("houses = %v", p.Linked["houses"])
	}
	heads, err := p.Cards()
	if err != nil {
		t.Fatal(err)
	}
	if !heads["c2"].IsMaverick || heads["c1"].CardNumber != "001" {
		t.Errorf("heads = %v", heads)
	}
}

func quiet() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func fastClient(url string) *Client {
	c := New(quiet())
	c.BaseURL = url + "/"
	c.Pacer.Pace = time.Millisecond
	c.Pacer.Longest = time.Millisecond
	return c
}

func TestGetRetriesThrottleAndRaisesPace(t *testing.T) {
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if n.Add(1) <= 2 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = io.WriteString(w, feedPage)
	}))
	defer srv.Close()
	c := fastClient(srv.URL)
	if _, err := c.Feed(context.Background(), FeedQuery{Expansion: 341, Page: 1}); err != nil {
		t.Fatal(err)
	}
	if n.Load() != 3 {
		t.Errorf("requests = %d, want 3", n.Load())
	}
	if c.Pacer.Pace != 2*time.Second+time.Millisecond {
		t.Errorf("pace = %s, want two seconds more", c.Pacer.Pace)
	}
}

func TestGetMapsLimitAndNotFound(t *testing.T) {
	for status, want := range map[int]error{
		http.StatusForbidden: ErrPastLimit,
		http.StatusNotFound:  ErrNotFound,
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(status)
		}))
		_, err := fastClient(srv.URL).Feed(context.Background(), FeedQuery{Page: 5001})
		srv.Close()
		if !errors.Is(err, want) {
			t.Errorf("status %d: err = %v, want %v", status, err, want)
		}
	}
}

func TestPacerCapsPaceAndBackoff(t *testing.T) {
	p := NewPacer()
	for range 30 {
		p.Throttled()
	}
	if p.Pace != p.MaxPace {
		t.Errorf("pace = %s, want %s", p.Pace, p.MaxPace)
	}
	b := p.backoff()
	var last time.Duration
	for range 10 {
		last = b.next()
	}
	if last != p.MaxBackoff || p.Longest != p.MaxBackoff {
		t.Errorf("last = %s, longest = %s, want %s", last, p.Longest, p.MaxBackoff)
	}
}
