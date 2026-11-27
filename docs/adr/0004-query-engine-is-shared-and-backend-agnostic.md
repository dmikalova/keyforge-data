# The query engine is shared and backend-agnostic

## Context

gallery.keyforge.cards and decks.keyforge.cards use one query language. The
deck language adds more fields. Card ratings come later as more fields. Cards
fit in memory (about 7,500 rows). Decks are in Postgres (about 4M rows).

## Decision

Put the query engine in `keyforge-data/query`:

- The lexer, the parser, and the AST.
- A field registry. Each site registers its fields with a name, one alias, a
  value type (text, name, enum, number), and an accessor.
- An in-memory evaluator for cards.
- Later: an AST-to-SQL compiler for decks.

The AST does not know about the backend. Move the engine to its own module
only if its release cycle becomes different from the data.

## Consequences

- A new field (a deck field or a rating) is one registry entry.
- The parser tests and fuzz tests apply to both sites.
