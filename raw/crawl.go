package raw

import (
	"bufio"
	"bytes"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"iter"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/klauspost/compress/zstd"

	"github.com/dmikalova/keyforge-data/mv"
)

// Manifest is the state of one crawl.
type Manifest struct {
	ID      string    `json:"id"`
	Mode    string    `json:"mode"`
	Started time.Time `json:"started"`
	Updated time.Time `json:"updated"`
	// Shards are the closed shards, in order.
	Shards []Shard `json:"shards"`
	// Open is the number of decks in the open shard.
	Open int `json:"open"`
	// Decks is the number of deck records in all shards.
	Decks int `json:"decks"`
	// Done is true when the crawl is complete.
	Done bool `json:"done"`
	// State is the crawler's own resume state.
	State jsontext.Value `json:"state,omitempty"`
}

// Shard is one closed shard.
type Shard struct {
	Name  string `json:"name"`
	Decks int    `json:"decks"`
}

// Crawl is an open crawl. Write decks with WriteDecks, save the crawler state
// with Checkpoint, and call Close at the end.
type Crawl struct {
	store    *Store
	dir      string
	Manifest Manifest

	open *os.File
	buf  *bufio.Writer
}

// OpenCrawl opens the crawl with the ID, or starts it. A started crawl
// continues from its manifest.
func (s *Store) OpenCrawl(id, mode string) (*Crawl, error) {
	dir := filepath.Join(s.Root, "crawls", id)
	if err := os.MkdirAll(filepath.Join(dir, "decks"), 0o750); err != nil {
		return nil, err
	}
	c := &Crawl{store: s, dir: dir}
	b, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	switch {
	case errors.Is(err, fs.ErrNotExist):
		now := time.Now().UTC()
		c.Manifest = Manifest{ID: id, Mode: mode, Started: now, Updated: now}
	case err != nil:
		return nil, err
	default:
		if err := json.Unmarshal(b, &c.Manifest); err != nil {
			return nil, fmt.Errorf("raw: decode manifest of %s: %w", id, err)
		}
		if c.Manifest.Mode != mode {
			return nil, fmt.Errorf(
				"raw: crawl %s has mode %q, not %q",
				id, c.Manifest.Mode, mode,
			)
		}
	}
	if err := c.openShard(); err != nil {
		return nil, err
	}
	return c, nil
}

// Dir returns the crawl directory.
func (c *Crawl) Dir() string { return c.dir }

func (c *Crawl) openPath() string {
	return filepath.Join(c.dir, "decks", fmt.Sprintf("%05d.jsonl", len(c.Manifest.Shards)))
}

// openShard opens the open shard for append. It drops a partial last line
// and any lines after the manifest's count, which a stop between a write and
// a checkpoint can leave.
func (c *Crawl) openShard() error {
	path := c.openPath()
	if err := truncateLines(path, c.Manifest.Open); err != nil {
		return err
	}
	f, err := os.OpenFile(
		path,
		os.O_CREATE|os.O_APPEND|os.O_WRONLY,
		0o640,
	)
	if err != nil {
		return err
	}
	c.open = f
	c.buf = bufio.NewWriter(f)
	return nil
}

// truncateLines keeps the first n lines of the file.
func truncateLines(path string, n int) error {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	end := 0
	for range n {
		i := bytes.IndexByte(b[end:], '\n')
		if i < 0 {
			return fmt.Errorf("raw: %s has fewer than %d lines", path, n)
		}
		end += i + 1
	}
	if end == len(b) {
		return nil
	}
	return os.Truncate(path, int64(end))
}

// WriteDecks appends deck records to the open shard. It closes the shard when
// it is full.
func (c *Crawl) WriteDecks(locale, source string, decks []mv.Deck) error {
	now := time.Now().UTC()
	for _, d := range decks {
		line, err := json.Marshal(Record{Fetched: now, Locale: locale, Source: source, Deck: d.Raw})
		if err != nil {
			return err
		}
		if _, err := c.buf.Write(append(line, '\n')); err != nil {
			return err
		}
		c.Manifest.Open++
		c.Manifest.Decks++
		if c.Manifest.Open >= ShardSize {
			if err := c.closeShard(); err != nil {
				return err
			}
			if err := c.openShard(); err != nil {
				return err
			}
		}
	}
	return nil
}

