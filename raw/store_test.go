package raw

import (
	"encoding/json/jsontext"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/dmikalova/keyforge-data/mv"
)

func decks(from, n int) []mv.Deck {
	out := make([]mv.Deck, n)
	for i := range n {
		id := fmt.Sprintf("d%d", from+i)
		out[i] = mv.Deck{ID: id, Raw: jsontext.Value(fmt.Sprintf(`{"id":%q}`, id))}
	}
	return out
}

func count(t *testing.T, s *Store, id string) int {
	t.Helper()
	n := 0
	for _, err := range s.Records(id) {
		if err != nil {
			t.Fatal(err)
		}
		n++
	}
	return n
}

func TestCrawlShardsAndReadsBack(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.OpenCrawl("x", "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := c.WriteDecks("en", "feed", decks(0, ShardSize+5)); err != nil {
		t.Fatal(err)
	}
	if err := c.Close(nil, true); err != nil {
		t.Fatal(err)
	}
	if got := len(c.Manifest.Shards); got != 2 {
		t.Errorf("shards = %d, want 2", got)
	}
	if got := count(t, s, "x"); got != ShardSize+5 {
		t.Errorf("records = %d, want %d", got, ShardSize+5)
	}
}

func TestCrawlDropsLinesAfterTheCheckpoint(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.OpenCrawl("x", "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := c.WriteDecks("en", "feed", decks(0, 3)); err != nil {
		t.Fatal(err)
	}
	if err := c.Checkpoint(map[string]int{"page": 1}); err != nil {
		t.Fatal(err)
	}
	// Two more decks and half a line reach the disk, but no checkpoint.
	if err := c.WriteDecks("en", "feed", decks(3, 2)); err != nil {
		t.Fatal(err)
	}
	if err := c.flush(); err != nil {
		t.Fatal(err)
	}
	if _, err := c.open.WriteString(`{"fetched":`); err != nil {
		t.Fatal(err)
	}
	_ = c.open.Close()

	c, err = s.OpenCrawl("x", "test")
	if err != nil {
		t.Fatal(err)
	}
	var st map[string]int
	if err := c.State(&st); err != nil {
		t.Fatal(err)
	}
	if st["page"] != 1 {
		t.Errorf("state = %v, want page 1", st)
	}
	if got := count(t, s, "x"); got != 3 {
		t.Errorf("records = %d, want 3", got)
	}
	if err := c.Close(nil, true); err != nil {
		t.Fatal(err)
	}
}

func TestOpenCrawlRefusesAnotherMode(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.OpenCrawl("x", "a")
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Close(nil, false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.OpenCrawl("x", "b"); err == nil {
		t.Fatal("OpenCrawl with another mode did not fail")
	}
}

func TestPutLinkedKeepsEachVersionOnce(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	v1 := map[string][]mv.Object{"cards": {{ID: "c1", Raw: jsontext.Value(`{"id":"c1","p":1}`)}}}
	v2 := map[string][]mv.Object{"cards": {{ID: "c1", Raw: jsontext.Value(`{"id":"c1","p":2}`)}}}
	for _, l := range []map[string][]mv.Object{v1, v1, v2} {
		if _, err := s.PutLinked("en", l); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.Linked("cards", "en")
	if err != nil {
		t.Fatal(err)
	}
	if len(got["c1"]) != 2 {
		t.Errorf("versions = %d, want 2", len(got["c1"]))
	}
	files, err := filepath.Glob(filepath.Join(s.Root, "linked", "cards", "en", "c1", "*.tmp"))
	if err != nil || len(files) != 0 {
		t.Errorf("temporary files left: %v %v", files, err)
	}
	if _, err := os.Stat(filepath.Join(s.Root, "linked", "cards", "de")); err == nil {
		t.Error("a language directory exists with no objects")
	}
}
