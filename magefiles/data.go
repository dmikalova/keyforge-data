//go:build mage

// Package main holds the keyforge-data mage targets. Run `mage -l` to list
// them, and `mage ci:fix && mage ci:check` before calling work done.
package main

import (
	"strings"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
)

// rawBucket is the raw store in the private bucket (ADR 0002).
const rawBucket = "gs://mklv-keyforge-cards/raw/mv"

// Raw holds the targets that read and write the raw store.
type Raw mg.Namespace

// CrawlSets scans the deck feed of each set into ./raw.
//
// Pass set slugs separated by commas, or "all". A stopped crawl continues
// when you run the same target on the same UTC day.
func (Raw) CrawlSets(slugs string) error {
	return kfdata(append([]string{"crawl-sets"}, split(slugs)...)...)
}

// Cover writes sources/mv/cover/<set>.json from a crawl-sets crawl.
//
// Pass the crawl ID (for example sets-20261126) and the set slugs separated
// by commas, or "all".
func (Raw) Cover(crawl, slugs string) error {
	return kfdata(append([]string{"cover", "-crawl", crawl}, split(slugs)...)...)
}

// Refresh fetches the cover decks of each set in all languages into ./raw.
//
// Pass set slugs separated by commas, or "all".
func (Raw) Refresh(slugs string) error {
	return kfdata(append([]string{"refresh"}, split(slugs)...)...)
}

// Status prints the state of each crawl in ./raw.
func (Raw) Status() error {
	return kfdata("status")
}

// Push copies ./raw to the private bucket.
//
// It only adds and updates objects. It does not delete objects in the bucket.
func (Raw) Push() error {
	return sh.RunV("gcloud", "storage", "rsync", "--recursive", "raw", rawBucket)
}

// Pull copies the private bucket's raw store to ./raw.
func (Raw) Pull() error {
	return sh.RunV("gcloud", "storage", "rsync", "--recursive", rawBucket, "raw")
}

func kfdata(args ...string) error {
	return sh.RunV("go", append([]string{"run", "./cmd/kfdata"}, args...)...)
}

func split(s string) []string {
	return strings.Split(s, ",")
}
