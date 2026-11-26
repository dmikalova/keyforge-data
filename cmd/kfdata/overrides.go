package main

import (
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/dmikalova/keyforge-data/overlay"
)

// overridesCmd runs "kfdata overrides lint": it checks the override files
// against a dataset and fails on an error. A warning names a card of a set
// that the crawl does not have yet.
func overridesCmd(args []string, stdout io.Writer) error {
	if len(args) == 0 || args[0] != "lint" {
		return errors.New(
			"usage: kfdata overrides lint [-dataset tmp/cards.json] [-overrides overrides]",
		)
	}
	fs := flag.NewFlagSet("overrides lint", flag.ContinueOnError)
	dataset := fs.String("dataset", "tmp/cards.json", "distilled dataset")
	dir := fs.String("overrides", "overrides", "overrides directory")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	files, err := overlay.LoadAll(*dir)
	if err != nil {
		return err
	}
	cards, err := readDataset(*dataset)
	if err != nil {
		return err
	}
	errs, warnings := 0, 0
	for _, f := range overlay.Lint(files, cards) {
		_, _ = fmt.Fprintln(stdout, f)
		if f.Error {
			errs++
		} else {
			warnings++
		}
	}
	_, _ = fmt.Fprintf(stdout, "%d errors, %d warnings\n", errs, warnings)
	if errs > 0 {
		return fmt.Errorf("the override files have %d errors", errs)
	}
	return nil
}
