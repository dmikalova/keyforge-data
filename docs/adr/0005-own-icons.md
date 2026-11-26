# Own icons, traced from official references

## Context

The sites need house, set, rarity, card type, and stat icons. keyteki and
Decks of KeyForge have icons, but many of them are their own drawings: the
keyteki set icons, the Decks of KeyForge houses and sets, and the
bingo-generator copies of them. The keyteki house icons are official art.

## Decision

- Houses: use the official keyteki house PNGs. Use the Keyraken icon from the
  `pack/mg` branch of dmikalova/keyteki. Get Elders and Ironyx Rebels from the
  KeyForge news pages, or from Archon Arcana.
- Sets: trace our own SVGs. Use Archon Arcana images and the Master Rulebook
  images (`keyforge-scripts/rules-to-markdown`) as references.
- Rarity: draw our own SVGs from the deck list cards: circle (common), diamond
  (uncommon), star (rare), diamond with a center mark (special).
- Card types: draw our own SVGs from the deck list cards: sword (artifact),
  eared mask (creature), arrow that curves down (action), up arrow (upgrade),
  card with an oval (token creature).
- Use one color with `currentColor`, so the theme can set the color.
- Keep the icons in `assets/icons/{house,set,rarity,type,stat}/`. Record the
  source of each file in `assets/icons/SOURCES.md`. Upload them to
  `images/icons/` in the public bucket.
- Do not copy keyteki or Decks of KeyForge drawings, except the official
  keyteki house icons.

## Consequences

- If a set has no usable reference, the todo records it, and the user gets
  the reference.
