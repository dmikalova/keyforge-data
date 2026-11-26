package inspect

import (
	"encoding/json/jsontext"
	"strings"
	"testing"

	"github.com/dmikalova/keyforge-data/mv"
	"github.com/dmikalova/keyforge-data/raw"
)

func obj(id, body string) mv.Object {
	return mv.Object{ID: id, Raw: jsontext.Value(`{"id":"` + id + `",` + body + `}`)}
}

func TestWriteShowsSpecialCards(t *testing.T) {
	store, err := raw.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cards := []mv.Object{
		obj(
			"a",
			`"card_title":"Troll","house":"Brobnar","card_type":"Creature","power":"8","armor":"0","rarity":"Common","card_number":"001","expansion":341`,
		),
		obj(
			"b",
			`"card_title":"Ortannu","house":"Untamed","card_type":"Gigantic Creature Base","power":"15","rarity":"Rare","card_number":"002","expansion":341`,
		),
		obj(
			"c",
			`"card_title":"Ortannu","house":"Untamed","card_type":"Gigantic Creature Art","power":null,"rarity":"Rare","card_number":"002","expansion":341`,
		),
		obj(
			"d",
			`"card_title":"Skybeast","house":"Skybeasts","card_type":"Creature","power":"X","rarity":"Common","card_number":"003","expansion":341`,
		),
		obj(
			"e",
			`"card_title":"Grunt","house":"Brobnar","card_type":"Token Creature","power":null,"rarity":"Token","card_number":"004","expansion":341`,
		),
	}
	if _, err := store.PutLinked("en", map[string][]mv.Object{"cards": cards}); err != nil {
		t.Fatal(err)
	}
	c, err := store.OpenCrawl("t", "test")
	if err != nil {
		t.Fatal(err)
	}
	deck := mv.Deck{ID: "d1", Raw: jsontext.Value(
		`{"id":"d1","expansion":341,"_links":{"houses":["Brobnar","Dis","Logos"],"cards":["a","d"]}}`,
	)}
	if err := c.WriteDecks("en", "feed", []mv.Deck{deck}); err != nil {
		t.Fatal(err)
	}
	if err := c.Close(nil, true); err != nil {
		t.Fatal(err)
	}

	var b strings.Builder
	if err := Write(&b, store, []string{"t"}); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	for _, want := range []string{
		"## Call of the Archons (CotA, expansion 341)",
		"Card objects: 5. Decks read: 1.",
		"### Houses that are not deck houses (1)",
		"### Gigantic halves (2)",
		"### Tokens (1)",
		"### Creature types with empty or non-numeric power (2)",
		"### Card numbers with more than one card object (2)",
		"### Cards outside the deck's houses (not mavericks) (1)",
		"| 003 | Skybeast | Creature | Skybeasts |",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("report has no %q\n%s", want, out)
		}
	}
}
