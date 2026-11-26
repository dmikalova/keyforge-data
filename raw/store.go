// Package raw is the raw store: the Master Vault responses as received
// (ADR 0002).
//
// The layout under the root directory is:
//
//	crawls/<crawl-id>/manifest.json         the crawl state
//	crawls/<crawl-id>/decks/<seq>.jsonl.zst  closed deck shards
//	crawls/<crawl-id>/decks/<seq>.jsonl      the open deck shard
//	linked/<kind>/<locale>/<id>/<hash>.json  linked objects, one file each
//
// A deck shard has one Record per line. A linked object is stored one time
// for each distinct content, so a changed card keeps its history. The root
// mirrors gs://mklv-keyforge-cards/raw/mv/.
package raw

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/dmikalova/keyforge-data/mv"
)

// ShardSize is the number of decks in one closed shard.
const ShardSize = 10_000

// Record is one deck line in a shard.
type Record struct {
	// Fetched is the time of the request.
	Fetched time.Time `json:"fetched"`
	// Locale is the language of the request.
	Locale string `json:"locale"`
	// Source is "feed" or "deck".
	Source string `json:"source"`
	// Deck is the deck object as received, without the linked objects.
	Deck jsontext.Value `json:"deck"`
}

// Store is a raw store in a local directory.
type Store struct {
	Root string
}

// Open returns the store at root and makes the directory.
func Open(root string) (*Store, error) {
	if err := os.MkdirAll(root, 0o750); err != nil {
		return nil, err
	}
	return &Store{Root: root}, nil
}

// PutLinked writes each linked object that the store does not have yet. It
// returns the number of new files.
func (s *Store) PutLinked(locale string, linked map[string][]mv.Object) (int, error) {
	n := 0
	for kind, objs := range linked {
		for _, o := range objs {
			if o.ID == "" {
				continue
			}
			sum := sha256.Sum256(o.Raw)
			dir := filepath.Join(s.Root, "linked", kind, locale, o.ID)
			path := filepath.Join(dir, hex.EncodeToString(sum[:8])+".json")
			if _, err := os.Stat(path); err == nil {
				continue
			}
			if err := os.MkdirAll(dir, 0o750); err != nil {
				return n, err
			}
			if err := writeAtomic(path, o.Raw); err != nil {
				return n, err
			}
			n++
		}
	}
	return n, nil
}

// LinkedVersion is one stored version of a linked object.
type LinkedVersion struct {
	ID      string
	Hash    string
	ModTime time.Time
	Raw     []byte
}

// Linked returns every stored version of every object of a kind in a
// language, by object ID.
func (s *Store) Linked(kind, locale string) (map[string][]LinkedVersion, error) {
	dir := filepath.Join(s.Root, "linked", kind, locale)
	out := map[string][]LinkedVersion{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return fs.SkipAll
			}
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ".json" {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		id := filepath.Base(filepath.Dir(path))
		out[id] = append(out[id], LinkedVersion{
			ID:      id,
			Hash:    trimExt(filepath.Base(path)),
			ModTime: info.ModTime(),
			Raw:     b,
		})
		return nil
	})
	return out, err
}

// Crawls returns the IDs of the stored crawls.
func (s *Store) Crawls() ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(s.Root, "crawls"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	return out, nil
}

func trimExt(name string) string {
	return name[:len(name)-len(filepath.Ext(name))]
}

// writeAtomic writes a file through a temporary file and a rename, so a stop
// never leaves half a file.
func writeAtomic(path string, b []byte) error {
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	w := bufio.NewWriter(f)
	if _, err := w.Write(b); err != nil {
		_ = f.Close()
		return err
	}
	if err := w.Flush(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func writeJSONAtomic(path string, v any) error {
	b, err := json.Marshal(v, jsontext.WithIndent("  "))
	if err != nil {
		return fmt.Errorf("raw: encode %s: %w", path, err)
	}
	return writeAtomic(path, append(b, '\n'))
}
