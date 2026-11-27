//go:build mage

package main

import (
	"bytes"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
)

const (
	// latestURL is the public URL of the pointer to the published dataset
	// version (ADR 0007). The sites read it and then the version file.
	latestURL    = "https://storage.googleapis.com/mklv-keyforge-cards-public/data/latest.json"
	datasetFile  = "tmp/cards.json"
	problemsFile = "docs/dataset-problems.md"
	problemsJSON = "tmp/problems.json"
	// notifiedFile is the problems file of the last Discord post.
	notifiedFile = "tmp/dataset-problems.notified.md"
	repo         = "dmikalova/keyforge-data"
	// discordSecrets holds the maintenance channel webhook.
	discordSecrets = "../infrastructure/secrets/discord.sops.json"
)

// Dataset holds the targets that make and publish the card dataset.
type Dataset mg.Namespace

// Distill writes tmp/cards.json and docs/dataset-problems.md from ./raw.
func (Dataset) Distill() error {
	return kfdata(
		"-root",
		rawRoot(),
		"distill",
		"-out",
		datasetFile,
		"-problems",
		problemsFile,
		"-problems-json",
		problemsJSON,
	)
}

// Publish distills the dataset, fills missing card images from Master
// Vault, uploads it to the public bucket if its
// version changed, and posts changed problems to the maintenance Discord channel.
func (Dataset) Publish() error {
	mg.Deps(Dataset.Distill)
	if err := kfdata("overrides", "lint", "-dataset", datasetFile); err != nil {
		return err
	}
	filled, err := fillImages()
	if err != nil {
		return err
	}
	grey, err := checkGreyFrames()
	if err != nil {
		return err
	}
	if filled > 0 || grey > 0 {
		if err := (Dataset{}).Distill(); err != nil {
			return err
		}
	}
	local, cards, err := version(datasetFile)
	if err != nil {
		return err
	}
	remote, remoteCards := publishedVersion()
	if err := checkShrink(cards, remoteCards); err != nil {
		return err
	}
	if remote == local {
		fmt.Println("dataset", local, "is already published")
	} else {
		if err := uploadVersion(local, remote); err != nil {
			return err
		}
		fmt.Println("published dataset", local, "over", remote)
	}
	if err := pruneVersions(local); err != nil {
		return err
	}
	return notifyProblems()
}

// datasetHead is the part of a dataset that publish reads.
type datasetHead struct {
	Version string           `json:"version"`
	Cards   []jsontext.Value `json:"cards"`
}

// version returns the version and the card count of a dataset file.
func version(path string) (string, int, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", 0, err
	}
	var d datasetHead
	if err := json.Unmarshal(b, &d); err != nil {
		return "", 0, err
	}
	return d.Version, len(d.Cards), nil
}

// checkShrink refuses a dataset that is empty or has less than 90% of the
// cards of the published dataset. A wrong raw store path once published an
// empty dataset, and the site showed no cards. Set KFDATA_FORCE=1 to publish
// a smaller dataset on purpose.
func checkShrink(cards, published int) error {
	if os.Getenv("KFDATA_FORCE") == "1" {
		return nil
	}
	if cards == 0 || cards < published*9/10 {
		return fmt.Errorf(
			"the dataset has %d cards and the published dataset has %d: not published (set KFDATA_FORCE=1 to publish it)",
			cards,
			published,
		)
	}
	return nil
}

// publishedVersion returns the version and the card count of the published
// dataset from the pointer file, or empty and 0.
func publishedVersion() (string, int) {
	client := &http.Client{Timeout: time.Minute}
	resp, err := client.Get(latestURL)
	if err != nil {
		return "", 0
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", 0
	}
	var l struct {
		Version string `json:"version"`
		Cards   int    `json:"cards"`
	}
	if err := json.UnmarshalRead(resp.Body, &l); err != nil {
		return "", 0
	}
	return l.Version, l.Cards
}

