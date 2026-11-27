// Command kfdata crawls Master Vault into the raw store and computes the
// cover decks.
//
// Usage:
//
//	kfdata [-root dir] crawl-sets [-crawl id] <set...|all>
//	kfdata [-root dir] cover -crawl id <set...|all>
//	kfdata [-root dir] refresh [-crawl id] [-locales en,de,...] <set...|all>
//	kfdata [-root dir] status [-crawl id]
//	kfdata [-root dir] report [-out file] [crawl...]
//
// crawl-sets scans the deck feed of each set and keeps the decks that add a
// card. cover writes sources/mv/cover/<set>.json from a crawl-sets crawl.
// refresh fetches the cover decks of each set in each language. A stopped
// crawl continues when you run the same command with the same crawl ID.
package main

import (
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/png"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	_ "golang.org/x/image/webp"

	"github.com/dmikalova/keyforge-data/artists"
	"github.com/dmikalova/keyforge-data/card"
	"github.com/dmikalova/keyforge-data/crawl"
	"github.com/dmikalova/keyforge-data/distill"
	"github.com/dmikalova/keyforge-data/images"
	"github.com/dmikalova/keyforge-data/inspect"
	"github.com/dmikalova/keyforge-data/mv"
	"github.com/dmikalova/keyforge-data/overlay"
	"github.com/dmikalova/keyforge-data/raw"
	"github.com/dmikalova/keyforge-data/sets"
)

// coverDir is where the cover files go, relative to the repository root.
const coverDir = "sources/mv/cover"

const usage = `usage: kfdata [-root dir] <command> [flags] [args]

commands:
  crawl-sets [-crawl id] <set...|all>   scan the deck feed of each set
  cover -crawl id <set...|all>          write the cover decks of each set
  refresh [-crawl id] [-locales l,...] <set...|all>
                                        fetch the cover decks in each language
  status [-crawl id]                    print the state of the crawls
  report [-out file] [crawl...]         write the raw data report
  gigantic -out file <base URL> <art URL>
                                        write a gigantic composite image
  fix-art -out file <blank URL> <art URL>
                                        put another printing's art under a blank image
  check-art [-workers n]                print the card images with a blank art box
  threads <todo|add|rules> ...           list and edit the curated threads of related cards
  overrides lint [-dataset f]            check the override files against the dataset
  feed [-dataset f] [-old f] [-old-feed f]
                                        write data/latest.json and data/feed.xml for a new dataset
  distill [-out file] [-problems file] [-overrides dir] [-artists dir]
                                        write the dataset and its problems
  artists-scan [-dataset file] [-ocr bin] [-dir dir] [-cache dir]
                                        OCR the artist of each printing without one
  artists-review [-show auto|kitty|inline|open|none] [-dir dir] [-cache dir]
                                        ask for the artists that the scan was not sure of
`

func report(store *raw.Store, args []string) error {
	fs := flag.NewFlagSet("report", flag.ContinueOnError)
	out := fs.String("out", "docs/data-report.md", "report file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	ids := fs.Args()
	if len(ids) == 0 {
		var err error
		if ids, err = store.Crawls(); err != nil {
			return err
		}
	}
	f, err := os.Create(*out)
	if err != nil {
		return err
	}
	if err := inspect.Write(f, store, ids); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func main() {
	if err := runWithSignals(os.Args[1:]); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "kfdata:", err)
		os.Exit(1)
	}
}

