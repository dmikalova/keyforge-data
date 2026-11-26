//go:build mage

package main

import (
	"bytes"
	"encoding/json/v2"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
)

const (
	// datasetObject is the dataset in the public bucket. The search site
	// reads it and reloads it when it changes.
	datasetObject = "gs://mklv-keyforge-cards-public/data/cards.json"
	datasetURL    = "https://storage.googleapis.com/mklv-keyforge-cards-public/data/cards.json"
	datasetFile   = "tmp/cards.json"
	problemsFile  = "docs/dataset-problems.md"
	repo          = "dmikalova/keyforge-data"
	// discordSecrets holds the maintenance channel webhook.
	discordSecrets = "../infrastructure/secrets/discord.sops.json"
)

// Dataset holds the targets that make and publish the card dataset.
type Dataset mg.Namespace

// Distill writes tmp/cards.json and docs/dataset-problems.md from ./raw.
func (Dataset) Distill() error {
	return kfdata("distill", "-out", datasetFile, "-problems", problemsFile)
}

// Publish distills the dataset, uploads it to the public bucket if its
// version changed, and posts changed problems to the maintenance Discord channel.
func (Dataset) Publish() error {
	mg.Deps(Dataset.Distill)
	local, err := version(datasetFile)
	if err != nil {
		return err
	}
	if remote := publishedVersion(); remote == local {
		fmt.Println("dataset", local, "is already published")
	} else {
		if err := sh.RunV("gcloud", "storage", "cp", "-Z",
			"--cache-control=public, max-age=60",
			"--content-type=application/json",
			datasetFile, datasetObject); err != nil {
			return err
		}
		fmt.Println("published dataset", local, "over", remote)
	}
	return notifyProblems()
}

func version(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var d struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(b, &d); err != nil {
		return "", err
	}
	return d.Version, nil
}

// publishedVersion returns the version of the published dataset, or empty.
func publishedVersion() string {
	client := &http.Client{Timeout: time.Minute}
	resp, err := client.Get(datasetURL)
	if err != nil {
		return ""
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	var d struct {
		Version string `json:"version"`
	}
	if err := json.UnmarshalRead(resp.Body, &d); err != nil {
		return ""
	}
	return d.Version
}

// notifyProblems posts the problems summary to the maintenance Discord
// channel when the problems differ from the committed problems file. Commit
// the problems file after a publish, so the next publish compares against
// it.
func notifyProblems() error {
	body, err := os.ReadFile(problemsFile)
	if err != nil {
		return err
	}
	old, err := sh.Output("git", "show", "HEAD:"+problemsFile)
	if err != nil {
		old = ""
	}
	if withoutVersion(old) == withoutVersion(string(body)) {
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
	return nil
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