// notifyProblems posts the problems summary to the maintenance Discord
// channel when the problems differ from the last problems that it posted
// (tmp/dataset-problems.notified.md).
func notifyProblems() error {
	body, err := os.ReadFile(problemsFile)
	if err != nil {
		return err
	}
	old, err := os.ReadFile(notifiedFile)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if withoutVersion(string(old)) == withoutVersion(string(body)) {
		fmt.Println("the dataset problems did not change")
		return nil
	}
	secrets, err := sh.Output("sops", "-d", discordSecrets)
	if err != nil {
		return err
	}
	var s struct {
		URL string `json:"MAINTENANCE_WEBHOOK_URL"`
	}
	if err := json.Unmarshal([]byte(secrets), &s); err != nil {
		return err
	}
	msg := "**keyforge-data: the dataset problems changed**\n" + problemsSummary(string(body)) +
		"\nSee docs/dataset-problems.md in " + repo + "."
	payload, err := json.Marshal(map[string]string{"content": msg})
	if err != nil {
		return err
	}
	resp, err := http.Post(s.URL, "application/json", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("discord: %s", resp.Status)
	}
	fmt.Println("posted the dataset problems to Discord")
	return os.WriteFile(notifiedFile, body, 0o600)
}

// withoutVersion drops the line with the dataset version, which changes on
// every distillation.
func withoutVersion(s string) string {
	var out []string
	for line := range strings.SplitSeq(s, "\n") {
		if !strings.HasPrefix(line, "Dataset version") {
			out = append(out, line)
		}
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

// problemsSummary returns the "Dataset version ... Problems: n." line and the
// count of each kind.
func problemsSummary(body string) string {
	var parts []string
	for line := range strings.SplitSeq(body, "\n") {
		switch {
		case strings.HasPrefix(line, "Dataset version"):
			parts = append([]string{line}, parts...)
		case strings.HasPrefix(line, "## "):
			parts = append(parts, "- "+strings.TrimPrefix(line, "## "))
		}
	}
	return strings.Join(parts, "\n")
}

// Images fills the missing card images that the last distillation found.
func (Dataset) Images() error {
	_, err := fillImages()
	return err
}

// fillImages downloads each missing card image from the Master Vault URL in
// tmp/problems.json, makes a 300×420 webp, and uploads it with the image
// path rule. It returns the number of images that it uploaded.
func fillImages() (int, error) {
	b, err := os.ReadFile(problemsJSON)
	if err != nil {
		return 0, err
	}
	var problems []struct {
		Kind   string `json:"kind"`
		Set    string `json:"set"`
		Number string `json:"number"`
		Detail string `json:"detail"`
	}
	if err := json.Unmarshal(b, &problems); err != nil {
		return 0, err
	}
	dir, err := os.MkdirTemp("tmp", "images-")
	if err != nil {
		return 0, err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	n := 0
	for _, p := range problems {
		name := p.Number
		png := filepath.Join(dir, p.Set+"-"+p.Number+".png")
		switch {
		case p.Kind == "missing image" && strings.HasPrefix(p.Detail, "https://"):
			if err := sh.Run("curl", "-sfL", "-o", png, p.Detail); err != nil {
				fmt.Println("download failed:", p.Set, p.Number, err)
				continue
			}
		case p.Kind == "missing gigantic image" && len(strings.Fields(p.Detail)) == 2:
			name += "-gigantic"
			args := append([]string{"gigantic", "-out", png}, strings.Fields(p.Detail)...)
			if err := kfdata(args...); err != nil {
				fmt.Println("gigantic failed:", p.Set, p.Number, err)
				continue
			}
		default:
			continue
		}
		webp := strings.TrimSuffix(png, ".png") + ".webp"
		// -exact keeps the transparent edges of a gigantic tile.
		if err := sh.Run(
			"cwebp", "-quiet", "-q", "80", "-exact", "-resize", "300", "420", png, "-o", webp,
		); err != nil {
			return n, err
		}
		dst := "gs://mklv-keyforge-cards-public/images/cards/" + p.Set + "/" + name + ".webp"
		if err := sh.Run("gcloud", "storage", "cp",
			"--cache-control=public, max-age=31536000, immutable",
			"--content-type=image/webp", webp, dst); err != nil {
			return n, err
		}
		fmt.Println("uploaded", dst)
		n++
	}
	return n, nil
}

// rawRoot is the raw store directory: KFDATA_ROOT, or ./raw. A worktree sets
// KFDATA_ROOT to the main checkout's raw store.
func rawRoot() string {
	if r := os.Getenv("KFDATA_ROOT"); r != "" {
		return r
	}
	return "raw"
}

// The dataset files of ADR 0007 in the public bucket.
const (
	publicBucket = "gs://mklv-keyforge-cards-public/"
	feedFile     = "tmp/feed.xml"
	latestFile   = "tmp/latest.json"
)

// uploadVersion uploads a new dataset version: the version file
// data/cards/<version>.json, then data/feed.xml, then the pointer
// data/latest.json. A client that reads the pointer always finds the version
// file. published is the version that the pointer names before the upload.
func uploadVersion(version, published string) error {
	// The published dataset and feed give the feed entry its card changes.
	// A first publish has neither, so remove the copies of a run before.
	_ = os.Remove("tmp/published.json")
	_ = os.Remove("tmp/published-feed.xml")
	if published != "" {
		_ = sh.Run("gcloud", "storage", "cp",
			publicBucket+"data/cards/"+published+".json", "tmp/published.json")
	}
	_ = sh.Run("gcloud", "storage", "cp", publicBucket+"data/feed.xml", "tmp/published-feed.xml")
	if err := kfdata(
		"feed",
		"-dataset",
		datasetFile,
		"-old",
		"tmp/published.json",
		"-old-feed",
		"tmp/published-feed.xml",
		"-out-feed",
		feedFile,
		"-out-latest",
		latestFile,
	); err != nil {
		return err
	}
	for _, u := range []struct{ src, dst, cache, typ string }{
		{datasetFile, "data/cards/" + version + ".json", "public, max-age=31536000, immutable", "application/json"},
		{feedFile, "data/feed.xml", "public, max-age=60", "application/atom+xml"},
		{latestFile, "data/latest.json", "public, max-age=60", "application/json"},
	} {
		if err := sh.RunV("gcloud", "storage", "cp", "-Z",
			"--cache-control="+u.cache, "--content-type="+u.typ,
			u.src, publicBucket+u.dst); err != nil {
			return err
		}
	}
	return nil
}

// versionMaxAge is the age at which publish deletes an old version file.
const versionMaxAge = 90 * 24 * time.Hour

// pruneVersions deletes the version files under data/cards/ that are older
// than versionMaxAge. It never deletes the current version, so
// data/latest.json always points to a file, also when no new version was
// published for a long time. A bucket lifecycle rule cannot make this
// exception.
func pruneVersions(current string) error {
	out, err := sh.Output("gcloud", "storage", "ls", "-l", publicBucket+"data/cards/")
	if err != nil {
		return err
	}
	keep := publicBucket + "data/cards/" + current + ".json"
	for line := range strings.SplitSeq(out, "\n") {
		f := strings.Fields(line)
		if len(f) != 3 || !strings.HasSuffix(f[2], ".json") || f[2] == keep {
			continue
		}
		created, err := time.Parse(time.RFC3339, f[1])
		if err != nil || time.Since(created) < versionMaxAge {
			continue
		}
		if err := sh.Run("gcloud", "storage", "rm", f[2]); err != nil {
			return err
		}
		fmt.Println("deleted the old dataset version", f[2])
	}
	return nil
}

// checkGreyFrames runs kfdata check-art -write: it checks the new card
// images for the grey frame of a maverick render and adds the hits to the
// greyFrame lists of the set override files. The check keeps its results in
// tmp/art-checks.json, so it downloads only new images. It returns the
// number of new grey frames.
func checkGreyFrames() (int, error) {
	out, err := sh.Output("go", "run", "./cmd/kfdata", "check-art", "-write")
	fmt.Println(out)
	if err != nil {
		return 0, err
	}
	n := 0
	for line := range strings.SplitSeq(out, "\n") {
		if _, err := fmt.Sscanf(line, "added %d grey frames", &n); err == nil {
			return n, nil
		}
	}
	return 0, nil
}