// runWithSignals runs the command and cancels it on an interrupt, so a crawl
// saves its state before it stops.
func runWithSignals(args []string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return run(ctx, args, os.Stdout, os.Stderr)
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("kfdata", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", "raw", "raw store directory")
	fs.Usage = func() { _, _ = io.WriteString(stderr, usage) }
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		fs.Usage()
		return errors.New("no command")
	}
	log := slog.New(slog.NewTextHandler(stderr, nil))
	store, err := raw.Open(*root)
	if err != nil {
		return err
	}
	cmd, rest := fs.Arg(0), fs.Args()[1:]
	switch cmd {
	case "crawl-sets":
		return crawlSets(ctx, log, store, rest)
	case "cover":
		return cover(log, store, rest)
	case "refresh":
		return refresh(ctx, log, store, rest)
	case "status":
		return status(store, rest, stdout)
	case "report":
		return report(store, rest)
	case "gigantic":
		return giganticCmd(ctx, rest)
	case "fix-art":
		return fixArt(ctx, rest)
	case "check-art":
		return checkArt(ctx, rest, stdout)
	case "threads":
		return threadsCmd(rest, stdout)
	case "overrides":
		return overridesCmd(rest, stdout)
	case "feed":
		return feedCmd(rest, stdout)
	case "distill":
		return distillCmd(ctx, store, rest, stdout)
	case "artists-scan":
		return artistsScan(ctx, rest, stdout)
	case "artists-hints":
		return artistsHints(rest, stdout)
	case "artists-review":
		return artistsReview(rest, os.Stdin, stdout)
	default:
		fs.Usage()
		return fmt.Errorf("unknown command %q", cmd)
	}
}

func crawlSets(ctx context.Context, log *slog.Logger, store *raw.Store, args []string) error {
	fs := flag.NewFlagSet("crawl-sets", flag.ContinueOnError)
	id := fs.String("crawl", "sets-"+time.Now().UTC().Format("20060102"), "crawl ID")
	dir := fs.String("overrides", "overrides", "overrides directory with the unused card numbers")
	if err := fs.Parse(args); err != nil {
		return err
	}
	targets, err := sets.Select(fs.Args())
	if err != nil {
		return err
	}
	stop, err := stopRule(*dir)
	if err != nil {
		return err
	}
	c, err := store.OpenCrawl(*id, crawl.ModeFeedSets)
	if err != nil {
		return err
	}
	log.Info("crawl", "id", *id, "dir", c.Dir())
	runErr := crawl.FeedSets(ctx, log, mv.New(log), c, targets, stop)
	state := crawl.FeedSetsState{}
	if err := c.State(&state); err != nil {
		return errors.Join(runErr, err)
	}
	done := runErr == nil
	for _, s := range targets {
		if st := state[s.Slug]; st == nil || !st.Done {
			done = false
		}
	}
	return errors.Join(runErr, c.Close(state, done))
}

func cover(log *slog.Logger, store *raw.Store, args []string) error {
	fs := flag.NewFlagSet("cover", flag.ContinueOnError)
	id := fs.String("crawl", "", "crawl-sets crawl ID")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *id == "" {
		return errors.New("cover needs -crawl")
	}
	targets, err := sets.Select(fs.Args())
	if err != nil {
		return err
	}
	if err := os.MkdirAll(coverDir, 0o750); err != nil {
		return err
	}
	for _, s := range targets {
		cv, err := crawl.ComputeCover(store, *id, s)
		if err != nil {
			return fmt.Errorf("cover %s: %w", s.Slug, err)
		}
		path := filepath.Join(coverDir, s.Slug+".json")
		b, err := json.Marshal(cv, jsontext.WithIndent("  "))
		if err != nil {
			return err
		}
		if err := os.WriteFile(path, append(b, '\n'), 0o600); err != nil {
			return err
		}
		log.Info("cover", "set", s.Slug, "cards", cv.Cards, "decks", len(cv.Decks), "file", path)
	}
	return nil
}

