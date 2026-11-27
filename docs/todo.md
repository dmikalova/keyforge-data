# Todo

The work plan for keyforge-data and the projects that use it. Do the phases in
order. The vocabulary is in [../CONTEXT.md](../CONTEXT.md).

## Phase 0: Repo setup

- Add `go.mod` (`github.com/dmikalova/keyforge-data`), `mklv.config.json`,
  `lefthook.jsonc`, `paseo.json`, `AGENTS.md`, and the magefiles with the
  project-standards `ci` import. Make `mage ci:check` print ALL GREEN.

## Phase 1: Icons (ADR 0005)

- Copy the official keyteki house PNGs (15 houses).
- Copy the Keyraken icon from the dmikalova/keyteki `pack/mg` branch.
- Find Elders and Ironyx Rebels on the KeyForge news pages. Use Archon Arcana
  if they are not there.
- Get the set references from Archon Arcana and the Master Rulebook PDF. Make
  a list of the sets that have no usable reference, and give it to the user.
- Trace the set SVGs.
- Draw the rarity SVGs (common, uncommon, rare, special) and the card type
  SVGs (artifact, creature, action, upgrade, token creature).
- Write `assets/icons/SOURCES.md`.
- Add a mage target that converts PNGs to webp and uploads the icons to
  `gs://mklv-keyforge-cards-public/images/icons/` with an immutable cache
  header.

## Phase 2: Model and crawler (ADR 0001, 0002)

- Write the types: `Card`, `Printing`, `Localization`, `Set`, `House`. Power
  is a number or X.
- Move the paced client and the 429 backoff from vex
  (`magefiles/cardlookup/importprovenance.go`). Make the base URL a setting.
- Write the raw store: the deck shard writer, the card object store, and the
  crawl manifest. Add a local directory store for tests.
- Write the crawl modes `feed(set)` and `byID(list)`. Keep `feed(all)` as a
  stub.
- Write the greedy set cover. Commit `sources/mv/cover/<set>.json`.

## Phase 3: Crawl

- Wait until the user's image pulls are complete.
- Run `feed(set)` for all sets.
- Run `byID(cover)` for all twelve locales.

## Phase 4: Distillation

- Done in the first pass (distill package): enhanced copies merge into
  the base card, gigantic halves join, the expansion 453 anomalies become
  Worlds Collide printings, and houseless cards get `is:houseless`.
  `mage dataset:publish` uploads the dataset and posts changed problems to
  the maintenance Discord channel.
- Check the Winds of Exchange token creatures when the crawl reaches that
  set. The user expects about 28. Find where Master Vault puts a deck's
  token creature.
- Check the anomaly rarity icon (`rarity/anomaly.svg`) against a printed
  anomaly.
- When the crawl has a set with Redemption cards, give a card a second
  house. A card that Master Vault prints in its own house and in Redemption
  has two houses, for example Shadows and Redemption. Redemption is always
  the second house when the card has another house. Then replace the
  "Appears in house Redemption in Tokens of Change" notes with the second
  house. Do not do this before the crawl has the Redemption cards.
- Run `kfdata check-art` after each image fill. Fix a blank art box with
  `kfdata fix-art` and the art of another printing.
- After the card crawl: run `mage artists:scan` for the last sets, then
  review the artists with the user (`mage artists:review`). Open items on
  2026-11-27: Deusillus (Caio Monteiro or Leo Aveiro), Bonerot Venom (Ivan
  Frolov or Ivan Tao), Shae "Cloudkicker" (Jeferson Cordeiro or Oscar
  Römer), Wasteful Regret (Fábio Mota or Fábio Perez), the spellings
  Jean-Baptiste Reynaud and Alisson Ricardo, and Dodger's 10 ("Allison
  Rick / Felipe Gallo" as one artist or two).
- Move the scheduled crawl and publish to the data.keyforge.cards service
  (ADR 0006), so they do not depend on this Mac.
- When Grim Reminders is crawled, check that The Grim Reaper shows its GR
  printing and not the Worlds Collide anomaly.
- Look at the raw data of each set before you write the rules. Make a report
  of what Master Vault does with gigantic halves, houseless creatures, evil
  twins, token creatures, reference cards, and X power in each set. Give it to
  the user.
