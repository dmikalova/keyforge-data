// Package sets is the table of KeyForge sets with their Master Vault expansion
// IDs. Master Vault has no expansions endpoint, so the IDs are pinned here.
package sets

import (
	"fmt"
	"slices"
	"strings"
)

// Set is one KeyForge set.
type Set struct {
	// Slug is the lowercase identifier used in paths and image URLs.
	Slug string `json:"slug"`
	// Code is the short printed code, for example "GR".
	Code string `json:"code"`
	// Name is the full English name.
	Name string `json:"name"`
	// Expansion is the Master Vault expansion ID.
	Expansion int `json:"expansion"`
}

// All lists every set in release order.
var All = []Set{
	{"callofthearchons", "CotA", "Call of the Archons", 341},
	{"ageofascension", "AoA", "Age of Ascension", 435},
	{"worldscollide", "WC", "Worlds Collide", 452},
	{"anomalyexpansion", "AE", "Anomaly Expansion", 453},
	{"massmutation", "MM", "Mass Mutation", 479},
	{"darktidings", "DT", "Dark Tidings", 496},
	{"windsofexchange", "WoE", "Winds of Exchange", 600},
	{"unchained2022", "UC22", "Unchained 2022", 601},
	{"vaultmasters2023", "VM23", "Vault Masters 2023", 609},
	{"grimreminders", "GR", "Grim Reminders", 700},
	{"menagerie", "Men", "Menagerie", 722},
	{"vaultmasters2024", "VM24", "Vault Masters 2024", 737},
	{"aemberskies", "AS", "Æmber Skies", 800},
	{"tokensofchange", "ToC", "Tokens of Change", 855},
	{"moremutation", "MoMu", "More Mutation", 874},
	{"propheticvisions", "PV", "Prophetic Visions", 886},
	{"martiancivilwar", "MCW", "Martian Civil War", 892},
	{"discovery", "D", "Discovery", 907},
	{"crucibleclash", "CC", "Crucible Clash", 918},
	{"draconianmeasures", "DM", "Draconian Measures", 928},
	{"vaultmasters2025", "VM25", "Vault Masters 2025", 939},
	{"vaultmasters2026", "VM26", "Vault Masters 2026", 964},
}

// BySlug returns the set with the slug.
func BySlug(slug string) (Set, bool) {
	i := slices.IndexFunc(All, func(s Set) bool { return s.Slug == slug })
	if i < 0 {
		return Set{}, false
	}
	return All[i], true
}

// ByExpansion returns the set with the Master Vault expansion ID.
func ByExpansion(id int) (Set, bool) {
	i := slices.IndexFunc(All, func(s Set) bool { return s.Expansion == id })
	if i < 0 {
		return Set{}, false
	}
	return All[i], true
}

// Select returns the sets for the arguments. The argument "all" selects every
// set. An unknown slug is an error.
func Select(args []string) ([]Set, error) {
	if len(args) == 1 && args[0] == "all" {
		return All, nil
	}
	if len(args) == 0 {
		return nil, fmt.Errorf("give one or more set slugs, or all: %s", Slugs())
	}
	out := make([]Set, 0, len(args))
	for _, a := range args {
		s, ok := BySlug(a)
		if !ok {
			return nil, fmt.Errorf("unknown set %q; known sets: %s", a, Slugs())
		}
		out = append(out, s)
	}
	return out, nil
}

// Slugs returns every slug, comma-separated, for error messages.
func Slugs() string {
	out := make([]string, len(All))
	for i, s := range All {
		out[i] = s.Slug
	}
	return strings.Join(out, ", ")
}
