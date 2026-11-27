package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json/v2"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"

	"github.com/dmikalova/keyforge-data/artists"
	"github.com/dmikalova/keyforge-data/card"
	"github.com/dmikalova/keyforge-data/query"
)

// ocrResult is one line of the artistocr output.
type ocrResult struct {
	Path     string `json:"path"`
	Readings []struct {
		Variant string `json:"variant"`
		Text    string `json:"text"`
	} `json:"readings"`
	Error string `json:"error,omitempty"`
}

func (r ocrResult) texts() []string {
	out := make([]string, 0, len(r.Readings))
	for _, x := range r.Readings {
		out = append(out, x.Text)
	}
	return out
}

// reviewItem is a printing that the scan did not match surely.
type reviewItem struct {
	Set      string   `json:"set"`
	Number   string   `json:"number"`
	Title    string   `json:"title"`
	Image    string   `json:"image"`
	Readings []string `json:"readings"`
}

// scanPrinting is a printing of the dataset without an artist.
type scanPrinting struct {
	set, number, title, image string
}

func (p scanPrinting) file() string {
	return strings.ReplaceAll(strings.Split(p.image, "?")[0], "/", "__")
}

type artistFlags struct {
	dir, cache string
}

func (a *artistFlags) register(fs *flag.FlagSet) {
	fs.StringVar(&a.dir, "dir", "overrides", "overrides directory")
	fs.StringVar(&a.cache, "cache", "tmp/artists", "image, OCR, and review cache")
}

func (a artistFlags) ocrFile() string    { return filepath.Join(a.cache, "ocr.jsonl") }
func (a artistFlags) reviewFile() string { return filepath.Join(a.cache, "review.json") }
func (a artistFlags) imageDir() string   { return filepath.Join(a.cache, "images") }

// artistsScan reads the artist of each printing that has none: it downloads
// the image, runs artistocr, and matches the readings to the known artists.
// It writes the sure matches to overrides/<set>.json and the others to the
// review queue.
func artistsScan(ctx context.Context, args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("artists-scan", flag.ContinueOnError)
	var af artistFlags
	af.register(fs)
	dataset := fs.String("dataset", "tmp/cards.json", "dataset file")
	ocrBin := fs.String("ocr", "tmp/artistocr", "artistocr binary")
	workers := fs.Int("workers", 8, "parallel downloads and OCR processes")
	hintsPath := fs.String(
		"hints",
		"",
		"CSV of card title and artist: its artists are candidates too",
	)
	if err := fs.Parse(args); err != nil {
		return err
	}
	known, err := artists.LoadKnown(af.dir)
	if err != nil {
		return err
	}
	assigned, err := artists.Load(af.dir)
	if err != nil {
		return err
	}
	aliases, err := artists.LoadAliases(af.dir)
	if err != nil {
		return err
	}
	// A new set brings artists that the known list does not have. Without
	// them as candidates, the scan matches a new artist to a known one
	// with a similar line: "Dib" became Rodrigo Camilo, not Djib.
	saved := len(known)
	if *hintsPath != "" {
		hints, err := loadHints(*hintsPath, aliases, known)
		if err != nil {
			return err
		}
		for _, name := range hints {
			if !slices.Contains(known, name) {
				known = append(known, name)
			}
		}
	}
	// A new alias renames the printings that have the variant spelling.
	aliases.CanonicalAll(assigned)
	todo, err := unassigned(*dataset, assigned)
	if err != nil {
		return err
	}
	if err := downloadImages(ctx, af.imageDir(), todo, *workers); err != nil {
		return err
	}
	results, err := runOCR(ctx, af, *ocrBin, todo, *workers)
	if err != nil {
		return err
	}
	examples := buildExamples(results, assigned)
	matched := 0
	// A match adds examples, and the examples can make more matches sure.
	// Stop when a pass matches nothing new.
	for {
		var rest []scanPrinting
		for _, p := range todo {
			texts := results[p.file()].texts()
			name, ok := artists.Decide(artists.Match(texts, known, examples, aliases))
			if !ok {
				rest = append(rest, p)
				continue
			}
			setArtist(assigned, p.set, p.number, name)
			examples.Add(name, texts)
			matched++
		}
		done := len(rest) == len(todo)
		todo = rest
		if done {
			break
		}
	}
	queue := make([]reviewItem, 0, len(todo))
	for _, p := range todo {
		queue = append(queue, reviewItem{
			Set: p.set, Number: p.number, Title: p.title,
			Image: filepath.Join(af.imageDir(), p.file()), Readings: results[p.file()].texts(),
		})
	}
	if err := saveAll(af.dir, assigned); err != nil {
		return err
	}
	// A hint artist that the scan assigned becomes a known artist.
	if err := saveUsedKnown(af.dir, known[:saved], assigned); err != nil {
		return err
	}
	if err := writeJSON(af.reviewFile(), queue); err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout,
		"%d printings without an artist: %d matched, %d to review (kfdata artists-review)\n",
		matched+len(queue), matched, len(queue))
	return err
}

