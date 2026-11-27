# data.keyforge.cards public surface

Status: accepted. It adds to ADR 0006 and follows project-standards ADR 0009
(the OpenAPI contract and the MCP endpoint). The Discord bot is for later.

## Context

ADR 0006 makes data.keyforge.cards the service that crawls Master Vault,
distills the dataset, and publishes it. It is the master crawler. Later it
also crawls decks.

This decision records what the service and the public bucket offer to other
programs:

- The sites (search, keyfordle, bingo, and later vex) must see a new dataset
  without a deploy.
- AI clients must be able to search cards.
- The user wants a Discord bot that answers with cards.

The user wants a new file for each dataset version. The user does not want to
manage old versions by hand.

The search site holds the query fields and the search engine in
`keyforge-card-search/internal/search`. keyforge-data holds only the query
grammar (ADR 0004).

## Decision

### Dataset files in the public bucket

| Object | Cache | Content |
| --- | --- | --- |
| `data/cards/<version>.json` | 1 year, immutable | One dataset version |
| `data/latest.json` | 60 seconds | The pointer: version, URL of the version file, generated time, card count, set count |
| `data/feed.xml` | 60 seconds | An Atom feed with one entry for each version |

- The publisher writes the version file first, then the feed, then the
  pointer. A client that reads the pointer always finds the version file.
- The publisher deletes version files that are older than 90 days. It never
  deletes the current version. A bucket lifecycle rule cannot make that
  exception: when no new version comes for 90 days, the rule deletes the
  file that the pointer names.
- The files stay in the bucket, not in the service. The sites keep working
  when the service is down.

### Change feed

- A program polls `data/latest.json` with `If-None-Match`. When the version
  changes, it downloads the version file. This is the simplest method for
  keyfordle, bingo, and vex, because they already read JSON and the pointer
  is less than 1 KB. The search site works this way now, with the full file.
- `data/feed.xml` is for people and for feed readers. Each entry gives the
  cards that the version adds, changes, and removes, and links to them on
  search.keyforge.cards.
- Do not make a JSON Feed in addition. The pointer and the Atom feed cover
  programs and people.

### Search engine

- The search fields and the search engine move from
  `keyforge-card-search/internal/search` to a keyforge-data package. The API,
  the MCP tools, and the bot use the same fields as the site.
- The deck search uses the same engine later, with the deck fields.
- search.keyforge.cards becomes only a frontend. It imports the engine for
  the pages, and it calls no search code of its own.

### API endpoints

The endpoints are huma operations (project-standards ADR 0009), so the
service generates `/openapi.json`, `/docs`, and `/llms.txt`.

| Endpoint | Content |
| --- | --- |
| `GET /api/cards?q=` | The cards that match a query, in the shape of the search site's JSON export |
| `GET /api/cards/{id}` | One card, with its printings, related cards, and mentions |
| `GET /api/sets` | The sets, in release order |
| `GET /api/syntax` | The query syntax as text, for AI clients |
| `GET /api/problems` | The dataset problems with their decks (ADR 0006) |
| `GET /api/decks/{id}`, `GET /api/decks?q=` | Later, after the deck crawl |
| `POST /api/decks/{id}/import` | Later. It adds a deck to the crawl queue. |

- The search site keeps its own `/api/cards` until the data service is live.
  Then it redirects to the data service.

### MCP tools

The service serves `/mcp`. Each tool is an API operation with the `x-mcp`
mark (project-standards ADR 0009):

| Tool (`operationId`) | Input | Output |
| --- | --- | --- |
| `search_cards` | A query in the shared grammar, a limit | The matching cards, in short form |
| `get_card` | A card ID or a card name | The full card |
| `list_sets` | None | The sets, with their codes and names |
| `query_syntax` | None | The syntax text, so that the client can write queries |

Later: `get_deck` and `search_decks`.

The operation descriptions are written for an AI reader, with an example
query each.

### Discord bot (later)

- The user accepts Cloud Run apps that run all the time. The bot can then
  keep a Discord gateway connection open in the data service.
- With the gateway connection and the message content intent, the bot reads
  `[[Card Name]]` in messages and answers with those cards. It finds the
  names with the mention matcher of distillation.
- The bot also has the slash command `/card name:<name>`, with Discord
  autocomplete.
- Each answer is an embed: the card name links to the card page on
  search.keyforge.cards, the image comes from the public bucket, and the
  description is the card text.
- The bot token is in sops in the infrastructure repo.

## Consequences

- The search site must have card pages with preview tags for the bot links
  to show well in Discord.
- Moving the search engine is a change in two repos. Do it before the API
  endpoints, so that the site and the API do not have two copies.
- The service gets the `api: true` setting and the `httpapi` module of
  project-standards ADR 0009.
- The bot needs an instance that always runs, with `cpu_idle = false` and a
  minimum of one instance (ADR 0006, Scheduling). That costs money each hour.
- The Discord application and its secrets are new. The infrastructure repo
  holds the secrets.
