# 0008: One override format as an overlay layer

Status: accepted

## Context

The dataset comes from Master Vault (ADR 0001). Master Vault has mistakes and
gaps. Over time, the repository got many kinds of curated data, each with its
own file format:

- Card fixes and houseless kind labels in `overrides/<set>.json`.
- Curated threads in `overrides/_threads.json`.
- Deck rules in `overrides/_deckrules.json`.
- The artist of each printing in `overrides/artists/<set>.json`.
- The known artists in `overrides/artists/known.txt`.
- The artist aliases in `overrides/artists/aliases.json`.

The user wants the dataset to use the distilled Master Vault data as much as
possible. For the special cases, an overlay layer changes the distilled data
into the presentation. The dataset must show the original Master Vault data
where it can.

## Decision

1. All curated data is in `overrides/`, and every file there has one format:
   `overlay.File`. A section that a file does not use is empty.
   - `<set>.json` has the data of one set: `cards` (fixes), `artists`,
     `kinds`, and `aliases`.
   - `_threads.json` has the curated `threads` and `notes`. People edit it.
   - `_deckrules.json` has the `rules` and `notes` of the Archon Arcana
     sheet. `mage threads:rules` writes it. Do not edit it.
   - `_artists.json` has the `knownArtists` and the `artistAliases`.
   - `_labels.json` has `labels`: names for people of the data values,
     such as the houseless group labels and the ability names. They win
     over the house and type names that Master Vault gives.
2. The loader rejects an unknown field, so that a misspelled section does
   not go unseen.
3. `kfdata overrides lint` (`mage overrides:lint`) checks the files against
   the dataset. It finds unknown sets, titles, and collector numbers,
   duplicate entries, and entries that do nothing. `mage dataset:publish`
   runs it and stops on an error. A missing title in the generated deck
   rules is a warning, because it names a card of a set that the crawl
   does not have yet.
4. The overlay stays separate from the Master Vault data in the dataset:
   - Some fields are always overlay data: the printing artist, the artist
     aliases, the card notes, the curated and deck-rule related threads,
     and the name aliases of the set files. The `card.Dataset` comment
     lists them.
   - `Card.Overlay` names the other fields that an override sets in place
     of the Master Vault value, for example `power`.
5. A curated entry wins over a generated entry. A curated thread wins over
   a deck rule with the same cards or fewer. A curated note replaces the
   generated notes of the same card and kind.

## Consequences

- One loader and one lint cover all curated data.
- The artist scan writes the `artists` section of the set file and keeps
  the other sections. The OCR tool gets the known artists in a temporary
  file in the artist cache.
- A data.keyforge.cards service (ADR 0006) can apply the same overlay, or
  serve the Master Vault values with the overlay marked.
- The set files are longer, because they hold the artists. The keys are
  sorted and each entry is on its own line, so the diffs stay small.
