//go:build mage

package main

import (
	"os"
	"path/filepath"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
	"github.com/magefile/mage/target"
)

const (
	artistOCRSource = "tools/artistocr/main.swift"
	artistOCRBin    = "tmp/artistocr"
	// artistHints is the title and artist list of the Archon Arcana bots
	// (docs/sources.md). The repository has no license, so the scan
	// downloads it to tmp and does not commit it.
	artistHints    = "tmp/hints/illustrators.csv"
	artistHintsURL = "https://raw.githubusercontent.com/blinkingline/archonarcanabots/" +
		"c47450df3f2ea6dbf731aa828cb3fd9c9e612356/skyjedi/illustrators.csv"
)

// Artists holds the targets that read the card artists. They run on macOS:
// the OCR is the Vision framework, which macOS has.
type Artists mg.Namespace

// Scan distills the dataset, then reads the artist of each printing that
// has none in overrides/<set>.json. It does nothing when every printing has
// an artist, so a crawl pipeline can run it after each set.
//
// The OCR matches the known artists and the artists of the hint list. The
// hint list then settles the printings where the OCR is not sure: the OCR
// and the hint agree, or the OCR reads nothing and the hint agrees with the
// card's other printings, or the card is new. It writes the artists to
// overrides/<set>.json and the rest to the review queue
// (tmp/artists/review.json). Run artists:review next, then dataset:publish.
func (Artists) Scan() error {
	mg.Deps(Dataset.Distill, buildArtistOCR, downloadArtistHints)
	if err := kfdata("artists-scan", "-ocr", artistOCRBin, "-hints", artistHints); err != nil {
		return err
	}
	return kfdata("artists-hints", "-hints", artistHints)
}

// downloadArtistHints downloads the hint list once.
func downloadArtistHints() error {
	if _, err := os.Stat(artistHints); err == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(artistHints), 0o750); err != nil {
		return err
	}
	return sh.Run("curl", "-sfL", "-o", artistHints, artistHintsURL)
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