func refresh(ctx context.Context, log *slog.Logger, store *raw.Store, args []string) error {
	fs := flag.NewFlagSet("refresh", flag.ContinueOnError)
	id := fs.String("crawl", "refresh-"+time.Now().UTC().Format("20060102"), "crawl ID")
	locales := fs.String("locales", strings.Join(mv.LocaleOrder, ","), "languages")
	if err := fs.Parse(args); err != nil {
		return err
	}
	targets, err := sets.Select(fs.Args())
	if err != nil {
		return err
	}
	var deckIDs []string
	for _, s := range targets {
		b, err := os.ReadFile(filepath.Join(coverDir, s.Slug+".json"))
		if err != nil {
			return fmt.Errorf("read the cover of %s (run cover first): %w", s.Slug, err)
		}
		var cv crawl.Cover
		if err := json.Unmarshal(b, &cv); err != nil {
			return err
		}
		deckIDs = append(deckIDs, cv.Decks...)
	}
	c, err := store.OpenCrawl(*id, crawl.ModeRefresh)
	if err != nil {
		return err
	}
	log.Info("refresh", "id", *id, "decks", len(deckIDs), "locales", *locales)
	return crawl.Refresh(ctx, log, mv.New(log), c, deckIDs, strings.Split(*locales, ","))
}

func status(store *raw.Store, args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	id := fs.String("crawl", "", "crawl ID; empty lists every crawl")
	if err := fs.Parse(args); err != nil {
		return err
	}
	ids := []string{*id}
	if *id == "" {
		var err error
		if ids, err = store.Crawls(); err != nil {
			return err
		}
	}
	for _, cid := range ids {
		b, err := os.ReadFile(filepath.Join(store.Root, "crawls", cid, "manifest.json"))
		if err != nil {
			return err
		}
		var m raw.Manifest
		if err := json.Unmarshal(b, &m); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(stdout, "%s  mode=%s decks=%d shards=%d done=%t updated=%s\n",
			m.ID, m.Mode, m.Decks, len(m.Shards), m.Done, m.Updated.Format(time.RFC3339))
		if m.Mode != crawl.ModeFeedSets {
			continue
		}
		var st crawl.FeedSetsState
		if err := json.Unmarshal(m.State, &st); err != nil {
			return err
		}
		for _, s := range sets.All {
			if x := st[s.Slug]; x != nil {
				_, _ = fmt.Fprintf(stdout, "  %-20s page=%-5d kept=%-4d cards=%-4d done=%t %s\n",
					s.Slug, x.Page, x.Kept, len(x.Seen), x.Done, x.Reason)
			}
		}
	}
	return nil
}

func distillCmd(ctx context.Context, store *raw.Store, args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("distill", flag.ContinueOnError)
	out := fs.String("out", "tmp/cards.json", "dataset file")
	problemsOut := fs.String("problems", "docs/dataset-problems.md", "problems file")
	problemsJSON := fs.String("problems-json", "tmp/problems.json", "problems file as JSON")
	overridesDir := fs.String("overrides", "overrides", "overrides directory")
	artistsDir := fs.String("artists", "overrides", "overrides directory with the artists")
	if err := fs.Parse(args); err != nil {
		return err
	}
	printingArtists, err := artists.Load(*artistsDir)
	if err != nil {
		return err
	}
	aliases, err := artists.LoadAliases(*artistsDir)
	if err != nil {
		return err
	}
	aliases.CanonicalAll(printingArtists)
	cards, err := distill.LoadCards(store)
	if err != nil {
		return err
	}
	decks, deckSets, err := distill.LoadDecks(store)
	if err != nil {
		return err
	}
	overrides, err := distill.LoadOverrides(*overridesDir)
	if err != nil {
		return err
	}
	images, err := distill.ListImages(ctx, &http.Client{Timeout: time.Minute})
	if err != nil {
		return err
	}
	d, problems := distill.Run(distill.Input{
		Cards:         cards,
		Images:        images,
		Overrides:     overrides,
		Artists:       printingArtists,
		ArtistAliases: aliases,
		Decks:         decks,
		DeckSets:      deckSets,
		Now:           time.Now(),
	})
	b, err := json.Marshal(d, json.Deterministic(true))
	if err != nil {
		return err
	}
	if err := os.WriteFile(*out, b, 0o600); err != nil {
		return err
	}
	pj, err := json.Marshal(problems)
	if err != nil {
		return err
	}
	if err := os.WriteFile(*problemsJSON, pj, 0o600); err != nil {
		return err
	}
	f, err := os.Create(*problemsOut)
	if err != nil {
		return err
	}
	if err := distill.WriteProblems(f, d.Version, problems); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "version %s: %d cards, %d sets, %d problems\n",
		d.Version, len(d.Cards), len(d.Sets), len(problems))
	return err
}