// buildExamples makes the examples from the OCR readings of the printings
// that have an artist. The readings come from the OCR cache: an image file
// name is "<set>__<number>.webp".
func buildExamples(
	results map[string]ocrResult,
	assigned map[string]map[string]string,
) artists.Examples {
	ex := artists.Examples{}
	for file, r := range results {
		set, num, ok := strings.Cut(strings.TrimSuffix(file, filepath.Ext(file)), "__")
		if !ok {
			continue
		}
		num = strings.TrimSuffix(num, "-gigantic")
		if name := assigned[set][num]; name != "" {
			ex.Add(name, r.texts())
		}
	}
	return ex
}

// unassigned returns the printings of the dataset that have no artist.
func unassigned(path string, assigned map[string]map[string]string) ([]scanPrinting, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var d card.Dataset
	if err := json.Unmarshal(b, &d); err != nil {
		return nil, err
	}
	var out []scanPrinting
	for _, c := range d.Cards {
		for _, p := range c.Printings {
			if p.Image == "" || assigned[p.Set][p.Number] != "" {
				continue
			}
			out = append(
				out,
				scanPrinting{set: p.Set, number: p.Number, title: c.Title, image: p.Image},
			)
		}
	}
	return out, nil
}

func downloadImages(ctx context.Context, dir string, ps []scanPrinting, workers int) error {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	client := &http.Client{Timeout: time.Minute}
	jobs := make(chan scanPrinting)
	var mu sync.Mutex
	var errs []error
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			for p := range jobs {
				if err := download(
					ctx,
					client,
					card.ImageBase+p.image,
					filepath.Join(dir, p.file()),
				); err != nil {
					mu.Lock()
					errs = append(errs, fmt.Errorf("%s %s: %w", p.set, p.number, err))
					mu.Unlock()
				}
			}
		})
	}
	for _, p := range ps {
		jobs <- p
	}
	close(jobs)
	wg.Wait()
	return errors.Join(errs...)
}

