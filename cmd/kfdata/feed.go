package main

import (
	"bytes"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"html"
	"io"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/dmikalova/keyforge-data/card"
)

// The public URLs of the dataset files (ADR 0007).
const (
	publicBase = "https://storage.googleapis.com/mklv-keyforge-cards-public/"
	searchBase = "https://gallery.keyforge.cards/"
	// feedDays is how long a feed entry stays. A bucket lifecycle rule
	// deletes the version files after 90 days, so an older entry has no
	// file.
	feedDays = 90
)

// atomFeed is data/feed.xml.
type atomFeed struct {
	XMLName xml.Name    `xml:"http://www.w3.org/2005/Atom feed"`
	Title   string      `xml:"title"`
	ID      string      `xml:"id"`
	Updated string      `xml:"updated"`
	Link    []atomLink  `xml:"link"`
	Entries []atomEntry `xml:"entry"`
}

type atomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr,omitempty"`
	Type string `xml:"type,attr,omitempty"`
}

type atomEntry struct {
	Title   string      `xml:"title"`
	ID      string      `xml:"id"`
	Updated string      `xml:"updated"`
	Link    []atomLink  `xml:"link"`
	Content atomContent `xml:"content"`
}

type atomContent struct {
	Type string `xml:"type,attr"`
	Body string `xml:",chardata"`
}

// change is the difference between two dataset versions, by card.
type change struct {
	added, changed, removed []card.Card
}

// diff returns the cards that the new dataset adds, changes, and removes.
func diff(old, cur []card.Card) change {
	before := map[string]card.Card{}
	for _, c := range old {
		before[c.ID] = c
	}
	var ch change
	seen := map[string]bool{}
	for _, c := range cur {
		seen[c.ID] = true
		o, ok := before[c.ID]
		switch {
		case !ok:
			ch.added = append(ch.added, c)
		case !sameCard(o, c):
			ch.changed = append(ch.changed, c)
		}
	}
	for _, c := range old {
		if !seen[c.ID] {
			ch.removed = append(ch.removed, c)
		}
	}
	return ch
}

func sameCard(a, b card.Card) bool {
	x, errA := json.Marshal(a, json.Deterministic(true))
	y, errB := json.Marshal(b, json.Deterministic(true))
	return errA == nil && errB == nil && bytes.Equal(x, y)
}

// summary returns the entry title: "12 added, 3 changed, 1 removed".
func (ch change) summary() string {
	var parts []string
	for _, p := range []struct {
		n    int
		verb string
	}{{len(ch.added), "added"}, {len(ch.changed), "changed"}, {len(ch.removed), "removed"}} {
		if p.n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", p.n, p.verb))
		}
	}
	if len(parts) == 0 {
		return "no card changes"
	}
	return strings.Join(parts, ", ")
}

// html returns the entry content: a list of the cards of each kind, with
// links to the search site. A removed card has no link.
func (ch change) html() string {
	var b strings.Builder
	list := func(head string, cards []card.Card, link bool) {
		if len(cards) == 0 {
			return
		}
		fmt.Fprintf(&b, "<h3>%s</h3><ul>", head)
		for _, c := range cards {
			name := html.EscapeString(c.Title)
			if link {
				fmt.Fprintf(&b, `<li><a href="%s?card=%s">%s</a></li>`,
					searchBase, url.QueryEscape(c.ID), name)
			} else {
				fmt.Fprintf(&b, "<li>%s</li>", name)
			}
		}
		b.WriteString("</ul>")
	}
	list("Added", ch.added, true)
	list("Changed", ch.changed, true)
	list("Removed", ch.removed, false)
	return b.String()
}

// feedEntry returns the feed entry of a dataset version.
func feedEntry(d card.Dataset, ch change) atomEntry {
	at := d.Generated.UTC().Format(time.RFC3339)
	return atomEntry{
		Title:   "Dataset " + d.Version + ": " + ch.summary(),
		ID:      "urn:keyforge-cards:dataset:" + d.Version,
		Updated: at,
		Link: []atomLink{
			{Href: publicBase + card.VersionPath(d.Version), Type: "application/json"},
		},
		Content: atomContent{Type: "html", Body: ch.html()},
	}
}

// updateFeed adds the entry of a version to the front of the feed and drops
// the entries older than feedDays. An entry of the same version is
// replaced.
func updateFeed(f atomFeed, e atomEntry, now time.Time) atomFeed {
	f.Title = "KeyForge card dataset"
	f.ID = "urn:keyforge-cards:dataset"
	f.Updated = e.Updated
	f.Link = []atomLink{
		{Href: publicBase + "data/feed.xml", Rel: "self", Type: "application/atom+xml"},
		{Href: searchBase},
	}
	keep := []atomEntry{e}
	cutoff := now.AddDate(0, 0, -feedDays)
	for _, old := range f.Entries {
		t, err := time.Parse(time.RFC3339, old.Updated)
		if old.ID == e.ID || err != nil || t.Before(cutoff) {
			continue
		}
		keep = append(keep, old)
	}
	f.Entries = keep
	return f
}

// feedCmd runs "kfdata feed": it writes data/latest.json and data/feed.xml
// for a new dataset, from the published dataset and the published feed.
func feedCmd(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("feed", flag.ContinueOnError)
	newFile := fs.String("dataset", "tmp/cards.json", "the new dataset")
	oldFile := fs.String("old", "", "the published dataset (optional)")
	oldFeed := fs.String("old-feed", "", "the published feed (optional)")
	outFeed := fs.String("out-feed", "tmp/feed.xml", "the feed to write")
	outLatest := fs.String("out-latest", "tmp/latest.json", "the pointer to write")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cur, err := readFullDataset(*newFile)
	if err != nil {
		return err
	}
	var old card.Dataset
	if *oldFile != "" {
		if old, err = readFullDataset(*oldFile); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	var f atomFeed
	if *oldFeed != "" {
		b, err := os.ReadFile(*oldFeed)
		switch {
		case err == nil:
			if err := xml.Unmarshal(b, &f); err != nil {
				return fmt.Errorf("%s: %w", *oldFeed, err)
			}
		case !errors.Is(err, os.ErrNotExist):
			return err
		}
	}
	ch := diff(old.Cards, cur.Cards)
	f = updateFeed(f, feedEntry(cur, ch), cur.Generated)
	x, err := xml.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(
		*outFeed,
		append(
			append(make([]byte, 0, len(xml.Header)+len(append(x, '\n'))), xml.Header...),
			append(x, '\n')...),
		0o600,
	); err != nil {
		return err
	}
	l := card.Latest{
		Version: cur.Version, URL: publicBase + card.VersionPath(cur.Version),
		Generated: cur.Generated, Cards: len(cur.Cards), Sets: len(cur.Sets),
	}
	b, err := json.Marshal(l, jsontext.Multiline(true), jsontext.WithIndent("  "))
	if err != nil {
		return err
	}
	if err := os.WriteFile(*outLatest, append(b, '\n'), 0o600); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(stdout, "%s: %s\n", cur.Version, ch.summary())
	return nil
}

// readFullDataset reads a dataset file.
func readFullDataset(file string) (card.Dataset, error) {
	var d card.Dataset
	b, err := os.ReadFile(file)
	if err != nil {
		return d, err
	}
	err = json.Unmarshal(b, &d)
	return d, err
}
