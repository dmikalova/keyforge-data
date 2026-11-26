// Package overlay is the format of the override files in overrides/. The
// overrides are the overlay layer: the curated data that distillation puts
// over the data from Master Vault. The dataset keeps the Master Vault data
// where it can, and the overlay corrects it, adds to it, or names it for
// people (ADR 0008).
//
// Every file in overrides/ is one File in JSON:
//
//   - <set>.json has the data of one set, by set slug: card fixes, artists,
//     houseless kinds, and name aliases.
//   - _threads.json has the curated threads and notes. kfdata threads add
//     writes it. People edit it.
//   - _deckrules.json has the deck rules and notes of the Archon Arcana
//     sheet. kfdata threads rules writes it. Do not edit it.
//   - _artists.json has the known artists and the artist aliases.
//   - _labels.json has the names for people of the data values.
package overlay

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/dmikalova/keyforge-data/card"
)

// The names of the files that are not a set.
const (
	ThreadsFile = "_threads"
	RulesFile   = "_deckrules"
	ArtistsFile = "_artists"
	LabelsFile  = "_labels"
)

// File is one override file. A section that a file does not use is empty.
type File struct {
	// Kinds maps a Master Vault house of houseless cards to its kind label,
	// for example "Skybeast" to "skybeast". A set file.
	Kinds map[string]string `json:"kinds,omitempty"`
	// Aliases maps a card title to extra names for name search. A set file.
	Aliases map[string][]string `json:"aliases,omitempty"`
	// Cards maps a collector number to corrections for that printing's card.
	// A set file.
	Cards map[string]CardFix `json:"cards,omitempty"`
	// Artists maps a collector number to the artist of the printing. Master
	// Vault has no artists. kfdata artists scan writes them. A set file.
	Artists map[string]string `json:"artists,omitempty"`
	// Threads are curated threads of related cards. A curated thread wins
	// over a deck rule with the same cards or fewer. _threads.json.
	Threads []Thread `json:"threads,omitempty"`
	// Rules are the deck rules of related cards. _deckrules.json.
	Rules []Rule `json:"rules,omitempty"`
	// Notes are the notes of single cards. A note in _threads.json replaces
	// the notes of _deckrules.json of the same card and kind.
	Notes []NoteRule `json:"notes,omitempty"`
	// KnownArtists are the artist names that the artist scan can choose.
	// _artists.json.
	KnownArtists []string `json:"knownArtists,omitempty"`
	// ArtistAliases maps another spelling of an artist to the artist name
	// that the printings use. _artists.json.
	ArtistAliases map[string]string `json:"artistAliases,omitempty"`
	// Labels are names for people, by kind and value, for example
	// "groups": {"anomaly": "Anomaly"}. They win over the names that the
	// Master Vault data gives. _labels.json.
	Labels map[string]map[string]string `json:"labels,omitempty"`
}

// CardFix corrects a Master Vault mistake.
type CardFix struct {
	Power *card.Power `json:"power,omitempty"`
	Armor *int        `json:"armor,omitempty"`
	// Note says why, for people.
	Note string `json:"note,omitempty"`
}

// Thread is a curated thread of related cards. Distillation adds a Related
// entry with the thread's label to each card in it.
type Thread struct {
	Label  string   `json:"label"`
	Titles []string `json:"titles"`
}

// Rule is a deck rule of related cards, for example "Faygin always appears
// in a deck with at least 2 copies of Urchin".
//
//   - A rule with Cards adds a Related entry with the label and the With
//     cards to each of the Cards.
//   - A rule without Cards is a group: each of the With cards gets the
//     others.
//
// A rule does not add an entry that an earlier thread of the card already
// has: the curated threads win.
type Rule struct {
	// Kind is the rule type of the source, for example "always".
	Kind  string   `json:"kind"`
	Label string   `json:"label"`
	Cards []string `json:"cards,omitempty"`
	With  []string `json:"with"`
}

// NoteRule is a note about one card: "Plague Rat: Always 3+ copies". A
// title can end in " (Evil Twin)", " (Anomaly)", or a house in parentheses
// for the card of that house: "Armageddon Cloak (Redemption)".
type NoteRule struct {
	Kind  string `json:"kind"`
	Title string `json:"title"`
	Text  string `json:"text"`
}

// IsSet reports whether a file name is a set file, not one of the shared
// files.
func IsSet(name string) bool {
	return !strings.HasPrefix(name, "_")
}

// Load reads one file. A missing file is an empty File. An unknown field is
// an error, so that a misspelled section does not go unseen.
func Load(dir, name string) (File, error) {
	var f File
	b, err := os.ReadFile(filepath.Join(dir, name+".json"))
	if errors.Is(err, fs.ErrNotExist) {
		return f, nil
	}
	if err != nil {
		return f, err
	}
	if err := json.Unmarshal(b, &f, json.RejectUnknownMembers(true)); err != nil {
		return f, fmt.Errorf("%s.json: %w", name, err)
	}
	return f, nil
}

// LoadAll reads every file in dir, by name without ".json". A missing
// directory is an empty map.
func LoadAll(dir string) (map[string]File, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	out := map[string]File{}
	for _, p := range files {
		name := strings.TrimSuffix(filepath.Base(p), ".json")
		f, err := Load(dir, name)
		if err != nil {
			return nil, err
		}
		out[name] = f
	}
	return out, nil
}

// Save writes one file with sorted keys and one entry per line, so that
// the diffs stay small.
func Save(dir, name string, f File) error {
	b, err := json.Marshal(
		f,
		json.Deterministic(true),
		jsontext.Multiline(true),
		jsontext.WithIndent("  "),
	)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, name+".json"), append(b, '\n'), 0o600)
}

// Update reads one file, changes it, and writes it.
func Update(dir, name string, change func(*File)) error {
	f, err := Load(dir, name)
	if err != nil {
		return err
	}
	change(&f)
	return Save(dir, name, f)
}
