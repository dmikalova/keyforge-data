//go:build mage

package main

import (
	"os"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
	"github.com/magefile/mage/target"
)

const (
	artistOCRSource = "tools/artistocr/main.swift"
	artistOCRBin    = "tmp/artistocr"
)

// Artists holds the targets that read the card artists. They run on macOS:
// the OCR is the Vision framework, which macOS has.
type Artists mg.Namespace

// Scan distills the dataset, then reads the artist of each printing that
// has none in overrides/<set>.json.
//
// It writes the sure matches to overrides/<set>.json and the others to the
// review queue (tmp/artists/review.json). Run artists:review next, then
// dataset:publish.
func (Artists) Scan() error {
	mg.Deps(Dataset.Distill, buildArtistOCR)
	return kfdata("artists-scan", "-ocr", artistOCRBin)
}

// Review asks for the artist of each printing in the review queue.
//
// Press Enter to take the first candidate, type a number to take another,
// or type the name. A name that is not in overrides/_artists.json is added after
// you confirm it. The image shows in the terminal in Ghostty, kitty, iTerm2,
// and VS Code (with terminal.integrated.enableImages), and in Preview in
// other terminals. Set KFDATA_SHOW to kitty, inline, open, or none to choose.
func (Artists) Review() error {
	show := os.Getenv("KFDATA_SHOW")
	if show == "" {
		show = "auto"
	}
	return kfdata("artists-review", "-show", show)
}

// buildArtistOCR compiles the Swift OCR tool when its source changed.
func buildArtistOCR() error {
	if newer, err := target.Path(artistOCRBin, artistOCRSource); err != nil || !newer {
		return err
	}
	return sh.RunV("swiftc", "-O", artistOCRSource, "-o", artistOCRBin)
}
