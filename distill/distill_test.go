package distill

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/dmikalova/keyforge-data/card"
)

func obj(id, body string) []byte {
	return []byte(`{"id":"` + id + `",` + body + `}`)
}

func find(t *testing.T, d card.Dataset, id string) card.Card {
	t.Helper()
	i := slices.IndexFunc(d.Cards, func(c card.Card) bool { return c.ID == id })
	if i < 0 {
		var ids []string
		for _, c := range d.Cards {
			ids = append(ids, c.ID)
		}
		t.Fatalf("no card %q in %v", id, ids)
	}
	return d.Cards[i]
}

func kinds(ps []Problem) []string {
	var out []string
	for _, p := range ps {
		out = append(out, p.Kind+" "+p.Set+" "+p.Number)
	}
	return out
}

func TestRun(t *testing.T) {
	in := Input{
		Cards: [][]byte{
			obj(
				"t1",
				`"card_title":"Troll","house":"Brobnar","card_type":"Creature","card_text":"Reap: Troll heals 3 damage.","traits":"Giant","power":"8","armor":"0","rarity":"Common","card_number":"046","expansion":341`,
			),
			obj(
				"t2",
				`"card_title":"Troll","house":"Brobnar","card_type":"Creature","card_text":"Reap: Troll heals 3 damage.","traits":"Giant","power":"8","armor":"0","rarity":"Common","card_number":"046","expansion":341,"is_enhanced":true`,
			),
			obj(
				"t3",
				`"card_title":"Troll","house":"Brobnar","card_type":"Creature","card_text":"Reap: Troll heals 3 damage.","traits":"Giant","power":"8","armor":"0","rarity":"Rare","card_number":"012","expansion":435`,
			),
			obj(
				"tm",
				`"card_title":"Troll","house":"Dis","card_type":"Creature","power":"8","rarity":"Common","card_number":"046","expansion":341,"is_maverick":true`,
			),
			obj(
				"g1",
				`"card_title":"Ultra Gravitron","house":"Logos","card_type":"Gigantic Creature Base","card_text":"Taunt.","power":"10","armor":"3","rarity":"Rare","card_number":"125","expansion":479`,
			),
			obj(
				"g2",
				`"card_title":"Ultra Gravitron","house":"Logos","card_type":"Gigantic Creature Art","power":null,"rarity":"FIXED","card_number":"125","expansion":479`,
			),
			obj(
				"a1",
				`"card_title":"Ghostform","house":"Dis","card_type":"Upgrade","rarity":"FIXED","card_number":"A01","expansion":453,"is_anomaly":true`,
			),
			obj(
				"a2",
				`"card_title":"Ghostform","house":"Logos","card_type":"Upgrade","rarity":"FIXED","card_number":"A01","expansion":453,"is_anomaly":true`,
			),
			obj(
				"q1",
				`"card_title":"Timequake","house":"Dis","card_type":"Action","rarity":"FIXED","card_number":"A09","expansion":453,"is_anomaly":true`,
			),
			obj(
				"q2",
				`"card_title":"Timequake","house":"Logos","card_type":"Action","rarity":"Rare","card_number":"300","expansion":479,"is_anomaly":true`,
			),
			obj(
				"m1",
				`"card_title":"Mimic Gel","house":"Logos","card_type":"Creature","power":null,"rarity":"Rare","card_number":"170","expansion":452`,
			),
			obj(
				"x1",
				`"card_title":"Picaroon","house":"Dis","card_type":"Creature","power":"X","rarity":"Uncommon","card_number":"028","expansion":479`,
			),
			obj(
				"r1",
				`"card_title":"Forge Ahead","house":"Prophecy","card_type":"Prophecy","rarity":"Special","card_number":"P01","expansion":886`,
			),
			obj(
				"s1",
				`"card_title":"Sky Whale","house":"Skybeast","card_type":"Creature","power":"6","rarity":"Rare","card_number":"400","expansion":800`,
			),
			obj(
				"b1",
				`"card_title":"ANT1-10NY","house":"Logos","card_type":"Creature","power":"3","rarity":"Rare","card_number":"100","expansion":800`,
			),
		},
		Images: []string{
			"callofthearchons/046.webp", "ageofascension/012.webp", "massmutation/125.webp",
			"worldscollide/A01.webp", "worldscollide/170.webp",
			"aemberskies/400-other-card.webp", "aemberskies/400-sky-whale.webp",
		},
		Overrides: map[string]Override{
			"worldscollide": {Cards: map[string]CardFix{"170": {Power: &card.Power{X: true}}}},
			"aemberskies":   {Aliases: map[string][]string{"ANT1-10NY": {"Anthony"}}},
		},
		Now: time.Date(2026, 11, 26, 0, 0, 0, 0, time.UTC),
	}
	d, problems := Run(in)

	troll := find(t, d, "troll-brobnar")
	if len(troll.Printings) != 2 || troll.Printings[0].Set != "callofthearchons" ||
		troll.Printings[1].Rarity != "Rare" {
		t.Errorf("troll printings: %+v", troll.Printings)
	}
	if got := troll.Printings[0].MVIDs; !slices.Equal(got, []string{"t1", "t2"}) {
		t.Errorf("troll MV IDs: %v (the maverick must not count)", got)
	}
	if troll.Power == nil || troll.Power.N != 8 || !slices.Equal(troll.Traits, []string{"giant"}) {
		t.Errorf("troll: %+v", troll)
	}

	g := find(t, d, "ultragravitron-logos")
	if g.Type != "creature" || !g.HasFlag(FlagGigantic) || g.Power.N != 10 || g.Armor != 3 ||
		g.Printings[0].Rarity != "Rare" || g.Keywords["taunt"] != 0 || len(g.Keywords) != 1 {
		t.Errorf("gigantic: %+v", g)
	}

	gf := find(t, d, "ghostform-none")
	if !gf.HasFlag(FlagMultiHouse) || !gf.HasFlag(FlagHouseless) || !gf.Printings[0].Anomaly ||
		gf.Printings[0].Set != "worldscollide" || gf.Printings[0].Image != "worldscollide/A01.webp" {
		t.Errorf("anomaly: %+v", gf)
	}
	// An anomaly with a printing in a playable set drops its expansion 453
	// objects.
	if tq := find(
		t,
		d,
		"timequake-logos",
	); len(tq.Printings) != 1 ||
		tq.Printings[0].Set != "massmutation" {
		t.Errorf("timequake: %+v", tq.Printings)
	}
	if p := find(t, d, "mimicgel-logos").Power; p == nil || !p.X {
		t.Errorf("override power X: %v", p)
	}
	if p := find(t, d, "picaroon-dis").Power; p == nil || !p.X {
		t.Errorf("power X: %v", p)
	}
	ref := find(t, d, "forgeahead-none")
	if ref.HasFlag(FlagHouseless) {
		t.Errorf("a reference card is not houseless")
	}
	if !ref.HasFlag(FlagReference) || ref.Type != "prophecy" {
		t.Errorf("reference: %+v", ref)
	}
	sky := find(t, d, "skywhale-none")
	if sky.Printings[0].Image != "aemberskies/400-sky-whale.webp" || !sky.HasFlag("skybeast") ||
		!sky.HasFlag(FlagHouseless) {
		t.Errorf("skybeast: %+v", sky)
	}
	ant := find(t, d, "ant110ny-logos")
	if !slices.Equal(ant.Aliases, []string{"anthony", "anti-iony"}) {
		t.Errorf("aliases: %v", ant.Aliases)
	}

	// Reference cards sort last, after the cards without a house.
	if last := d.Cards[len(d.Cards)-1]; last.ID != "forgeahead-none" {
		t.Errorf("last card is %s", last.ID)
	}
	if len(d.Sets) != 6 || d.Sets[0].Slug != "callofthearchons" || d.Version == "" {
		t.Errorf("sets %v version %q", d.Sets, d.Version)
	}

	got := strings.Join(kinds(problems), "\n")
	for _, want := range []string{
		"missing image aemberskies 100",
		"missing image massmutation 028",
		"unlabeled houseless kind aemberskies 400",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("problems have no %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "creature without power worldscollide") {
		t.Errorf("the override must clear the power problem:\n%s", got)
	}

	again, _ := Run(in)
	if again.Version != d.Version {
		t.Errorf("version is not stable: %s %s", d.Version, again.Version)
	}
}

func TestSlug(t *testing.T) {
	for in, want := range map[string]string{
		"It’s Coming...":    "its-coming",
		"Veylan Analyst":    "veylan-analyst",
		"Æmber Imp":         "aember-imp",
		"Ortannu’s Binding": "ortannus-binding",
	} {
		if got := Slug(in); got != want {
			t.Errorf("Slug(%q) = %q, want %q", in, got, want)
		}
	}
}
