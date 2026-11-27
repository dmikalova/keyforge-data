package main

import (
	"context"
	"encoding/json/v2"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/dmikalova/keyforge-data/card"
	"github.com/dmikalova/keyforge-data/distill"
	"github.com/dmikalova/keyforge-data/images"
	"github.com/dmikalova/keyforge-data/overlay"
)

// artCheck is the check result of one image generation.
type artCheck struct {
	Blank bool `json:"blank,omitempty"`
	Grey  bool `json:"grey,omitempty"`
}

// checkArt checks each card image in the public bucket. It prints the
// images whose art box is blank (images.BlankArt), and the images of cards
// with a house that have the grey frame of a maverick render
// (images.GreyFrame). The cache keeps the result of each image generation,
// so a run downloads only new images. With -write, it adds the grey frames
// to the greyFrame list of the set override files.
func checkArt(ctx context.Context, args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("check-art", flag.ContinueOnError)
	workers := flags.Int("workers", 8, "parallel downloads")
	dataset := flags.String(
		"dataset",
		"tmp/cards.json",
		"dataset that gives the cards with a house",
	)
	cacheFile := flags.String(
		"cache",
		"tmp/art-checks.json",
		"results of the images checked before",
	)
	dir := flags.String("overrides", "overrides", "overrides directory")
	write := flags.Bool("write", false, "add the grey frames to the overrides")
	if err := flags.Parse(args); err != nil {
		return err
	}
	housed, err := housedPrintings(*dataset)
	if err != nil {
		return err
	}
	cache, err := loadArtChecks(*cacheFile)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: time.Minute}
	names, err := distill.ListImages(ctx, client)
	if err != nil {
		return err
	}
	var todo []string
	for _, n := range names {
		if _, ok := cache[n]; !ok {
			todo = append(todo, n)
		}
	}
	failed := runArtChecks(ctx, client, todo, *workers, cache)
	if err := saveArtChecks(*cacheFile, cache); err != nil {
		return err
	}
	var blank, grey []string
	for _, n := range names {
		r := cache[n]
		base, _, _ := strings.Cut(n, "?")
		if r.Blank {
			blank = append(blank, base)
		}
		if title, ok := housed[base]; ok && r.Grey {
			grey = append(grey, base+" "+title)
		}
	}
	slices.Sort(blank)
	slices.Sort(grey)
	slices.Sort(failed)
	for _, n := range blank {
		_, _ = fmt.Fprintln(stdout, "blank", n)
	}
	for _, n := range grey {
		_, _ = fmt.Fprintln(stdout, "grey frame", n)
	}
	for _, n := range failed {
		_, _ = fmt.Fprintln(stdout, "failed", n)
	}
	if *write {
		added, err := writeGreyFrames(*dir, grey)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(stdout, "added %d grey frames to the overrides\n", added)
	}
	_, err = fmt.Fprintf(stdout,
		"checked %d images (%d new): %d blank, %d grey frames, %d failed\n",
		len(names), len(todo), len(blank), len(grey), len(failed))
	return err
}

// runArtChecks downloads and checks the images, and adds the results to the
// cache. It returns the images that failed.
func runArtChecks(
	ctx context.Context,
	client *http.Client,
	names []string,
	workers int,
	cache map[string]artCheck,
) []string {
	jobs := make(chan string)
	var mu sync.Mutex
	var failed []string
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			for n := range jobs {
				img, err := fetchImage(ctx, client, card.ImageBase+n)
				mu.Lock()
				if err != nil {
					failed = append(failed, n+": "+err.Error())
				} else {
					cache[n] = artCheck{Blank: images.BlankArt(img), Grey: images.GreyFrame(img)}
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
	return failed
}

// housedPrintings returns the image names of the printings that must have a
// house frame, mapped to the card title: the printings of the cards with a
// house, not the anomalies and not the gigantic composites. The name is the
// printing's own image, <set>/<number>.webp, also where the dataset shows
// another printing's image.
func housedPrintings(path string) (map[string]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var d card.Dataset
	if err := json.Unmarshal(b, &d); err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, c := range d.Cards {
		if c.House == "" || c.HasFlag(distill.FlagGigantic) {
			continue
		}
		for _, p := range c.Printings {
			if !p.Anomaly {
				out[p.Set+"/"+p.Number+".webp"] = c.Title
			}
		}
	}
	return out, nil
}

// writeGreyFrames adds each grey frame, "<set>/<number>.webp <title>", to
// the greyFrame list of its set override file. It returns the number of
// numbers that it added.
func writeGreyFrames(dir string, grey []string) (int, error) {
	bySet := map[string][]string{}
	for _, g := range grey {
		name, _, _ := strings.Cut(g, " ")
		set, file, ok := strings.Cut(name, "/")
		if !ok {
			continue
		}
		bySet[set] = append(bySet[set], strings.TrimSuffix(file, ".webp"))
	}
	added := 0
	for set, nums := range bySet {
		f, err := overlay.Load(dir, set)
		if err != nil {
			return added, err
		}
		var fresh []string
		for _, n := range nums {
			if !slices.Contains(f.GreyFrame, n) {
				fresh = append(fresh, n)
			}
		}
		if len(fresh) == 0 {
			continue
		}
		if err := overlay.Update(dir, set, func(f *overlay.File) {
			f.GreyFrame = append(f.GreyFrame, fresh...)
			slices.Sort(f.GreyFrame)
		}); err != nil {
			return added, err
		}
		added += len(fresh)
	}
	return added, nil
}

func loadArtChecks(path string) (map[string]artCheck, error) {
	out := map[string]artCheck{}
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return out, nil
}

func saveArtChecks(path string, cache map[string]artCheck) error {
	b, err := json.Marshal(cache, json.Deterministic(true))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}
