package distill

import (
	"maps"
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
				"s1",
				`"card_title":"Akugyo","house":"Brobnar","card_type":"Creature","power":"5","rarity":"Special","card_number":"S01","expansion":800`,
			),
			obj(
				"s2",
				`"card_title":"Akugyo","house":"Mars","card_type":"Creature","power":"5","rarity":"Special","card_number":"S01","expansion":800`,
			),
			obj(
				"c1",
				`"card_title":"Ecto-Charge","house":"Dis","card_type":"Action","rarity":"Special","card_number":"A11","expansion":453,"is_anomaly":true`,
			),
			obj(
				"n1",
				`"card_title":"Lost Anomaly","house":"Dis","card_type":"Action","rarity":"FIXED","card_number":"A99","expansion":453,"is_anomaly":true`,
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
				"e1",
				`"card_title":"Exchange Officer","house":"Star Alliance","card_type":"Creature","power":"3","rarity":"Rare","card_number":"341","expansion":479`,
			),
			obj(
				"e2",
				`"card_title":"Exchange Officer","house":"Dis","card_type":"Creature","power":"3","rarity":"Rare","card_number":"282","expansion":609,"is_maverick":true`,
			),
			obj(
				"e3",
				`"card_title":"Exchange Officer","house":"Logos","card_type":"Creature","power":"3","rarity":"Rare","card_number":"282","expansion":609,"is_maverick":true`,
			),
			obj(
				"o1",
				`"card_title":"Lonely Maverick","house":"Dis","card_type":"Action","rarity":"Rare","card_number":"099","expansion":609,"is_maverick":true`,
			),
			obj(
				"h1",
				`"card_title":"Horseman of Famine","house":"Sanctum","card_type":"Creature","card_text":"Play/Fight/Reap: Gain 1A.","power":"3","rarity":"FIXED","card_number":"247","expansion":341`,
			),
			obj(
				"dc1",
				`"card_title":"Dark Centurion","house":"Saurian","card_type":"Creature","power":"5","rarity":"Uncommon","card_number":"203","expansion":479`,
			),
			obj(
				"dc2",
				`"card_title":"Dark Centurion","house":"Redemption","card_type":"Creature","power":"5","rarity":"Uncommon","card_number":"152","expansion":855`,
			),
			obj(
				"bd1",
				`"card_title":"Bordan the Redeemed","house":"Redemption","card_type":"Creature","power":"6","rarity":"Rare","card_number":"150","expansion":855`,
			),
			obj(
				"bp1",
				`"card_title":"Blypyp","house":"Mars","card_type":"Creature","power":"2","rarity":"Common","card_number":"180","expansion":435`,
			),
			obj(
				"bp2",
				`"card_title":"Blypyp","house":"Elders","card_type":"Creature","power":"2","rarity":"Common","card_number":"010","expansion":892`,
			),
			obj(
				"gr1",
				`"card_title":"The Grim Reaper","house":"Geistoid","card_type":"Creature","power":"6","rarity":"Rare","card_number":"155","expansion":700`,
			),
			obj(
				"gr2",
				`"card_title":"The Grim Reaper","house":"Brobnar","card_type":"Creature","power":"6","rarity":"Rare","card_number":"233","expansion":722,"is_maverick":true`,
			),
			obj(
				"sh1",
				`"card_title":"Shooler","house":"Dis","card_type":"Creature","power":"5","rarity":"Common","card_number":"080","expansion":341`,
			),
			obj(
				"sh2",
				`"card_title":"Shooler","house":"Dis","card_type":"Creature","power":"5","rarity":"Common","card_number":"090","expansion":722`,
			),
			obj(
				"v1",
				`"card_title":"Master of 2","house":"Brobnar","card_type":"Creature","power":"4","rarity":"Variant","card_number":"090","expansion":341`,
			),
			obj(
				"w1",
				`"card_title":"Old Egad","house":"Logos","card_type":"Creature","power":"1","rarity":"Rare","card_number":"100","expansion":496`,
			),
			obj(
				"w2",
				`"card_title":"Old Egad","house":"Logos","card_type":"Evil Twin Creature","power":"2","rarity":"Evil Twin","card_number":"004","expansion":496`,
			),
			obj(
				"k1",
				`"card_title":"Berserker","house":"Brobnar","card_type":"Token Creature","power":"2","rarity":"Token","card_number":"T01","expansion":800`,
			),
			obj(
				"b1",
				`"card_title":"ANT1-10NY","house":"Logos","card_type":"Creature","power":"3","rarity":"Rare","card_number":"100","expansion":800`,
			),
		},
		Images: []string{
			"callofthearchons/046.webp?v=7", "ageofascension/012.webp", "massmutation/125.webp",
			"worldscollide/A01.webp", "worldscollide/170.webp",
			"aemberskies/400-other-card.webp", "aemberskies/400-sky-whale.webp",
			"grimreminders/155.webp", "menagerie/233.webp",
			"callofthearchons/080.webp", "menagerie/090.webp",
		},
		Overrides: map[string]Override{
			"worldscollide": {Cards: map[string]CardFix{"170": {Power: &card.Power{X: true}}}},
			"aemberskies": {
				Aliases:      map[string][]string{"ANT1-10NY": {"Anthony"}},
				KindPrefixes: map[string]string{"S": "Skybeast"},
			},
			"menagerie": {GreyFrame: []string{"090"}},
		},
		Decks:    map[string][]string{"b1": {"deck-1"}},
		DeckSets: map[string][]int{"a1": {479, 452}, "a2": {452}, "q1": {452}, "c1": {600}},
		Now:      time.Date(2026, 11, 26, 0, 0, 0, 0, time.UTC),
	}
	d, problems := Run(in)

	troll := find(t, d, "troll-brobnar")
	if len(troll.Printings) != 2 || troll.Printings[0].Set != "callofthearchons" ||
		troll.Printings[1].Rarity != "Rare" {
		t.Errorf("troll printings: %+v", troll.Printings)
	}
	if img := troll.Printings[0].Image; img != "callofthearchons/046.webp?v=7" {
		t.Errorf("troll image %q keeps the version", img)
	}
	if got := troll.Printings[0].MVIDs; !slices.Equal(got, []string{"t1", "t2"}) {
		t.Errorf("troll MV IDs: %v (the maverick must not count)", got)
	}
	if troll.Power == nil || troll.Power.N != 8 || !slices.Equal(troll.Traits, []string{"giant"}) {
		t.Errorf("troll: %+v", troll)
	}

	if ew := find(
		t,
		d,
		"oldegad-logos-eviltwin",
	); len(ew.Related) != 1 ||
		ew.Related[0].Label != "Evil twin" ||
		ew.Related[0].Cards[0] != "oldegad-logos" {
		t.Errorf("evil twin related: %+v", ew.Related)
	}
	// A card that Master Vault prints in the house of the deck gets the kind
	// of its number prefix.
	if sb := find(t, d, "akugyo-none"); sb.Group != "skybeast" ||
		!sb.HasFlag("skybeast") || !sb.HasFlag(FlagHouseless) {
		t.Errorf("skybeast: group %q flags %v", sb.Group, sb.Flags)
	}
	if gf := find(t, d, "ghostform-none"); gf.Group != "anomaly" {
		t.Errorf("anomaly group %q", gf.Group)
	}
	if ref := find(t, d, "forgeahead-none"); ref.Group != "prophecy" {
		t.Errorf("prophecy group %q", ref.Group)
	}
	if ab := find(t, d, "troll-brobnar").Abilities; !slices.Equal(ab, []string{"reap"}) {
		t.Errorf("troll abilities: %v", ab)
	}

	g := find(t, d, "ultragravitron-logos")
	if g.Type != "creature" || !g.HasFlag(FlagGigantic) || g.Power.N != 10 || g.Armor != 3 ||
		g.Printings[0].Rarity != "Rare" || g.Keywords["taunt"] != 0 || len(g.Keywords) != 1 {
		t.Errorf("gigantic: %+v", g)
	}

	gf := find(t, d, "ghostform-none")
	if gf.Printings[0].Rarity != "Anomaly" {
		t.Errorf("anomaly rarity: %q", gf.Printings[0].Rarity)
	}
	// Master Vault gives most anomalies the rarity Special. Every anomaly
	// printing is rarity Anomaly.
	if ec := find(t, d, "ectocharge-none"); ec.Printings[0].Rarity != "Anomaly" {
		t.Errorf("ecto-charge rarity: %q, want Anomaly", ec.Printings[0].Rarity)
	}
	if !gf.HasFlag(FlagHouseless) || !gf.Printings[0].Anomaly ||
		gf.Printings[0].Set != "worldscollide" || gf.Printings[0].Image != "worldscollide/A01.webp" {
		t.Errorf("anomaly: %+v", gf)
	}
	// An anomaly takes the oldest set whose decks have it.
	if ec := find(
		t,
		d,
		"ectocharge-none",
	); ec.Printings[0].Set != "windsofexchange" ||
		ec.Printings[0].Number != "A11" {
		t.Errorf("ecto-charge: %+v", ec.Printings)
	}
	// An anomaly with a printing in a playable set keeps its anomaly
	// printing in the oldest set whose decks have it, on the same card.
	tq := find(t, d, "timequake-logos")
	if len(tq.Printings) != 2 ||
		tq.Printings[0].Set != "worldscollide" || !tq.Printings[0].Anomaly ||
		tq.Printings[0].Rarity != "Anomaly" || tq.Printings[0].Number != "A09" ||
		tq.Printings[1].Set != "massmutation" ||
		tq.House != "logos" || tq.HasFlag(FlagHouseless) || tq.Group != "" {
		t.Errorf("timequake: %+v", tq)
	}
	for _, c := range d.Cards {
		if c.ID == "timequake-none" {
			t.Errorf("the anomaly printing made a second card: %+v", c)
		}
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
	if !ref.HasFlag(FlagReference) || ref.Type != "reference" {
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

	// A printing with only maverick objects takes the house of the card's
	// other printings.
	eo := find(t, d, "exchangeofficer-staralliance")
	if len(eo.Printings) != 2 || eo.Printings[1].Set != "vaultmasters2023" ||
		len(eo.Printings[1].MVIDs) != 2 {
		t.Errorf("exchange officer: %+v", eo.Printings)
	}

	hf := find(t, d, "horsemanoffamine-sanctum")
	if hf.Printings[0].Rarity != "Special" || hf.Text != "Play/Fight/Reap: Gain 1 Æmber." {
		t.Errorf("horseman: %q %q", hf.Printings[0].Rarity, hf.Text)
	}

	if ew := find(
		t,
		d,
		"oldegad-logos-eviltwin",
	); !ew.HasFlag(FlagEvilTwin) || ew.Type != "creature" ||
		ew.Power.N != 2 || ew.Printings[0].Rarity != "Rare" {
		t.Errorf("evil twin: %+v", ew)
	}
	// A Redemption printing of a card of another house joins that card,
	// with the printing's house. A Redemption-only card stays Redemption.
	dc := find(t, d, "darkcenturion-saurian")
	if dc.House != "saurian" || len(dc.Printings) != 2 ||
		dc.Printings[0].Set != "massmutation" || dc.Printings[0].House != "" ||
		dc.Printings[1].Set != "tokensofchange" || dc.Printings[1].House != "redemption" {
		t.Errorf("dark centurion: %+v", dc)
	}
	for _, c := range d.Cards {
		if c.ID == "darkcenturion-redemption" {
			t.Errorf("the Redemption printing made a second card")
		}
	}
	if bd := find(t, d, "bordantheredeemed-redemption"); bd.House != "redemption" ||
		bd.Printings[0].House != "" {
		t.Errorf("bordan: %+v", bd)
	}
	// A card that a later set prints in another house, such as Martian Civil
	// War's Elders, joins the card of its first house.
	bp := find(t, d, "blypyp-mars")
	if len(bp.Printings) != 2 || bp.Printings[1].Set != "martiancivilwar" ||
		bp.Printings[1].House != "elders" || bp.Printings[0].House != "" {
		t.Errorf("blypyp: %+v", bp)
	}
	for _, c := range d.Cards {
		if c.ID == "blypyp-elders" {
			t.Errorf("the Elders printing made a second card")
		}
	}
	// A printing that Master Vault has only as mavericks shows the art of
	// the card's newest printing that is not a maverick.
	gr := find(t, d, "thegrimreaper-geistoid")
	men := gr.Printings[len(gr.Printings)-1]
	if men.Set != "menagerie" || !men.Maverick || men.Image != "grimreminders/155.webp" {
		t.Errorf("grim reaper menagerie printing: %+v", men)
	}
	// A normal printing whose image has the grey frame of a maverick render
	// (overrides greyFrame) is treated like a maverick-only printing.
	sh := find(t, d, "shooler-dis")
	if len(sh.Printings) != 2 || !sh.Printings[1].Maverick ||
		sh.Printings[1].Image != "callofthearchons/080.webp" || sh.Printings[0].Maverick {
		t.Errorf("shooler: %+v", sh.Printings)
	}
	// Variant and FIXED cards are Special, like the later sets print them.
	if m2 := find(t, d, "masterof2-brobnar"); m2.Printings[0].Rarity != "Special" {
		t.Errorf("variant rarity: %q, want Special", m2.Printings[0].Rarity)
	}
	if n := find(t, d, "oldegad-logos"); n.HasFlag(FlagEvilTwin) || n.Power.N != 1 {
		t.Errorf("normal twin: %+v", n)
	}

	if bz := find(
		t,
		d,
		"berserker-brobnar",
	); bz.Type != "creature" ||
		!bz.HasFlag(FlagTokenCreature) ||
		bz.Printings[0].Rarity != "Token Creature" {
		t.Errorf("token creature: %+v", bz)
	}

	// Reference cards sort last, after the cards without a house.
	if last := d.Cards[len(d.Cards)-1]; last.ID != "forgeahead-none" {
		t.Errorf("last card is %s", last.ID)
	}
	if len(d.Sets) != 13 || d.Sets[0].Slug != "callofthearchons" || d.Version == "" {
		t.Errorf("sets %v version %q", d.Sets, d.Version)
	}

	got := strings.Join(kinds(problems), "\n")
	for _, want := range []string{
		"missing image aemberskies 100",
		"missing image massmutation 028",
		"anomaly without a set  A99",
		"missing gigantic image massmutation 125",
		"printing with only mavericks vaultmasters2023 099",
		"unlabeled houseless kind aemberskies 400",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("problems have no %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "creature without power worldscollide") {
		t.Errorf("the override must clear the power problem:\n%s", got)
	}

	i := slices.IndexFunc(problems, func(p Problem) bool {
		return p.Kind == ProblemMissingImage && p.Set == "aemberskies" && p.Number == "100"
	})
	if i < 0 || !slices.Equal(problems[i].Decks, []string{"deck-1"}) {
		t.Errorf("the missing image problem has no deck: %v", problems)
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

func TestUsedAliases(t *testing.T) {
	cards := []card.Card{{Printings: []card.Printing{{Artist: "Mateusz Mizak"}}}}
	got := usedAliases(map[string]string{
		"Matthew Mizak": "Mateusz Mizak",
		"Jorge Ramos":   "Jorge Ramos Márquez",
	}, cards)
	want := map[string]string{"Matthew Mizak": "Mateusz Mizak"}
	if !maps.Equal(got, want) {
		t.Errorf("usedAliases = %v, want %v", got, want)
	}
	if got := usedAliases(nil, cards); got != nil {
		t.Errorf("usedAliases(nil) = %v, want nil", got)
	}
}