// download writes the body of u to path, unless path exists.
func download(ctx context.Context, client *http.Client, u, path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return errors.New(resp.Status)
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

// runOCR returns the OCR result of each printing by image file name. It
// keeps the results in the OCR cache and runs artistocr only for the new
// images. Delete the cache to read all images again.
func runOCR(
	ctx context.Context, af artistFlags, bin string, ps []scanPrinting, workers int,
) (map[string]ocrResult, error) {
	results, err := readOCRCache(af.ocrFile())
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, p := range ps {
		if _, ok := results[p.file()]; !ok {
			paths = append(paths, filepath.Join(af.imageDir(), p.file()))
		}
	}
	if len(paths) == 0 {
		return results, nil
	}
	// artistocr reads the known artists from a file, one name per line.
	known, err := artists.LoadKnown(af.dir)
	if err != nil {
		return nil, err
	}
	knownFile := filepath.Join(af.cache, "known.txt")
	if err := os.WriteFile(knownFile, []byte(strings.Join(known, "\n")+"\n"), 0o600); err != nil {
		return nil, err
	}
	lines, err := ocrParallel(ctx, bin, knownFile, paths, workers)
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(af.ocrFile(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	for _, l := range lines {
		var r ocrResult
		if err := json.Unmarshal(l, &r); err != nil {
			_ = f.Close()
			return nil, err
		}
		results[filepath.Base(r.Path)] = r
		if _, err := f.Write(append(l, '\n')); err != nil {
			_ = f.Close()
			return nil, err
		}
	}
	return results, f.Close()
}

func readOCRCache(path string) (map[string]ocrResult, error) {
	out := map[string]ocrResult{}
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	for l := range bytes.Lines(b) {
		var r ocrResult
		if err := json.Unmarshal(bytes.TrimSpace(l), &r); err != nil {
			return nil, err
		}
		out[filepath.Base(r.Path)] = r
	}
	return out, nil
}

// ocrParallel runs one artistocr process per worker and returns the JSON
// lines of all of them.
func ocrParallel(
	ctx context.Context,
	bin, known string,
	paths []string,
	workers int,
) ([][]byte, error) {
	var mu sync.Mutex
	var out [][]byte
	var errs []error
	var wg sync.WaitGroup
	for w := range min(workers, len(paths)) {
		var chunk []string
		for i := w; i < len(paths); i += workers {
			chunk = append(chunk, paths[i])
		}
		wg.Go(func() {
			cmd := exec.CommandContext(ctx, bin, known)
			cmd.Stdin = strings.NewReader(strings.Join(chunk, "\n") + "\n")
			cmd.Stderr = os.Stderr
			b, err := cmd.Output()
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs = append(errs, err)
				return
			}
			for l := range bytes.Lines(b) {
				if l = bytes.TrimSpace(l); len(l) > 0 {
					out = append(out, slices.Clone(l))
				}
			}
		})
	}
	wg.Wait()
	return out, errors.Join(errs...)
}

func setArtist(m map[string]map[string]string, set, number, name string) {
	if m[set] == nil {
		m[set] = map[string]string{}
	}
	m[set][number] = name
}

func saveAll(dir string, m map[string]map[string]string) error {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	for set, nums := range m {
		if err := artists.Save(dir, set, nums); err != nil {
			return err
		}
	}
	return nil
}

func writeJSON(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

// reviewer asks the user for the artist of each printing in the review
// queue.
type reviewer struct {
	af       artistFlags
	show     string
	known    []string
	assigned map[string]map[string]string
	// examples grow with each answer, so the next matches improve.
	examples artists.Examples
	aliases  artists.Aliases
	in       *bufio.Reader
	out      io.Writer
}

// artistsReview asks for the artist of each printing in the review queue.
// It saves after each answer, so a stopped review continues where it was.
func artistsReview(args []string, stdin io.Reader, stdout io.Writer) error {
	fs := flag.NewFlagSet("artists-review", flag.ContinueOnError)
	r := reviewer{in: bufio.NewReader(stdin), out: stdout}
	r.af.register(fs)
	fs.StringVar(
		&r.show,
		"show",
		showAuto,
		"how to show the review image: auto, kitty (Ghostty, kitty), inline (iTerm2, VS Code), open (Preview), none",
	)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if r.show == showAuto {
		r.show = detectShow()
	}
	b, err := os.ReadFile(r.af.reviewFile())
	if err != nil {
		return err
	}
	var queue []reviewItem
	if err := json.Unmarshal(b, &queue); err != nil {
		return err
	}
	if r.known, err = artists.LoadKnown(r.af.dir); err != nil {
		return err
	}
	if r.assigned, err = artists.Load(r.af.dir); err != nil {
		return err
	}
	if r.aliases, err = artists.LoadAliases(r.af.dir); err != nil {
		return err
	}
	r.aliases.CanonicalAll(r.assigned)
	results, err := readOCRCache(r.af.ocrFile())
	if err != nil {
		return err
	}
	r.examples = buildExamples(results, r.assigned)
	for len(queue) > 0 {
		// The scan queues the printings of one card together. Ask one time
		// for all of them: reprints mostly keep the art.
		n := 1
		for n < len(queue) && queue[n].Title == queue[0].Title {
			n++
		}
		answered, err := r.askGroup(queue[:n], len(queue))
		if err != nil {
			return err
		}
		if answered < 0 {
			break
		}
		queue = queue[answered:]
		if err := writeJSON(r.af.reviewFile(), queue); err != nil {
			return err
		}
	}
	_, err = fmt.Fprintf(stdout, "%d printings left in the review queue\n", len(queue))
	return err
}

// askGroup asks for the artist of the printings of one card. It returns the
// number of printings that it handled, or -1 when the user quits. The answer
// "p" asks for the first printing alone, for a reprint with new art.
func (r *reviewer) askGroup(group []reviewItem, left int) (int, error) {
	var readings []string
	for _, it := range group {
		readings = append(readings, it.Readings...)
	}
	cands := artists.Match(readings, r.known, r.examples, r.aliases)
	// An artist that the user added can make the match sure now.
	if name, ok := artists.Decide(cands); ok && len(group) == 1 {
		_, _ = fmt.Fprintf(
			r.out,
			"%s %s %s: %s (sure match)\n",
			group[0].Set,
			group[0].Number,
			group[0].Title,
			name,
		)
		return 1, r.assign(group, name)
	}
	r.printGroup(group, cands, left)
	for {
		_, _ = fmt.Fprint(r.out, r.prompt(len(group)))
		line, err := r.in.ReadString('\n')
		if err != nil && line == "" {
			return -1, nil
		}
		line = strings.TrimSpace(line)
		switch {
		case line == "q":
			return -1, nil
		case line == "s":
			return len(group), nil
		case line == "p" && len(group) > 1:
			return r.askGroup(group[:1], left)
		case line == "" && len(cands) > 0:
			return len(group), r.assign(group, cands[0].Name)
		}
		if n, err := strconv.Atoi(line); err == nil && n >= 1 && n <= min(5, len(cands)) {
			return len(group), r.assign(group, cands[n-1].Name)
		}
		if name, ok := r.typed(line); ok {
			return len(group), r.assign(group, name)
		}
	}
}

func (r *reviewer) printGroup(group []reviewItem, cands []artists.Candidate, left int) {
	if err := r.showStrips(group); err != nil {
		_, _ = fmt.Fprintln(r.out, "cannot show the image:", err)
	}
	_, _ = fmt.Fprintf(r.out, "\n[%d left] %s\n", left, group[0].Title)
	for _, it := range group {
		_, _ = fmt.Fprintf(r.out, "  %s %s: %s\n    OCR: %s\n",
			it.Set, it.Number, it.Image, strings.Join(uniq(it.Readings), " | "))
	}
	for i, c := range cands[:min(5, len(cands))] {
		_, _ = fmt.Fprintf(r.out, "  %d) %s (%.2f)\n", i+1, c.Name, c.Score)
	}
}

func (r *reviewer) prompt(n int) string {
	if n > 1 {
		return fmt.Sprintf(
			"all %d printings: Enter=1, 1-5=pick, a name, p=one printing at a time, s=skip, q=quit > ",
			n,
		)
	}
	return "Enter=1, 1-5=pick, a name, s=skip, q=quit > "
}

// typed returns the known artist that the user typed, or adds a new artist
// to the known list after the user confirms it.
func (r *reviewer) typed(line string) (string, bool) {
	if len(line) < 2 {
		return "", false
	}
	line = r.aliases.Canonical(line)
	for _, k := range r.known {
		if query.Normalize(k) == query.Normalize(line) {
			return k, true
		}
	}
	_, _ = fmt.Fprintf(r.out, "%q is not a known artist. Add it? [y/N] ", line)
	ans, _ := r.in.ReadString('\n')
	if strings.TrimSpace(strings.ToLower(ans)) != "y" {
		return "", false
	}
	r.known = append(r.known, line)
	if err := artists.SaveKnown(r.af.dir, r.known); err != nil {
		_, _ = fmt.Fprintln(r.out, "cannot save the known artists:", err)
		return "", false
	}
	return line, true
}

func (r *reviewer) assign(group []reviewItem, name string) error {
	for _, it := range group {
		setArtist(r.assigned, it.Set, it.Number, name)
		r.examples.Add(name, it.Readings)
		if err := artists.Save(r.af.dir, it.Set, r.assigned[it.Set]); err != nil {
			return err
		}
	}
	return nil
}

// Ways to show the review image.
const (
	showAuto   = "auto"
	showKitty  = "kitty"
	showInline = "inline"
	showOpen   = "open"
	showNone   = "none"
)

// detectShow picks the image protocol of the terminal: the kitty graphics
// protocol for Ghostty and kitty, the iTerm2 protocol for iTerm2 and VS Code,
// and Preview for other terminals.
func detectShow() string {
	term, prog := os.Getenv("TERM"), os.Getenv("TERM_PROGRAM")
	switch {
	case prog == "ghostty" || term == "xterm-ghostty" || term == "xterm-kitty" ||
		os.Getenv("KITTY_WINDOW_ID") != "":
		return showKitty
	case prog == "iTerm.app" || prog == "vscode":
		return showInline
	default:
		return showOpen
	}
}

func (r *reviewer) showStrips(group []reviewItem) error {
	if r.show == showNone {
		return nil
	}
	png, err := reviewPNG(group)
	if err != nil {
		return err
	}
	switch r.show {
	case showKitty:
		return writeKitty(r.out, png)
	case showInline:
		// The iTerm2 inline image protocol. VS Code shows it when
		// terminal.integrated.enableImages is on.
		_, err := fmt.Fprintf(r.out, "\x1b]1337;File=inline=1;preserveAspectRatio=1:%s\a\n",
			base64.StdEncoding.EncodeToString(png))
		return err
	}
	it := group[0]
	path := strings.TrimSuffix(it.Image, filepath.Ext(it.Image)) + "-strip.png"
	if err := os.WriteFile(path, png, 0o600); err != nil {
		return err
	}
	return exec.Command("open", "-g", path).Run()
}

// writeKitty shows a PNG with the kitty graphics protocol, which Ghostty
// supports. The data goes in chunks of 4096 bytes. q=2 stops the terminal
// replies, which would otherwise come in as typed input.
func writeKitty(w io.Writer, png []byte) error {
	data := base64.StdEncoding.EncodeToString(png)
	var b strings.Builder
	for i := 0; i < len(data); i += 4096 {
		chunk := data[i:min(i+4096, len(data))]
		more := 0
		if i+4096 < len(data) {
			more = 1
		}
		if i == 0 {
			fmt.Fprintf(&b, "\x1b_Gf=100,a=T,q=2,m=%d;%s\x1b\\", more, chunk)
		} else {
			fmt.Fprintf(&b, "\x1b_Gm=%d;%s\x1b\\", more, chunk)
		}
	}
	b.WriteString("\n")
	_, err := io.WriteString(w, b.String())
	return err
}

// Layout of the review image. The card title is on top. Under it on the
// left, each printing has its set and number and its artist strip, scaled 5
// times. On the right is the full first card, for a gigantic or odd image.
const (
	stripScale = 5
	stripW     = 130 * stripScale
	stripH     = 26 * stripScale
	cardW      = 450
	cardH      = 630
	gap        = 10
	titleScale = 3
	labelScale = 2
	// lineH is the height of a basicfont line.
	lineH = 13
)

var (
	reviewBG    = image.NewUniform(color.RGBA{0x20, 0x20, 0x24, 0xff})
	reviewText  = image.NewUniform(color.RGBA{0xf0, 0xf0, 0xf0, 0xff})
	reviewLabel = image.NewUniform(color.RGBA{0xa0, 0xa8, 0xb8, 0xff})
	// asciiTitle replaces the characters that basicfont does not have.
	asciiTitle = strings.NewReplacer(
		"’",
		"'",
		"‘",
		"'",
		"“",
		`"`,
		"”",
		`"`,
		"–",
		"-",
		"—",
		"-",
		"…",
		"...",
	)
)

// reviewPNG returns the review image of the printings of one card.
func reviewPNG(group []reviewItem) ([]byte, error) {
	titleH := lineH*titleScale + 2*gap
	rowH := lineH*labelScale + stripH + gap
	h := titleH + max(rowH*len(group), cardH+gap)
	out := image.NewRGBA(image.Rect(0, 0, stripW+gap+cardW+gap, h))
	draw.Draw(out, out.Bounds(), reviewBG, image.Point{}, draw.Src)
	drawText(out, image.Pt(gap, gap), asciiTitle.Replace(group[0].Title), titleScale, reviewText)
	for i, it := range group {
		src, err := decodeFile(it.Image)
		if err != nil {
			return nil, err
		}
		top := titleH + i*rowH
		drawText(out, image.Pt(0, top), it.Set+" "+it.Number, labelScale, reviewLabel)
		b := src.Bounds()
		sx, sy := float64(b.Dx())/300, float64(b.Dy())/420
		crop := image.Rect(int(20*sx), int(378*sy), int(150*sx), int(404*sy)).Add(b.Min)
		stripTop := top + lineH*labelScale
		xdraw.CatmullRom.Scale(out, image.Rect(0, stripTop, stripW, stripTop+stripH),
			src, crop, xdraw.Over, nil)
		if i == 0 {
			xdraw.CatmullRom.Scale(
				out,
				image.Rect(stripW+gap, titleH, stripW+gap+cardW, titleH+cardH),
				src,
				b,
				xdraw.Over,
				nil,
			)
		}
	}
	var buf bytes.Buffer
	err := png.Encode(&buf, out)
	return buf.Bytes(), err
}

// drawText draws s with basicfont, scaled up with nearest neighbor, with its
// top left corner at at.
func drawText(dst draw.Image, at image.Point, s string, scale int, col image.Image) {
	face := basicfont.Face7x13
	w := font.MeasureString(face, s).Ceil()
	small := image.NewRGBA(image.Rect(0, 0, w, lineH))
	d := font.Drawer{Dst: small, Src: col, Face: face, Dot: fixed.P(0, face.Ascent)}
	d.DrawString(s)
	r := image.Rect(at.X, at.Y, at.X+w*scale, at.Y+lineH*scale)
	xdraw.NearestNeighbor.Scale(dst, r, small, small.Bounds(), xdraw.Over, nil)
}

func decodeFile(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	img, _, err := image.Decode(f)
	return img, err
}

func uniq(s []string) []string {
	out := slices.Clone(s)
	slices.Sort(out)
	return slices.Compact(out)
}

// saveUsedKnown adds the assigned artists that known does not have to the
// known artists.
func saveUsedKnown(dir string, known []string, assigned map[string]map[string]string) error {
	out := slices.Clone(known)
	for _, nums := range assigned {
		for _, name := range nums {
			if !slices.Contains(out, name) {
				out = append(out, name)
			}
		}
	}
	if len(out) == len(known) {
		return nil
	}
	return artists.SaveKnown(dir, out)
}
