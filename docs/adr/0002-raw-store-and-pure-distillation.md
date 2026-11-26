# Raw store and pure distillation

## Context

Master Vault changes how it encodes some cards from set to set: gigantic
halves, houseless creatures, evil twins, and token creatures. vex wrote its
rules before it looked at the data, and it lost data:

- It kept one card per number and title, so it dropped gigantic art halves.
- It read power and armor only for creatures and gigantic bases.

vex also kept only the transformed output, so each rule fix needed a new crawl
of one to four hours.

A full crawl of all decks (about 3.6M) is a later goal. The storage format must
work for that size.

## Decision

Split the pipeline into a crawl and a distillation.

The crawl writes the raw store in the private bucket
`gs://mklv-keyforge-cards`:

- Decks: `raw/mv/decks/<crawl-id>/<seq>.jsonl.zst`. Each line is one deck
  response without `_linked`. A shard holds about 10,000 decks.
- Card objects: stored one time each, keyed by ID, locale, and content hash. A
  changed card object gets a new hash, so the history stays.
- Each crawl has a `manifest.json` with its shards, counts, time range, and the
  last page. A stopped crawl continues from the manifest.

The crawler has these modes:

- `feed(set)`: page the deck feed for one set. Keep a deck only if it adds a
  card object that the crawl has not seen.
- `byID(list)`: fetch a list of decks, one time per locale.
- `feed(all)`: the full crawl. It is not run yet (see ADR 0003).

After a `feed(set)` crawl, a greedy set cover finds the cover decks. Commit the
list to `sources/mv/cover/<set>.json`. A refresh fetches only the cover decks.

Distillation is a pure function from the raw store to the dataset. It does not
fetch. Write the rules for special cards after a look at the real raw data of
each set. Distillation writes unknown cases to the anomaly report and does not
fail.

Use one paced HTTP client for all Master Vault requests. It slows down after
each 429 response. Its base URL is a setting.

## Consequences

- A rule fix runs again in seconds, with no new crawl.
- Measured sizes: one deck is about 1.9 KB of JSON and about 500 B with zstd.
  4M decks are about 1–2 GB with zstd. About 87% of a page with `links=cards`
  is card objects, and the card store removes this repetition.
- A shard is the smallest unit to read. To get one deck, read its shard.
