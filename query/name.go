package query

import "strings"

// The match tiers of a name, from the worst to the best. A match in a better
// tier always scores more than a match in a worse tier.
const (
	tierSubstring = iota
	tierWordStart
	tierPrefix
	tierExact
)

// tierWeight is the score of one tier. The points inside a tier stay below
// it.
const tierWeight = 1000

// NameScore reports whether pattern is a substring of text, and how good
// the match is. Both are compared in normalized form, so case, diacritics,
// spaces, and punctuation do not matter. An empty pattern matches with a
// score of zero.
//
// A higher score is a better match. The tiers, from the best: the whole
// text, a prefix, a match at the start of a word, and a match inside a
// word. Inside a tier, a shorter text scores more, because the pattern
// covers more of it.
func NameScore(pattern, text string) (int, bool) {
	p := Normalize(pattern)
	if p == "" {
		return 0, true
	}
	chars, starts := wordChars(text)
	s := string(chars)
	t := -1
	switch {
	case s == p:
		t = tierExact
	case strings.HasPrefix(s, p):
		t = tierPrefix
	default:
		for i := 0; i+len(p) <= len(s); i++ {
			if s[i:i+len(p)] != p {
				continue
			}
			if starts[i] {
				t = tierWordStart
				break
			}
			t = tierSubstring
		}
	}
	if t < 0 {
		return 0, false
	}
	return t*tierWeight + max(0, tierWeight-1-len(s)), true
}

// wordChars returns the normalized characters of text and marks the ones
// that start a word.
func wordChars(text string) ([]byte, []bool) {
	f := Fold(text)
	var chars []byte
	var starts []bool
	newWord := true
	for i := range len(f) {
		c := f[i]
		if c >= 'a' && c <= 'z' || c >= '0' && c <= '9' {
			chars = append(chars, c)
			starts = append(starts, newWord)
			newWord = false
			continue
		}
		newWord = true
	}
	return chars, starts
}
