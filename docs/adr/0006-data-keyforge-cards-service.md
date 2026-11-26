# data.keyforge.cards service

Status: proposed. The card crawl runs locally until the service exists.

## Context

The dataset now goes to the public bucket from a local `mage dataset:publish`
run (ADR 0002). The search site reads it from the bucket and reloads it when
it changes. That works, but it depends on one Mac that is awake.

The full deck crawl (ADR 0003) must run for about four weeks without a stop,
and then read the newest decks every 10 minutes. It needs a server.

The other sites (search, decks, keyfordle, bingo) need cards and, later,
decks. Each site must not have its own copy of the crawler or the
distillation rules.

## Decision

- keyforge-data becomes a Cloud Run service at data.keyforge.cards, in
  addition to a Go library. The library packages stay importable.
- The service has three parts:
  - **API.** Read-only HTTP endpoints for cards (the dataset, one card, a
    query with the shared grammar) and, after the deck crawl, decks (one
    deck, a deck query). A deck import request goes into the priority queue
    of the crawler (ADR 0003).
  - **Crawler.** The one paced Master Vault client. It runs as a Cloud Run
    job or a worker with one instance, so there is only one client. Its
    modes are the set scan, the cover refresh, the full deck crawl, the
    newest-deck tail, and the deck import queue.
  - **Publisher.** After a crawl adds card objects, it runs distillation,
    fills the missing images, publishes `data/cards.json`, and posts changed
    problems to the maintenance Discord channel.
- The raw store moves to the private bucket as the primary copy. The local
  `./raw` becomes a cache.
- The sites keep reading `data/cards.json` from the public bucket. The API is
  for queries and decks, not for the card list. The bucket is cheaper and
  does not go down with the service.
- Turn on the stages in this order:
  1. The local card crawl continues until all sets are done (now).
  2. The service with the publisher and the card API, from the raw store in
     the bucket.
  3. The crawler in the service, in card mode (set scan, cover refresh).
  4. The full deck crawl and the deck database, when the user starts it.

## Consequences

- keyforge-data changes from `kind: library` to `kind: cloudrun` in
  `mklv.config.json`, and gets an infra stack `gcp/apps/keyforge-data`.
- The full deck crawl needs a database (ADR 0003). The card API does not.
- A crawl in the service must keep its state in the bucket, because a Cloud
  Run instance has no disk that persists.

## Open questions

- Is the crawler a Cloud Run job on a schedule, or one always-on instance?
  A job that runs every 10 minutes fits the tail. The four-week full crawl
  fits an always-on instance better.
- Is the API public, or only for the keyforge.cards sites?
