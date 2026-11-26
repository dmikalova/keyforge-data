//go:build mage

package main

import (
	"strings"

	"github.com/magefile/mage/mg"
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

// Add adds a thread, or adds titles to the thread with that label. Separate
// the titles with semicolons: mage threads:add "Brews" "Narp’s Brew;Mega Narp".
func (Threads) Add(label, titles string) error {
	return kfdata(
		append([]string{"threads", "add", "-label", label}, strings.Split(titles, ";")...)...)
}