- Write the distillation rules from the report.
  - Gigantic: one card with a base half and an art half.
  - Houseless: detect cards that are not in one of the deck's houses. Use the
    kind labels from the overrides. Write unlabeled kinds to the anomaly
    report.
  - Evil twins: separate cards. Use the type that Master Vault gives.
  - Token creatures: type creature, flag `is:tokencreature`.
  - Reference cards: house none, flag `is:reference`.
  - Read power and armor for every creature-like type. Keep X power as X.
- Derive keywords and enhance counts from the card text.
- Make the generic aliases. Make the first list of curated aliases from names
  with digits, and give it to the user (`ANT1-10NY` → `anthony`).
- Write the overrides. Include the known Master Vault mistakes (for example,
  the Geistoid token in Tokens of Change with no armor).
- Add a validation test: every creature-like card has power (a number or X).
- Write the dataset and embed it. Tag a release.

## Phase 5: Images

- Download the images for all printings that are not in the bucket. Only 6 of
  22 sets are there now.
- Make the gigantic composite images: the two halves side by side, rotated,
  and scaled to one card tile.
- Upload with the existing path rule and the immutable cache header.

## Phase 6: Move keyfordle and the amasser to keyforge-data

- Move the amasser to keyforge-data and the data.keyforge.cards files.
- Replace `internal/cards` and the catalog build with keyforge-data.
- Keep the `card_name` column. Add a test: every name in the old pool resolves
  in keyforge-data.

## Phase 7: Move vex to keyforge-data

- On hold: the user wants to wait with vex.
- Use the keyforge-data types and data.
- Keep the simulator data in vex.
- Delete `internal/cards/provenance` and the import code in
  `magefiles/cardlookup`.
- Do not put icons from keyforge-data into vex.

## Phase 8: Card search

- Build gallery.keyforge.cards (search.keyforge.cards until 2026-11-27). The
  plan is in the gallery.keyforge.cards repo.

## Phase 9: data.keyforge.cards (ADR 0006, ADR 0007)

- Make data.keyforge.cards the master crawler. All Master Vault calls go
  through its one paced client. The Mac does not crawl.
- After the card crawl is complete, change the crawler to a deck crawler that
  runs on the data.keyforge.cards container (ADR 0003).
- Find the Master Vault limit before the deck crawl. On 2026-11-26 a 10 s
  pace read about 240 feed pages an hour for 8 hours. Then Master Vault
  throttled the crawl after about 2,000 pages and blocked it for about
  12 hours. At 260 pages an hour and 10 decks a page, 3.5M decks take about
  56 days. If the limit is about 2,000 pages a day, they take years. Test
  these options:
  - A larger `page_size` (25 or 50). This changes the time the most.
  - Requests with the user's Master Vault login cookie. Find out whether a
    logged-in client gets a higher limit.
  - Two crawlers. Read the Master Vault terms first. Two crawlers that get
    around a limit can get the IP or the account blocked.
- Done: `mage dataset:publish` writes the versioned dataset files
  (`data/cards/<version>.json`), the pointer (`data/latest.json`), and the
  Atom feed (`data/feed.xml`).
- Document the endpoints with the standard that all repos use. Add an MCP
  server so that AI clients can search cards (ADR 0007).
- Discord card bot: a `/card` slash command and `[[Card Name]]` in messages
  (ADR 0007).

## Later

- Deck rules: `mage threads:rules` imports the rules that link cards and the
  notes about one card (docs/sources.md). Run it again after each crawl that
  adds a set.
- Artist hints: use the Archon Arcana artist data (docs/sources.md) as hints
  in `artists:scan`, so that a new set needs fewer review answers.
- Artist scan: run it for each set that the crawl adds, so that the new
  printings get artists.
- "Seen in N decks" on the card view, and the cards that a card is often
  with. This needs the deck crawl.
- A full breakdown of each card and its mechanics. This needs vex first.
- Localization: translate the whole search page, with a language button
  ("en") next to the download button. Use the official translated terms.
  This needs the crawl of the other languages.
- Images for the other locales.
- Print proxies of the results. This is only an idea. It is not planned.
