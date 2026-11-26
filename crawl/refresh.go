package crawl

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/dmikalova/keyforge-data/mv"
	"github.com/dmikalova/keyforge-data/raw"
)

// ModeRefresh is the manifest mode of a Refresh crawl.
const ModeRefresh = "refresh"

// RefreshState is the resume state of a Refresh crawl: the number of decks
// done, by language.
type RefreshState map[string]int

// Refresh fetches each deck in each language and stores the decks and their
// linked objects. A deck that Master Vault no longer has is logged and
// skipped. It continues a stopped crawl from the crawl's state.
func Refresh(
	ctx context.Context,
	log *slog.Logger,
	client *mv.Client,
	c *raw.Crawl,
	deckIDs, locales []string,
) error {
	state := RefreshState{}
	if err := c.State(&state); err != nil {
		return err
	}
	for _, locale := range locales {
		if _, ok := mv.Locales[locale]; !ok {
			return fmt.Errorf("unknown locale %q", locale)
		}
		for i := state[locale]; i < len(deckIDs); i++ {
			id := deckIDs[i]
			p, err := client.Deck(ctx, id, locale)
			switch {
			case errors.Is(err, mv.ErrNotFound):
				log.Warn("deck not found", "deck", id, "locale", locale)
			case err != nil:
				return fmt.Errorf("deck %s (%s): %w", id, locale, err)
			default:
				if err := c.WriteDecks(locale, "deck", p.Decks); err != nil {
					return err
				}
				if _, err := c.PutLinked(locale, p.Linked); err != nil {
					return err
				}
			}
			state[locale] = i + 1
			if err := c.Checkpoint(state); err != nil {
				return err
			}
			log.Info("deck", "locale", locale, "n", i+1, "of", len(deckIDs))
		}
	}
	return c.Close(state, true)
}
