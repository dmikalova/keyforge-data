package query

import (
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// foldPairs maps the special characters of KeyForge text to ASCII before the
// diacritics are removed. The runes are code points, so invisible characters
// stay visible in the source.
var foldPairs = []struct {
	r  rune
	to string
}{
	{0x2019, "'"},   // right single quote
	{0x2018, "'"},   // left single quote
	{0x02bc, "'"},   // modifier letter apostrophe
	{0x201c, `"`},   // left double quote
	{0x201d, `"`},   // right double quote
	{0x00c6, "ae"},  // capital ae
	{0x00e6, "ae"},  // small ae
	{0x0152, "oe"},  // capital oe
	{0x0153, "oe"},  // small oe
	{0x00df, "ss"},  // sharp s
	{0x00d8, "o"},   // capital o with stroke
	{0x00f8, "o"},   // small o with stroke
	{0x0141, "l"},   // capital l with stroke
	{0x0142, "l"},   // small l with stroke
	{0x00d7, "x"},   // multiplication sign
	{0x2010, "-"},   // hyphen
	{0x2011, "-"},   // non-breaking hyphen
	{0x2013, "-"},   // en dash
	{0x2014, "-"},   // em dash
	{0x2212, "-"},   // minus sign
	{0x00a0, " "},   // non-breaking space
	{0xfeff, ""},    // byte-order mark
	{0x200b, ""},    // zero-width space
	{0x2026, "..."}, // ellipsis
}

var foldReplacer = func() *strings.Replacer {
	args := make([]string, 0, 2*len(foldPairs))
	for _, p := range foldPairs {
		args = append(args, string(p.r), p.to)
	}
	return strings.NewReplacer(args...)
}()

// Fold returns s in lowercase ASCII where it can: Æ becomes ae, letters lose
// their diacritics, and typographic quotes and dashes become plain ones.
// Other characters stay.
func Fold(s string) string {
	s = foldReplacer.Replace(s)
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	if folded, _, err := transform.String(t, s); err == nil {
		s = folded
	}
	return strings.ToLower(s)
}

// Normalize returns Fold(s) with only its letters and digits. It is the form
// for enum values and for fuzzy name matching: "Star Alliance",
// "staralliance", and "star-alliance" are the same.
func Normalize(s string) string {
	s = Fold(s)
	var b strings.Builder
	for _, r := range s {
		if r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)) {
			b.WriteRune(r)
		}
	}
	return b.String()
}
