# data.keyforge.cards public surface

Status: proposed. It adds to ADR 0006 and follows project-standards ADR 0009
(the OpenAPI contract and the MCP endpoint).

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
| `data/cards.json` | 60 seconds | The current dataset, until all sites read `latest.json`. Then delete it. |

- The publisher writes the version file first, then the feed, then the
  pointer. A client that reads the pointer always finds the version file.
- A bucket lifecycle rule deletes version files after 90 days. The publisher
  does not delete files.
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

### API endpoints

| Endpoint | Content |
| --- | --- |
| `GET /api/cards?q=` | The cards that match a query, in the shape of the search site's JSON export |
| `GET /api/cards/{id}` | One card, with its printings, related cards, and mentions |
| `GET /api/sets` | The sets, in release order |
| `GET /api/syntax` | The query syntax as text, for AI clients |
| `GET /api/problems` | The dataset problems with their decks (ADR 0006) |
| `GET /api/decks/{id}`, `GET /api/decks?q=` | Later, after the deck crawl |
| `POST /api/decks/{id}/import` | Later. It adds a deck to the crawl queue. |

- The search fields and the search engine move from
  `keyforge-card-search/internal/search` to a keyforge-data package. The site
  imports them. The API, the MCP tools, and the bot then use the same fields
  as the site.
- The search site keeps its own `/api/cards` until the data service is live.
  Then it redirects to the data service.

### MCP tools

The service serves `/mcp` (project-standards ADR 0009). The tools:

| Tool | Input | Output |
| --- | --- | --- |
| `search_cards` | A query in the shared grammar, a limit | The matching cards, in short form |
| `get_card` | A card ID or a card name | The full card |
| `list_sets` | None | The sets, with their codes and names |
| `query_syntax` | None | The syntax text, so that the client can write queries |

Later: `get_deck` and `search_decks`.

### Discord bot

- The bot is a Discord application with an HTTP interactions endpoint at
  `POST /discord/interactions` in the data service. It needs no gateway
  connection, so it needs no always-on instance.
- The bot has the slash command `/card name:<name>`. The name option uses
  Discord autocomplete, which also comes over the interactions endpoint.
- The bot also has a message command "Find cards". A user selects it on a
  message. The bot finds the card names in the message with the mention
  matcher of distillation, and answers with those cards.
- Do not read `[[Card Name]]` in every message. That needs a gateway
  connection with the message content intent, and an instance that always
  runs. That costs money each hour.
- A cold start can take more than 3 seconds, and Discord waits 3 seconds. The
  endpoint sends a deferred answer first, and then sends the cards as a
  follow-up message.
- Each answer is an embed: the card name links to the card page on
  search.keyforge.cards, the image comes from the public bucket, and the
  description is the card text.
- The service checks the Ed25519 signature of each request with the
  application public key. The bot token is used only to register the
  commands, from a mage target. It is in sops in the infrastructure repo.

## Consequences

- The search site must have card pages with preview tags for the bot links
  to show well in Discord.
- Moving the search engine is a change in two repos. Do it before the API
  endpoints, so that the site and the API do not have two copies.
- The service gets the `api: true` setting and the `httpapi` module of
  project-standards ADR 0009.
- The Discord application and its secrets are new. The infrastructure repo
  holds the secrets.
