# keyforge-data

The shared KeyForge card data and query engine. This file is a glossary only.
It has no implementation detail.

## Card model

**Card**:
One playable card identity: a name and a house. Two cards with the same name in
different houses are two cards. An evil twin is a separate card from the card
with the same name.
_Avoid_: card printing, oracle card.

**Card ID**:
The stable key of a card: the slug of the folded name, plus the house, plus a
suffix for an evil twin (`ancient-bear-untamed`,
`armadrone-logos-evil-twin`).

**Printing**:
One appearance of a card in one set, with that set's collector number, rarity,
image, and anomaly status. A reprint is a printing of a card in a later set.

**Localization**:
The text of a printing in one language: the title, the card text, the flavor
text, and the image. The first dataset has English only. The model allows all
twelve Master Vault languages.

**Default printing**:
The printing that a result tile shows. Use the newest set. Prefer a printing
that is not an anomaly. Use only printings that the query's set filter allows.

**Folded name**:
The name in lowercase ASCII, without diacritics or punctuation. `Æ` becomes
`ae`. Name search matches the folded name and the aliases.

**Alias**:
An extra folded name that name search also matches. Generic aliases come from a
digit map (0→o, 1→i, 3→e, 4→a, 5→s, 7→t). Curated aliases come from the
overrides (`ANT1-10NY` → `anthony`).

**Set**:
A KeyForge expansion, with a Master Vault expansion ID, a code (`GR`), a slug
(`grimreminders`), and a name.
_Avoid_: expansion (except for the Master Vault ID).

**House**:
One of the real KeyForge houses. A card that is not in a real house has the
house "none".

**Reference card**:
A card that is not part of a deck's 36 cards: an archon power, a prophecy, or
the tide. Master Vault gives these cards a pseudo-house (`archonpower`,
`prophecy`, `thetide`). The flag is `is:reference`.

**Houseless card**:
A card that is in a deck but not in one of the deck's three houses. All
houseless cards have the flag `is:houseless`. There are three groups:

- The houseless kinds: skybeasts (Æmber Skies), revenants (Grim Reminders),
  mercenaries (a later set). The overrides give each kind a label, and the
  label becomes a flag (`is:skybeast`).
- The anomalies that have no printing in a playable set.
- The cards that Master Vault prints in the house of the deck (Dark Æmber
  Vault, It's Coming...).

**Maverick** and **legacy**:
A maverick is a card in a house that is not its own. A legacy is a card from
an older set in a newer deck. Card search ignores both: they are copies of a
printing that the dataset already has. One exception: a printing that
Master Vault sends only as mavericks (Exchange Officer in Vault Masters
1) is kept in the house of the card's other printings.

**Anomaly**:
A card with the anomaly frame and its own rarity symbol. Master Vault keeps
the Worlds Collide anomalies in a separate expansion (453). Players never see
that expansion: the anomalies come in Worlds Collide decks, so they are
Worlds Collide printings with the numbers A01 to A10. If an anomaly has a
printing in a playable set, that printing replaces the expansion 453 card.

**Enhanced copy**:
A card object that Master Vault makes for a card with Enhance icons in one
deck. A deck needs the icons. The card gallery ignores enhanced copies: they
are the same card as the base card.

**Gigantic creature**:
One card with two halves: the base half and the art half. The base half has the
power, the armor, and the text. The image is a composite: the two halves side
by side, rotated, and scaled to one card tile.

**Evil twin**:
A separate card with the same name and house as a normal card, with its own
number, text, stats, and image. The flag is `is:eviltwin`.

**Token creature**:
A creature that a deck brings as its token. Its type is creature. The flag is
`is:tokencreature`.

**Power**:
A number, or X. A card with X power matches `power:x`. Numeric comparisons do
not match a card with X power.

**Bonus aember**:
The printed aember icons on a card (`aember`). Other bonus icons exist only on
enhanced cards in decks, so they are deck data, not card data.

**Enhance**:
The number of bonus icons that a card gives to other cards ("Enhance …"), in
total and per icon type. Distillation derives it from the card text.

**Keyword**:
A rules keyword in the card text (Elusive, Assault 2, Deploy …). Distillation
derives it from the card text.

**Artist**:
The artist name that a printing prints at the bottom left. Master Vault does
not send it. An OCR scan reads it from the card image and matches it to the
known artists. The user answers the matches that are not sure.

**Known artists**:
The list of artist names that the scan can match. The user adds a name in the
review.

**Artist alias**:
A different spelling of a known artist: a native and an anglicized name, a
short form, a studio's other name, or a misprint. The dataset uses the
canonical name for all spellings. The dataset `artistAliases` map gives the
other spellings, so that search finds them.

**Variant**:
The printed rarity that Master Vault gives to house variants. A variant group
links the members of one variant family.

**Note**:
A short fact about how a card appears in decks, for example "Always 3+
copies" or "Always a Maverick". The notes come from the deck rules of the
Archon Arcana sheet (docs/sources.md). They are overlay data, not Master
Vault data.

## Data pipeline

**Master Vault (MV)**:
The official KeyForge deck API. It is the only data source.

**Crawl**:
One run of the crawler. A crawl reads Master Vault pages and writes the raw
store. A crawl has a manifest, so it can continue after a stop.

**Raw store**:
The Master Vault responses as received, in the private bucket. Decks are in
zstd JSONL shards. Card objects are stored one time each, by ID, locale, and
content hash.
_Avoid_: cache.

**Cover decks**:
The smallest list of decks that together contain every card object of a set.
The list is committed. A refresh fetches only the cover decks.

**Distillation**:
The pure function that makes the dataset from the raw store. It does not
fetch. It runs again in seconds after a rule change.
_Avoid_: import (for this step).

**Overrides**:
The files in overrides/. They are the overlay: the curated data that
distillation puts over the Master Vault data (ADR 0008). All files have one
format (package overlay). A set file has the card fixes, the artists, the
houseless kind labels, and the name aliases of one set. The shared files
start with "_": the curated threads and notes, the deck rules, and the
artists. `mage overrides:lint` checks them.

**Overlay**:
The data that the overrides add to the Master Vault data, or put in its
place. The dataset keeps the Master Vault data where it can. `Card.Overlay`
names the card fields that an override set.

**Anomaly report**:
The list of cards that the distillation rules do not recognize. Distillation
writes it to `docs/todo.md` and does not fail.
