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
	// problemsIssue is the title of the GitHub issue that lists the dataset
	// problems. A change in the problems adds a comment, so the user gets a
	// notification.
	problemsIssue = "Dataset problems"
	repo          = "dmikalova/keyforge-data"
)

// Dataset holds the targets that make and publish the card dataset.
type Dataset mg.Namespace

// Distill writes tmp/cards.json and docs/dataset-problems.md from ./raw.
func (Dataset) Distill() error {
	return kfdata("distill", "-out", datasetFile, "-problems", problemsFile)
}

// Publish distills the dataset, uploads it to the public bucket if its
// version changed, and updates the "Dataset problems" GitHub issue.
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
	return updateProblemsIssue()
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

// gh runs the GitHub CLI with the keyring login. The GITHUB_TOKEN in the
// environment is not valid for this user.
func gh(args ...string) (string, error) {
	return sh.OutputWith(map[string]string{"GITHUB_TOKEN": ""}, "gh", args...)
}

// updateProblemsIssue makes the issue body the problems file. If the
// problems changed, it adds a comment with the new summary line, so the user
// gets a notification. With no problems, it closes the issue.
func updateProblemsIssue() error {
	body, err := os.ReadFile(problemsFile)
	if err != nil {
		return err
	}
	number, err := gh("issue", "list", "-R", repo, "--state", "all", "--search",
		"in:title \""+problemsIssue+"\"", "--json", "number", "--jq", ".[0].number // empty")
	if err != nil {
		return err
	}
	summary := problemsSummary(string(body))
	clean := strings.Contains(summary, "Problems: 0.")
	if number == "" {
		if clean {
			return nil
		}
		return sh.RunWith(
			map[string]string{"GITHUB_TOKEN": ""},
			"gh",
			"issue",
			"create",
			"-R",
			repo,
			"--title",
			problemsIssue,
			"--body-file",
			problemsFile,
			"--assignee",
			"dmikalova",
		)
	}
	old, err := gh("issue", "view", number, "-R", repo, "--json", "body", "--jq", ".body")
	if err != nil {
		return err
	}
	if strings.TrimSpace(old) == string(bytes.TrimSpace(body)) {
		return nil
	}
	if _, err := gh("issue", "edit", number, "-R", repo, "--body-file", problemsFile); err != nil {
		return err
	}
	if clean {
		_, err = gh("issue", "close", number, "-R", repo, "--comment", summary)
		return err
	}
	state, err := gh("issue", "view", number, "-R", repo, "--json", "state", "--jq", ".state")
	if err != nil {
		return err
	}
	if state != "OPEN" {
		if _, err := gh("issue", "reopen", number, "-R", repo); err != nil {
			return err
		}
	}
	_, err = gh("issue", "comment", number, "-R", repo, "--body", "The problems changed. "+summary)
	return err
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