// PutLinked writes the linked objects to the store.
func (c *Crawl) PutLinked(locale string, linked map[string][]mv.Object) (int, error) {
	return c.store.PutLinked(locale, linked)
}

// Checkpoint flushes the open shard and saves the manifest with the state.
func (c *Crawl) Checkpoint(state any) error {
	if err := c.flush(); err != nil {
		return err
	}
	if state != nil {
		b, err := json.Marshal(state)
		if err != nil {
			return err
		}
		c.Manifest.State = b
	}
	c.Manifest.Updated = time.Now().UTC()
	return writeJSONAtomic(filepath.Join(c.dir, "manifest.json"), c.Manifest)
}

// State decodes the saved crawler state into v. It does nothing when no
// state is saved.
func (c *Crawl) State(v any) error {
	if len(c.Manifest.State) == 0 {
		return nil
	}
	return json.Unmarshal(c.Manifest.State, v)
}

func (c *Crawl) flush() error {
	if err := c.buf.Flush(); err != nil {
		return err
	}
	return c.open.Sync()
}

// closeShard compresses the open shard into a closed shard.
func (c *Crawl) closeShard() error {
	if err := c.flush(); err != nil {
		return err
	}
	if err := c.open.Close(); err != nil {
		return err
	}
	path := c.openPath()
	name := filepath.Base(path) + ".zst"
	if err := compressFile(path, filepath.Join(c.dir, "decks", name)); err != nil {
		return err
	}
	c.Manifest.Shards = append(c.Manifest.Shards, Shard{Name: name, Decks: c.Manifest.Open})
	c.Manifest.Open = 0
	if err := writeJSONAtomic(filepath.Join(c.dir, "manifest.json"), c.Manifest); err != nil {
		return err
	}
	return os.Remove(path)
}

// Close saves the state and closes the crawl. With done set, it also closes
// the open shard and marks the crawl complete.
func (c *Crawl) Close(state any, done bool) error {
	if done {
		c.Manifest.Done = true
		if c.Manifest.Open > 0 {
			if err := c.closeShard(); err != nil {
				return err
			}
			if err := c.openShard(); err != nil {
				return err
			}
		}
	}
	if err := c.Checkpoint(state); err != nil {
		return err
	}
	return c.open.Close()
}

func compressFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	tmp := dst + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	enc, err := zstd.NewWriter(out, zstd.WithEncoderLevel(zstd.SpeedBetterCompression))
	if err != nil {
		_ = out.Close()
		return err
	}
	if _, err := in.WriteTo(enc); err != nil {
		_ = enc.Close()
		_ = out.Close()
		return err
	}
	if err := enc.Close(); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Sync(); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}

// Records reads every deck record of a crawl: the closed shards in order,
// then the open shard.
func (s *Store) Records(crawlID string) iter.Seq2[Record, error] {
	return func(yield func(Record, error) bool) {
		dir := filepath.Join(s.Root, "crawls", crawlID, "decks")
		entries, err := os.ReadDir(dir)
		if err != nil {
			yield(Record{}, err)
			return
		}
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			n := e.Name()
			if strings.HasSuffix(n, ".jsonl.zst") || strings.HasSuffix(n, ".jsonl") {
				names = append(names, n)
			}
		}
		slices.Sort(names)
		for _, n := range names {
			if !readShard(filepath.Join(dir, n), yield) {
				return
			}
		}
	}
}

func readShard(path string, yield func(Record, error) bool) bool {
	f, err := os.Open(path)
	if err != nil {
		return yield(Record{}, err)
	}
	defer func() { _ = f.Close() }()
	var r io.Reader = f
	if strings.HasSuffix(path, ".zst") {
		dec, err := zstd.NewReader(f)
		if err != nil {
			return yield(Record{}, err)
		}
		defer dec.Close()
		r = dec
	}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 1<<20), 64<<20)
	for sc.Scan() {
		var rec Record
		if err := json.Unmarshal(sc.Bytes(), &rec); err != nil {
			// A partial last line in the open shard is not an error.
			if !strings.HasSuffix(path, ".zst") {
				return true
			}
			return yield(Record{}, fmt.Errorf("raw: %s: %w", path, err))
		}
		if !yield(rec, nil) {
			return false
		}
	}
	if err := sc.Err(); err != nil {
		return yield(Record{}, err)
	}
	return true
}
