//go:build mage

package main

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
)

// iconsDst is where the sites read the icons.
const iconsDst = "gs://mklv-keyforge-cards-public/images/icons/"

// Icons holds the targets for the icons in assets/icons (ADR 0005).
type Icons mg.Namespace

// Publish converts the PNG icons to webp and uploads all icons to the
// public bucket at images/icons/<group>/<name>.<ext>. The cache time is one
// day, so a redrawn icon replaces the old one soon.
func (Icons) Publish() error {
	dir, err := os.MkdirTemp("tmp", "icons-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	groups := []string{"house", "rarity", "set", "stat", "type"}
	for _, g := range groups {
		out := filepath.Join(dir, g)
		if err := os.MkdirAll(out, 0o750); err != nil {
			return err
		}
		files, err := filepath.Glob(filepath.Join("assets/icons", g, "*"))
		if err != nil {
			return err
		}
		for _, f := range files {
			name := filepath.Base(f)
			switch filepath.Ext(name) {
			case ".png":
				webp := filepath.Join(out, strings.TrimSuffix(name, ".png")+".webp")
				if err := sh.Run(
					"cwebp",
					"-quiet",
					"-q",
					"90",
					"-exact",
					f,
					"-o",
					webp,
				); err != nil {
					return err
				}
			case ".svg":
				if err := sh.Copy(filepath.Join(out, name), f); err != nil {
					return err
				}
			}
		}
	}
	return sh.RunV("gcloud", "storage", "rsync", "--recursive",
		"--cache-control=public, max-age=86400", dir, iconsDst)
}
