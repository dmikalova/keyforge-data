# Master Vault is the only data source

## Context

The card data had three possible sources:

- The vex provenance JSON. It has no flavor text, no anomaly flags, and no
  locales. It drops gigantic art halves, and it has null stats for token
  creatures and evil twins.
- The keyteki packs. They have keywords and twelve locales, but no flavor text,
  and they do not have all sets. They are AGPL-3.0.
- Master Vault. It has every field that the cards have, in every locale. It
  has no artist field and no card endpoint, so cards come from deck responses.

## Decision

Use Master Vault as the only source. Get cards from deck responses. Get the
locales from the cover decks with one request per language.

Do not read keyteki or Decks of KeyForge data.

## Consequences

- There is one source and no license question for the data.
- The artist field stays empty until an OCR pass reads the artist from the
  card images (see the todo).
- A card that no crawled deck contains is not in the dataset. The anomaly
  report and the overrides cover this.
