package main

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/dmikalova/keyforge-data/card"
	"github.com/dmikalova/keyforge-data/distill"
)

const threadsUsage = `usage: kfdata threads <todo|add|rules> [flags]

  todo [-dataset file] [-overrides dir]
        print the Variant printings that are in no curated thread, in
        candidate families: runs of numbers in one set and one house
  add [-overrides dir] [-dataset file] -label "Label" "Title" "Title" ...
        add a curated thread, or add titles to the thread with that label
  rules [-overrides dir] [-dataset file] [-in file]
        write overrides/_deckrules.json from the Archon Arcana
        relationships_and_notes sheet (TSV)
`

// threadsCmd edits the curated threads of related cards in
// overrides/_threads.json.
func threadsCmd(args []string, stdout io.Writer) error {
	if len(args) == 0 {
		_, _ = io.WriteString(stdout, threadsUsage)
		return errors.New("no threads command")
	}
	fs := flag.NewFlagSet("threads "+args[0], flag.ContinueOnError)
	dataset := fs.String("dataset", "tmp/cards.json", "distilled dataset")
	overrides := fs.String("overrides", "overrides", "overrides directory")
	label := fs.String("label", "", "thread label")
	in := fs.String("in", "tmp/deckrules.tsv", "deck rules sheet (TSV)")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	file := filepath.Join(*overrides, "_threads.json")
	threads, err := readThreads(file)
	if err != nil {
		return err
	}
	cards, err := readDataset(*dataset)
	if err != nil {
		return err
	}
	switch args[0] {
	case "todo":
		return threadsTodo(stdout, cards, threads)
	case "rules":
		return threadsRules(stdout, *in, filepath.Join(*overrides, "_deckrules.json"), cards)
	case "add":
		if *label == "" || fs.NArg() < 2 {
			return errors.New(`usage: kfdata threads add -label "Label" "Title" "Title"`)
		}
		return threadsAdd(stdout, file, cards, threads, *label, fs.Args())
	default:
		_, _ = io.WriteString(stdout, threadsUsage)
		return fmt.Errorf("unknown threads command %q", args[0])
	}
}

func readThreads(file string) ([]distill.Thread, error) {
	b, err := os.ReadFile(file)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var o distill.Override
	if err := json.Unmarshal(b, &o); err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	return o.Threads, nil
}

func readDataset(file string) ([]card.Card, error) {
	b, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("%w (run mage dataset:distill first)", err)
	}
	var d card.Dataset
	if err := json.Unmarshal(b, &d); err != nil {
		return nil, err
	}
	return d.Cards, nil
}

// variant is one Variant printing.
type variant struct {
	set, house, number, title string
}

func threadsTodo(w io.Writer, cards []card.Card, threads []distill.Thread) error {
	inThread := map[string]bool{}
	for _, th := range threads {
		for _, t := range th.Titles {
			inThread[t] = true
		}
	}
	var vs []variant
	for _, c := range cards {
		if inThread[c.Title] {
			continue
		}
		for _, p := range c.Printings {
			if p.Rarity == "Variant" {
				vs = append(vs, variant{p.Set, c.House, p.Number, c.Title})
			}
		}
	}
	slices.SortFunc(vs, func(a, b variant) int {
		return strings.Compare(a.set+a.house+a.number, b.set+b.house+b.number)
	})
	fams := families(vs)
	for _, f := range fams {
		var titles []string
		for _, v := range f {
			titles = append(titles, v.title)
		}
		first, last := f[0], f[len(f)-1]
		_, err := fmt.Fprintf(w, "# %s %s %s-%s (%d cards)\nmage threads:add \"LABEL\" %q\n\n",
			first.set, first.house, first.number, last.number, len(f), strings.Join(titles, ";"))
		if err != nil {
			return err
		}
	}
	_, err := fmt.Fprintf(
		w,
		"%d Variant printings in no thread, in %d candidate families.\n",
		len(vs),
		len(fams),
	)
	return err
}

// families groups the variants of one set and one house that share a name
// word: the last word ("Brew", "Bane", "Ambassador", "Fiend" in
// "Dino-Fiend"), or else the first word ("Monument"). A variant without a
// shared word is a family of one.
func families(vs []variant) [][]variant {
	count := map[string]int{}
	for _, v := range vs {
		first, last := nameWords(v.title)
		count[v.set+"\x00"+v.house+"\x00L"+last]++
		count[v.set+"\x00"+v.house+"\x00F"+first]++
	}
	index := map[string]int{}
	var out [][]variant
	for _, v := range vs {
		first, last := nameWords(v.title)
		base := v.set + "\x00" + v.house + "\x00"
		key := base + "L" + last
		if count[key] < 2 && count[base+"F"+first] >= 2 {
			key = base + "F" + first
		}
		if count[key] < 2 {
			key = base + "#" + v.number
		}
		i, ok := index[key]
		if !ok {
			i = len(out)
			index[key] = i
			out = append(out, nil)
		}
		out[i] = append(out[i], v)
	}
	return out
}

// nameWords returns the first and the last word of a title, split at spaces
// and hyphens.
func nameWords(title string) (string, string) {
	ws := strings.FieldsFunc(title, func(r rune) bool { return r == ' ' || r == '-' })
	if len(ws) == 0 {
		return "", ""
	}
	return ws[0], ws[len(ws)-1]
}

func threadsAdd(
	w io.Writer,
	file string,
	cards []card.Card,
	threads []distill.Thread,
	label string,
	titles []string,
) error {
	known := map[string]bool{}
	for _, c := range cards {
		known[c.Title] = true
	}
	for _, t := range titles {
		if !known[t] {
			return fmt.Errorf("no card has the title %q (check the curly apostrophe ’)", t)
		}
	}
	i := slices.IndexFunc(threads, func(th distill.Thread) bool { return th.Label == label })
	if i < 0 {
		threads = append(threads, distill.Thread{Label: label})
		i = len(threads) - 1
	}
	for _, t := range titles {
		if !slices.Contains(threads[i].Titles, t) {
			threads[i].Titles = append(threads[i].Titles, t)
		}
	}
	b, err := json.Marshal(
		distill.Override{Threads: threads},
		jsontext.Multiline(true),
		jsontext.WithIndent("  "),
	)
	if err != nil {
		return err
	}
	if err := os.WriteFile(file, append(b, '\n'), 0o600); err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "thread %q has %d cards\n", label, len(threads[i].Titles))
	return err
}
