# Full deck crawl and deck database (deferred)

Status: deferred. This ADR records the design so that the work now does not
block it.

## Context

decks.keyforge.cards needs all decks. Measured facts about the Master Vault
deck feed (2026-11-25):

- The feed has about 3.62M decks. Call of the Archons has about 1.30M.
- The maximum `page_size` is 10.
- A query returns at most about 50,000 decks. Page 5000 returns 403. The `v2`
  endpoint has the same limit.
- `ordering=date` gives the oldest decks first. `ordering=-date` gives the
  newest first.
- Any set can get new decks at any time, but the rate is slow.

Decks of KeyForge calls the feed directly from its server, one page each
minute. It sends single deck imports through a separate proxy service, which
gives it a second IP address.

## Decision

- Use one paced crawler. Do not use a proxy or many IP addresses.
- Crawl from the oldest deck to the newest. Divide each query that is larger
  than 50,000 decks into slices: set, then set × house combination, until each
  slice is below the limit. Compare the stored count of each slice with the
  feed count.
- After the full crawl, read the newest decks every 10 minutes.
- When a user asks to import a deck, put the request in a priority queue on the
  same paced client. Send the result to the user when the fetch is complete.
  Do not use a second crawler.
- Store the decks in Postgres (Supabase) in a compact form: card indexes as a
  small-integer array, the houses, and the enhancements. The raw store stays
  the source of truth.

## Consequences

- The full crawl takes about four weeks. It runs one time.
- The deck query engine compiles the query AST to SQL. The card search
  evaluates the same AST in memory (see ADR 0004).
- The Supabase plan must grow to hold about 4M decks.
