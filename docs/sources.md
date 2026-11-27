# Data sources

Master Vault is the only source of the dataset (ADR 0001). This file lists
the other sources that we use to check the data, or that can give data that
Master Vault does not have.

The icon sources are in [../assets/icons/SOURCES.md](../assets/icons/SOURCES.md).

## archonarcanabots

The bot repository of the Archon Arcana wiki:
<https://github.com/blinkingline/archonarcanabots>. We read it at commit
`c47450df3f2ea6dbf731aa828cb3fd9c9e612356` (2026-11-17).

The repository has no license file. Use its data to check our data and as
hints. Do not copy its files into this repository.

### Artists

| Source | Content | Use |
| --- | --- | --- |
| `skyjedi/illustrators.csv` | The illustrator of each card title (2641 rows, 259 names) | Hints for the artist scan, and a check of the artist files |
| `data/artists_479.csv`, `data/artists_496.csv` | The artist of each card number in Mass Mutation and Dark Tidings | The same, for each printing |
| The `kfa001`, `kfa002`, `woe`, `gr`, and `as` Google Sheets | The `Artist` column, with the set name and the card number | The same, for Adventures, Winds of Exchange, Grim Reminders, and Æmber Skies |

The sheet links are in `data/spreadsheets.json`. The sheets are public. Add
`/export?format=tsv` to a sheet link to download it.

On 2026-11-26 we compared the artist scan with `illustrators.csv`:

- 2423 of 2486 printings agreed.
- 46 printings had a different spelling of one artist. The aliases in
  [../overrides/_artists.json](../overrides/_artists.json) now
  join the spellings. The artist files use only the canonical names. The
  dataset `artistAliases` map gives the other spellings to search.
- 18 printings disagreed. In all 18, the card image showed that the scan was
  correct. The CSV gives a studio for an artist (Art Tavern for Anton
  Zemskov), and it swaps the artists of related cards (Teliga and Hunting
  Witch).

The card image is the authority. When a source and the card image disagree,
use the card image.

`mage artists:scan` downloads `illustrators.csv` to `tmp/hints/` and uses it
in two steps:

1. The OCR matches the known artists and the artists of the CSV. Without the
   CSV artists, the OCR matched a new artist to a known artist with a similar
   line: "Dib" became Rodrigo Camilo, not Djib.
2. `kfdata artists-hints` settles the printings where the OCR is not sure.
   It takes the CSV artist when:
   - the OCR and the CSV agree;
   - the OCR reads nothing useful, and the card's other printings have the
     CSV artist;
   - the OCR reads nothing useful, and the card has no other printing.

The card images are 300 by 420 pixels, also the originals. The artist line is
too small to read by eye. On 2026-11-27 the scan of the sets from Vault
Masters 2023 to Prophetic Visions settled 3125 printings: 2635 by OCR alone,
283 by OCR and CSV, 76 by the CSV and the other printings, 126 by the CSV for
new cards, and 5 are in the review. 12 OCR matches disagreed with the CSV, and
their readings showed the CSV artist. The artist files have the CSV artist.

### Deck rules

The `relationships_and_notes` Google Sheet has 285 rules about the cards in a
deck. Each row has a type, the cards, and a note. The types are:

| Type | Rows | Example |
| --- | --- | --- |
| `always` | 63 | Faygin always appears in a deck with at least 2 copies of Urchin. |
| `sometimes` | 23 | Urchin sometimes appears in a deck with Faygin. |
| `copies` | 7 | A deck always has at least 2 copies of Angry Mob. |
| `fixed` | 37 | The Four Horsemen always appear together. |
| `others` | 4 | A Shard always appears with at least one other Shard. |
| `variant` | 42 | Only one "Master of" variant can appear in a deck. |
| `house` | 13 | Dark Æmber Vault can appear in any house in Mass Mutation. |
| `houseswap` | 85 | Armageddon Cloak appears in house Redemption in Tokens of Change. |
| `anomaly` | 6 | Orb of Wonder first appeared as an Anomaly in Worlds Collide. |
| `special` | 3 | Exchange Officer always appears as a Maverick. |

`mage threads:rules` downloads the sheet to `tmp/deckrules.tsv` and writes
the `fixed`, `always`, `sometimes`, `others`, and `variant` rules to
`overrides/_deckrules.json`. Distillation adds each rule to the related
threads of its cards:

- A rule with page cards links each page card to the other cards, for
  example "Faygin: Always with Urchin". The label does not give the number
  of copies.
- A rule without page cards is a group. Each card of the group gets the
  other cards.
- A curated thread (`overrides/_threads.json`) wins. A rule adds nothing to
  a card if a thread of the card already has all the rule's cards.
- Use a curated thread to give a family a better name or to add a card
  that the sheet does not list. The curated threads name the variant
  families ("Key Imps", "Banes") and the Mass Mutation mutants ("Dis
  mutants" for the Dæmo- cards, "Fiend mutants" for the -Fiend cards).
- The import drops a variant rule of a card and its Mega creature, because
  distillation links them as "Mega creature".

The command prints the titles that no card has. Most of them are cards from
sets that the crawl does not have yet. Run `mage threads:rules` again after a
crawl adds a set.

The `copies`, `house`, `houseswap`, `anomaly`, and `special` rules are about
one card. The command writes them as notes to `overrides/_deckrules.json`.
Distillation adds each note to the `notes` of its card, for example "Always 3+
copies" on Plague Rat. A title with a house in parentheses, such as "Armageddon
Cloak (Redemption)", is the card of that house. If the dataset does not have
that card yet, the note is dropped. A note in `overrides/_threads.json`
replaces the imported notes of the same card and kind, for example to fix a
long note or a misspelling in the sheet.

### Other data

| Source | Content | Possible use |
| --- | --- | --- |
| `data/Archon Arcana Translations - terms.tsv` | The set names and the game terms in 11 languages | Search and display in other languages |
| `data/traits/<locale>.json` | The traits in 11 languages | The same |
| `data/my_card_db_locales.json` | The card text from the wiki in other languages | A check of the future localizations |
| `data/my_card_db.json` | The wiki card data by title and expansion, with the keywords and the enhance counts | A check of the derived keywords and enhance counts |
| `skyjedi/<expansion>.json` | Master Vault card objects for 17 expansions (341 to 907) | A list of the sets that our crawl does not have yet |
| `data/reprint_update.csv`, `data/changed_cards_*.json` | The text changes between the printings of a card | The text of each printing |
| `data/VM2024 Cards - List.csv`, `data/list_cards_928.csv`, `data/list_cards_964.csv`, `data/manual_cards_964.json` | The card lists of later sets | A check of the set coverage after a crawl |
