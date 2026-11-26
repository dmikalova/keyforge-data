# data.keyforge.cards service

Status: accepted. The card crawl runs locally until the service exists.

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
  - **Crawler.** The one paced Master Vault client, inside the same app.
    The app keeps its own schedule (see Scheduling). There is no Cloud Run
    job and no new Cloud Scheduler job. Its modes are the set scan, the cover refresh, the full deck crawl, the
    newest-deck tail, and the deck import queue.
  - **Publisher.** After a crawl adds card objects, it runs distillation,
    fills the missing images, publishes `data/cards.json`, and posts changed
    problems to the maintenance Discord channel.
  - **Problems table.** Each problem that distillation finds (a missing
    image, a blank art box, an unknown card) is a row with the decks that
    have the card, and when it was found and fixed. The API serves a report
    of the decks and their card problems.
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

## Scheduling

The user does not want a Cloud Run job or a new Cloud Scheduler job for each
schedule, because each one adds cost.

The Cloud Run module sets `cpu_idle = true` and a minimum of zero instances.
Between requests the instance gets no CPU, and it can stop. A goroutine timer
alone does not run on time.

The mklv service already sends a request to each service with the label
`warm=true` every 10 minutes, at no added cost. The app uses that request as
its clock:

- A goroutine scheduler holds the due time of each task (tail crawl, cover
  refresh, publish). The tasks and their state are in the bucket.
- Each request (the warm request or a user request) wakes the scheduler. The
  warm request waits while the due tasks run, for up to 9 minutes, so the
  instance keeps its CPU. Then it returns.
- The full deck crawl runs in these 9-minute slices, every 10 minutes. It
  continues from its saved state each time.
- If a later need requires a task to run without requests, the alternative
  is `cpu_idle = false` with one minimum instance. That costs money each
  hour, so it needs the user's approval.

## Consequences

- keyforge-data changes from `kind: library` to `kind: cloudrun` in
  `mklv.config.json`, and gets an infra stack `gcp/apps/keyforge-data`.
- The full deck crawl needs a database (ADR 0003). The card API does not.
- A crawl in the service must keep its state in the bucket, because a Cloud
  Run instance has no disk that persists.

## API access

The API is public. It needs a rate limit for each client IP, because a
query can be expensive.
