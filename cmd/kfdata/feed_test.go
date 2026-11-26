package main

import (
	"strings"
	"testing"
	"time"

	"github.com/dmikalova/keyforge-data/card"
)

func TestDiff(t *testing.T) {
	old := []card.Card{
		{ID: "a", Title: "A"},
		{ID: "b", Title: "B", Text: "old"},
		{ID: "c", Title: "C"},
	}
	cur := []card.Card{
		{ID: "a", Title: "A"},
		{ID: "b", Title: "B", Text: "new"},
		{ID: "d", Title: "D & E"},
	}
	ch := diff(old, cur)
	if got := ch.summary(); got != "1 added, 1 changed, 1 removed" {
		t.Errorf("summary = %q", got)
	}
	h := ch.html()
	for _, want := range []string{
		`<a href="https://search.keyforge.cards/?card=d">D &amp; E</a>`,
		`<li>C</li>`,
	} {
		if !strings.Contains(h, want) {
			t.Errorf("html %q does not have %q", h, want)
		}
	}
	if got := diff(cur, cur).summary(); got != "no card changes" {
		t.Errorf("same dataset summary = %q", got)
	}
}

func TestUpdateFeed(t *testing.T) {
	now := time.Date(2026, 11, 26, 0, 0, 0, 0, time.UTC)
	f := atomFeed{Entries: []atomEntry{
		{ID: "urn:keyforge-cards:dataset:v2", Updated: now.AddDate(0, 0, -1).Format(time.RFC3339)},
		{
			ID:      "urn:keyforge-cards:dataset:v1",
			Updated: now.AddDate(0, 0, -100).Format(time.RFC3339),
		},
	}}
	d := card.Dataset{Version: "v3", Generated: now}
	f = updateFeed(f, feedEntry(d, change{}), now)
	var ids []string
	for _, e := range f.Entries {
		ids = append(ids, e.ID)
	}
	if got := strings.Join(
		ids,
		" ",
	); got != "urn:keyforge-cards:dataset:v3 urn:keyforge-cards:dataset:v2" {
		t.Errorf("entries = %s, want v3 and v2 (v1 is older than 90 days)", got)
	}
	if f.Entries[0].Link[0].Href != publicBase+card.VersionPath("v3") {
		t.Errorf("link = %s", f.Entries[0].Link[0].Href)
	}
}
