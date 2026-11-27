package main

import (
	"encoding/json/v2"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/dmikalova/keyforge-data/card"
	"github.com/dmikalova/keyforge-data/overlay"
)

// TestHousedPrintings checks that only the printings of cards with a house
// are checked for a grey frame, by their own image name.
func TestHousedPrintings(t *testing.T) {
	d := card.Dataset{Cards: []card.Card{
		{Title: "The Grim Reaper", House: "geistoid", Printings: []card.Printing{
			{Set: "worldscollide", Number: "A07", Anomaly: true},
			{Set: "menagerie", Number: "233", Maverick: true, Image: "grimreminders/155.webp"},
		}},
		{
			Title:     "Dark Æmber Vault",
			Printings: []card.Printing{{Set: "massmutation", Number: "001"}},
		},
		{
			Title: "Ultra Gravitron", House: "logos", Flags: []string{"gigantic"},
			Printings: []card.Printing{{Set: "massmutation", Number: "125"}},
		},
	}}
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "cards.json")
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := housedPrintings(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got["menagerie/233.webp"] != "The Grim Reaper" {
		t.Errorf("housed printings = %v, want only menagerie/233.webp", got)
	}
}

func TestWriteGreyFrames(t *testing.T) {
	dir := t.TempDir()
	if err := overlay.Save(dir, "menagerie", overlay.File{GreyFrame: []string{"500"}}); err != nil {
		t.Fatal(err)
	}
	grey := []string{"menagerie/233.webp The Grim Reaper", "menagerie/500.webp Other"}
	// 500 is in the list already, so only 233 is new.
	if added, err := writeGreyFrames(dir, grey); err != nil || added != 1 {
		t.Fatalf("added = %d, err = %v, want 1 and no error", added, err)
	}
	f, err := overlay.Load(dir, "menagerie")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(f.GreyFrame, []string{"233", "500"}) {
		t.Errorf("greyFrame = %v, want [233 500]", f.GreyFrame)
	}
}
