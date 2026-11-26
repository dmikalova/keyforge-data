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
- Run `kfdata check-art` after each image fill. Fix a blank art box with
  `kfdata fix-art` and the art of another printing.
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

## Phase 6: Move keyfordle to keyforge-data

- Replace `internal/cards` and the catalog build with keyforge-data.
- Keep the `card_name` column. Add a test: every name in the old pool resolves
  in keyforge-data.

## Phase 7: Move vex to keyforge-data

- Use the keyforge-data types and data.
- Keep the simulator data in vex.
- Delete `internal/cards/provenance` and the import code in
  `magefiles/cardlookup`.
- Do not put icons from keyforge-data into vex.

## Phase 8: Card search

- Build search.keyforge.cards. The plan is in the keyforge-card-search repo.

## Later

- Artist OCR: crop the artist line from each card image and read it with OCR.
  The user checks each new artist name (for example, the accents). Then add the
  `artist` field.
- Full deck crawl and deck database (ADR 0003).
- Images for the other locales.
- Variant groups, for the card pages.