// giganticCmd downloads the two halves of a gigantic creature and writes
// the composite card tile as a PNG.
func giganticCmd(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("gigantic", flag.ContinueOnError)
	out := fs.String("out", "", "PNG file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 2 || *out == "" {
		return errors.New("usage: gigantic -out file.png <base URL> <art URL>")
	}
	client := &http.Client{Timeout: time.Minute}
	var halves [2]image.Image
	for i, u := range fs.Args() {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		img, err := png.Decode(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			return fmt.Errorf("%s: %w", u, err)
		}
		halves[i] = img
	}
	f, err := os.Create(*out)
	if err != nil {
		return err
	}
	if err := png.Encode(f, images.Gigantic(halves[0], halves[1])); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// checkArt downloads each card image in the public bucket and prints the
// images whose art box is blank.
func checkArt(ctx context.Context, args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("check-art", flag.ContinueOnError)
	workers := fs.Int("workers", 8, "parallel downloads")
	if err := fs.Parse(args); err != nil {
		return err
	}
	client := &http.Client{Timeout: time.Minute}
	names, err := distill.ListImages(ctx, client)
	if err != nil {
		return err
	}
	jobs := make(chan string)
	var mu sync.Mutex
	var blank, failed []string
	var wg sync.WaitGroup
	for range *workers {
		wg.Go(func() {
			for n := range jobs {
				img, err := fetchImage(ctx, client, card.ImageBase+n)
				mu.Lock()
				switch {
				case err != nil:
					failed = append(failed, n+": "+err.Error())
				case images.BlankArt(img):
					blank = append(blank, n)
				}
				mu.Unlock()
			}
		})
	}
	for _, n := range names {
		jobs <- n
	}
	close(jobs)
	wg.Wait()
	slices.Sort(blank)
	slices.Sort(failed)
	for _, n := range blank {
		_, _ = fmt.Fprintln(stdout, "blank", n)
	}
	for _, n := range failed {
		_, _ = fmt.Fprintln(stdout, "failed", n)
	}
	_, err = fmt.Fprintf(
		stdout,
		"checked %d images: %d blank, %d failed\n",
		len(names),
		len(blank),
		len(failed),
	)
	return err
}

// fetchImage downloads and decodes a PNG or webp image.
func fetchImage(ctx context.Context, client *http.Client, u string) (image.Image, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New(resp.Status)
	}
	img, _, err := image.Decode(resp.Body)
	return img, err
}

// fixArt puts the art of another printing under an image with a blank art
// box, and writes a PNG.
func fixArt(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("fix-art", flag.ContinueOnError)
	out := fs.String("out", "", "PNG file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 2 || *out == "" {
		return errors.New("usage: fix-art -out file.png <blank image URL> <art image URL>")
	}
	client := &http.Client{Timeout: time.Minute}
	top, err := fetchImage(ctx, client, fs.Arg(0))
	if err != nil {
		return err
	}
	art, err := fetchImage(ctx, client, fs.Arg(1))
	if err != nil {
		return err
	}
	if !images.BlankArt(top) {
		return errors.New("the first image has art")
	}
	f, err := os.Create(*out)
	if err != nil {
		return err
	}
	if err := png.Encode(f, images.Underlay(top, art)); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// stopRule returns the default stop rule with the unused card numbers of the
// set override files.
func stopRule(dir string) (crawl.StopRule, error) {
	stop := crawl.DefaultStop
	files, err := overlay.LoadAll(dir)
	if err != nil {
		return stop, err
	}
	stop.Unused = map[string]map[string]bool{}
	for name, f := range files {
		if !overlay.IsSet(name) || len(f.Unused) == 0 {
			continue
		}
		stop.Unused[name] = map[string]bool{}
		for n := range f.Unused {
			stop.Unused[name][n] = true
		}
	}
	return stop, nil
}
