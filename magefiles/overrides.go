//go:build mage

package main

import "github.com/magefile/mage/mg"

// Overrides holds the targets for the override files (package overlay).
type Overrides mg.Namespace

// Lint checks the override files against tmp/cards.json: unknown sets,
// titles, and collector numbers, duplicates, and entries that do nothing.
func (Overrides) Lint() error {
	mg.Deps(Dataset.Distill)
	return kfdata("overrides", "lint", "-dataset", datasetFile)
}
