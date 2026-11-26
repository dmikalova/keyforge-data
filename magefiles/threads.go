//go:build mage

package main

import (
	"strings"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
)

// Threads holds the targets that edit the curated threads of related cards
// in overrides/_threads.json. Run mage dataset:distill first, because the
// targets check titles against tmp/cards.json.
type Threads mg.Namespace

// Todo prints the Variant printings that are in no thread, in candidate
// families, each with a ready threads:add command. Replace LABEL and remove
// the cards that do not belong.
func (Threads) Todo() error {
	return kfdata("threads", "todo")
}

// deckRulesURL is the Archon Arcana relationships_and_notes sheet as TSV
// (docs/sources.md).
const deckRulesURL = "https://docs.google.com/spreadsheets/d/" +
	"1EsGk3hEQ-65MB39hB0Cs6zWPg4cO99UB9Ulx8xLtYpU/export?format=tsv"

// Rules downloads the Archon Arcana deck rules to tmp/deckrules.tsv and
// writes overrides/_deckrules.json. It prints the titles that no card has.
func (Threads) Rules() error {
	if err := sh.Run("curl", "-sfL", "-o", "tmp/deckrules.tsv", deckRulesURL); err != nil {
		return err
	}
	return kfdata("threads", "rules", "-in", "tmp/deckrules.tsv")
}

// Add adds a thread, or adds titles to the thread with that label. Separate
// the titles with semicolons: mage threads:add "Brews" "Narp’s Brew;Mega Narp".
func (Threads) Add(label, titles string) error {
	return kfdata(
		append([]string{"threads", "add", "-label", label}, strings.Split(titles, ";")...)...)
}
